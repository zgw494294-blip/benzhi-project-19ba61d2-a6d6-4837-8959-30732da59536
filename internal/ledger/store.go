package ledger

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
)

const schemaVersion = 1
const maxFrameSize = 32 << 20

type Store struct {
	mu             sync.RWMutex
	directory      string
	logPath        string
	snapshotPath   string
	logFile        *os.File
	tasks          map[string]*domain.TrialTask
	idempotency    map[string]idempotencyRecord
	lastSequence   uint64
	lastDigest     string
	credentialTask map[string]string
}

func Open(directory string) (*Store, error) {
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, err
	}
	logPath := filepath.Join(directory, "events.frames")
	file, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE, 0o640)
	if err != nil {
		return nil, err
	}
	s := &Store{directory: directory, logPath: logPath, snapshotPath: filepath.Join(directory, "projection.json"), logFile: file, tasks: map[string]*domain.TrialTask{}, idempotency: map[string]idempotencyRecord{}, credentialTask: map[string]string{}}
	if err := s.recoverFrames(); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := s.verifyExistingProjection(); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := s.writeProjectionLocked(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logFile == nil {
		return nil
	}
	err := s.logFile.Close()
	s.logFile = nil
	return err
}

func cloneTask(task *domain.TrialTask) (*domain.TrialTask, error) {
	raw, err := json.Marshal(task)
	if err != nil {
		return nil, err
	}
	var result domain.TrialTask
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func checksumFrame(f frame) (string, error) {
	f.Digest = ""
	raw, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Store) applyFrame(f frame) error {
	if f.Event.Snapshot == nil {
		return errors.New("事件帧缺少任务投影")
	}
	copy, err := cloneTask(f.Event.Snapshot)
	if err != nil {
		return err
	}
	s.tasks[f.Event.TaskID] = copy
	if f.IdempotencyKey != "" {
		s.idempotency[f.Event.TaskID+"\x00"+f.IdempotencyKey] = idempotencyRecord{RequestDigest: f.RequestDigest, Response: append(json.RawMessage(nil), f.Response...)}
	}
	if copy.Credential != nil {
		s.credentialTask[copy.Credential.CredentialNo] = copy.ID
	}
	s.lastSequence, s.lastDigest = f.Sequence, f.Digest
	return nil
}

func (s *Store) recoverFrames() error {
	if _, err := s.logFile.Seek(0, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReader(s.logFile)
	var offset int64
	for {
		start := offset
		var length uint32
		err := binary.Read(reader, binary.BigEndian, &length)
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			if err := s.logFile.Truncate(start); err != nil {
				return err
			}
			break
		}
		if err != nil {
			return err
		}
		offset += 4
		if length == 0 || length > maxFrameSize {
			return fmt.Errorf("事件帧长度 %d 无效，偏移 %d", length, start)
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(reader, payload); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				if err := s.logFile.Truncate(start); err != nil {
					return err
				}
				break
			}
			return err
		}
		offset += int64(length)
		var f frame
		if err := json.Unmarshal(payload, &f); err != nil {
			return fmt.Errorf("事件帧 JSON 损坏，偏移 %d: %w", start, err)
		}
		if f.SchemaVersion != schemaVersion {
			return fmt.Errorf("不支持事件 schemaVersion %d", f.SchemaVersion)
		}
		if f.Sequence != s.lastSequence+1 {
			return fmt.Errorf("事件序号不连续：得到 %d，期望 %d", f.Sequence, s.lastSequence+1)
		}
		if f.PreviousDigest != s.lastDigest {
			return fmt.Errorf("事件前序摘要不匹配，序号 %d", f.Sequence)
		}
		digest, err := checksumFrame(f)
		if err != nil {
			return err
		}
		if digest != f.Digest {
			return fmt.Errorf("事件校验和不匹配，序号 %d", f.Sequence)
		}
		if err := s.applyFrame(f); err != nil {
			return err
		}
	}
	_, err := s.logFile.Seek(0, io.SeekEnd)
	return err
}

func (s *Store) verifyExistingProjection() error {
	raw, err := os.ReadFile(s.snapshotPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var previous projection
	if err := json.Unmarshal(raw, &previous); err != nil {
		return fmt.Errorf("投影快照损坏: %w", err)
	}
	if previous.SchemaVersion != schemaVersion {
		return fmt.Errorf("不支持投影 schemaVersion %d", previous.SchemaVersion)
	}
	if previous.LastSequence > s.lastSequence {
		return errors.New("投影序号超前于事件日志")
	}
	if previous.LastSequence == s.lastSequence && previous.LastDigest != s.lastDigest {
		return errors.New("投影摘要与事件日志不一致")
	}
	return nil
}

func (s *Store) writeProjectionLocked() error {
	p := projection{SchemaVersion: schemaVersion, LastSequence: s.lastSequence, LastDigest: s.lastDigest, Tasks: s.tasks, Idempotency: s.idempotency}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(s.directory, ".projection-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	cleanup := func() { _ = temporary.Close(); _ = os.Remove(temporaryName) }
	if err := temporary.Chmod(0o640); err != nil {
		cleanup()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryName)
		return err
	}
	if err := os.Rename(temporaryName, s.snapshotPath); err != nil {
		_ = os.Remove(temporaryName)
		return err
	}
	directory, err := os.Open(s.directory)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (s *Store) Load(taskID string) (*domain.TrialTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task := s.tasks[taskID]
	if task == nil {
		return nil, domain.NotFound("试配任务", taskID)
	}
	return cloneTask(task)
}

func (s *Store) List() ([]*domain.TrialTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.tasks))
	for id := range s.tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]*domain.TrialTask, 0, len(ids))
	for _, id := range ids {
		copy, err := cloneTask(s.tasks[id])
		if err != nil {
			return nil, err
		}
		result = append(result, copy)
	}
	return result, nil
}

func (s *Store) Replay(taskID, key, requestDigest string) (json.RawMessage, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.idempotency[taskID+"\x00"+key]
	if !ok {
		return nil, false, nil
	}
	if record.RequestDigest != requestDigest {
		return nil, false, domain.Conflict("同一 idempotencyKey 已用于不同请求")
	}
	return append(json.RawMessage(nil), record.Response...), true, nil
}

func (s *Store) Commit(task *domain.TrialTask, expectedVersion int, eventType, actor, key, requestDigest string, response any) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idemKey := task.ID + "\x00" + key
	if prior, ok := s.idempotency[idemKey]; ok {
		if prior.RequestDigest != requestDigest {
			return nil, domain.Conflict("同一 idempotencyKey 已用于不同请求")
		}
		return append(json.RawMessage(nil), prior.Response...), nil
	}
	current := s.tasks[task.ID]
	if expectedVersion == 0 {
		if current != nil {
			return nil, domain.Conflict("试配任务已存在")
		}
	} else if current == nil || current.Version != expectedVersion {
		actual := 0
		if current != nil {
			actual = current.Version
		}
		return nil, domain.Conflict(fmt.Sprintf("版本冲突：当前为 %d，提交期望为 %d", actual, expectedVersion))
	}
	if task.Version != expectedVersion+1 {
		return nil, domain.Conflict("提交后的任务版本必须精确递增 1")
	}
	rawResponse, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	f := frame{SchemaVersion: schemaVersion, Sequence: s.lastSequence + 1, PreviousDigest: s.lastDigest, Event: task.Event(eventType, actor, task.UpdatedAt), IdempotencyKey: key, RequestDigest: requestDigest, Response: rawResponse}
	f.Digest, err = checksumFrame(f)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	if len(payload) > maxFrameSize {
		return nil, errors.New("事件帧超过大小上限")
	}
	if err := binary.Write(s.logFile, binary.BigEndian, uint32(len(payload))); err != nil {
		return nil, err
	}
	if _, err := s.logFile.Write(payload); err != nil {
		return nil, err
	}
	if err := s.logFile.Sync(); err != nil {
		return nil, err
	}
	if err := s.applyFrame(f); err != nil {
		return nil, err
	}
	if err := s.writeProjectionLocked(); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), rawResponse...), nil
}

func (s *Store) NextCredentialSequence() (uint64, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var max uint64
	previous := ""
	for _, task := range s.tasks {
		if task.Credential != nil && task.Credential.Sequence > max {
			max, previous = task.Credential.Sequence, task.Credential.ContentDigest
		}
	}
	return max + 1, previous
}

func (s *Store) FindCredential(number string) (*domain.TrialTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	taskID := s.credentialTask[number]
	if taskID == "" {
		return nil, domain.NotFound("放行凭据", number)
	}
	return cloneTask(s.tasks[taskID])
}

func (s *Store) Integrity() (uint64, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSequence, s.lastDigest
}
