package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/policy"
)

type Service struct {
	store        *ledger.Store
	now          func() time.Time
	locksMu      sync.Mutex
	taskLocks    map[string]*sync.Mutex
	credentialMu sync.Mutex
}

func NewService(store *ledger.Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }, taskLocks: map[string]*sync.Mutex{}}
}

func (s *Service) taskLock(taskID string) *sync.Mutex {
	s.locksMu.Lock()
	defer s.locksMu.Unlock()
	lock := s.taskLocks[taskID]
	if lock == nil {
		lock = &sync.Mutex{}
		s.taskLocks[taskID] = lock
	}
	return lock
}

func validateMeta(meta WriteMeta, creating bool) error {
	if creating && meta.ExpectedVersion != 0 {
		return domain.Invalid("expectedVersion", "创建任务时 expectedVersion 必须为 0")
	}
	if !creating && meta.ExpectedVersion < 1 {
		return domain.Invalid("expectedVersion", "写操作 expectedVersion 必须为正整数")
	}
	key := strings.TrimSpace(meta.IdempotencyKey)
	if len(key) < 8 || len(key) > 128 {
		return domain.Invalid("idempotencyKey", "idempotencyKey 长度必须为 8 到 128 个字符")
	}
	return nil
}

func requestDigest(command any) (string, error) {
	raw, err := json.Marshal(command)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func deterministicID(prefix, seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return prefix + "-" + hex.EncodeToString(sum[:8])
}

func decodeResult(raw json.RawMessage) (*ActionResult, error) {
	var result ActionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func resultFor(task *domain.TrialTask) *ActionResult {
	result := &ActionResult{Task: task}
	var latestRetest time.Time
	for i := range task.Panels {
		panel := &task.Panels[i]
		for j := len(panel.Remediations) - 1; j >= 0; j-- {
			plan := panel.Remediations[j]
			if plan.RetestID == "" {
				continue
			}
			if plan.CompletedAt == nil || plan.CompletedAt.Before(latestRetest) {
				continue
			}
			outcome := &RetestOutcome{Passed: plan.Outcome == "passed", RequiresRemediation: plan.Outcome == "failed", RetestID: plan.RetestID, Round: panel.RetestRound, NewDeviations: []domain.Deviation{}}
			for _, deviation := range panel.Deviations {
				if deviation.SourceRetestID == plan.RetestID {
					outcome.NewDeviations = append(outcome.NewDeviations, deviation)
				}
			}
			result.RetestOutcome = outcome
			latestRetest = *plan.CompletedAt
			break
		}
	}
	for i := range task.Panels {
		panel := &task.Panels[i]
		seen := map[int]bool{}
		for _, m := range panel.Measurements {
			if m.Round == 0 {
				seen[m.CheckpointDay] = true
			}
		}
		for _, day := range panel.ScheduledCheckpoints {
			if !seen[day] {
				next := day
				result.NextCheckpoint = &next
				return result
			}
		}
	}
	return result
}

type createTrialOutcome struct {
	result *ActionResult
	err    error
}

func (s *Service) createTrialLocked(cmd CreateTrialCommand, digest string) (*ActionResult, error) {
	if raw, ok, err := s.store.Replay(cmd.ID, cmd.IdempotencyKey, digest); err != nil {
		return nil, err
	} else if ok {
		return decodeResult(raw)
	}
	task, err := domain.NewTrialTask(cmd.ID, cmd.SiteName, cmd.WallSection, cmd.SubstrateCondition, cmd.Owner, cmd.AcceptanceThresholds, s.now())
	if err != nil {
		return nil, err
	}
	result := resultFor(task)
	raw, err := s.store.Commit(task, 0, "trial_created", cmd.Owner, cmd.IdempotencyKey, digest, result)
	if err != nil {
		return nil, err
	}
	return decodeResult(raw)
}

func (s *Service) CreateTrial(ctx context.Context, cmd CreateTrialCommand) (*ActionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateMeta(cmd.WriteMeta, true); err != nil {
		return nil, err
	}
	if err := policy.ValidateThresholds(cmd.AcceptanceThresholds); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cmd.ID) == "" {
		cmd.ID = deterministicID("TRIAL", cmd.IdempotencyKey)
	}
	digest, err := requestDigest(cmd)
	if err != nil {
		return nil, err
	}
	started := make(chan struct{})
	outcome := make(chan createTrialOutcome, 1)
	go func() {
		lock := s.taskLock(cmd.ID)
		lock.Lock()
		close(started)
		defer lock.Unlock()
		result, err := s.createTrialLocked(cmd, digest)
		outcome <- createTrialOutcome{result: result, err: err}
	}()
	<-started
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case completed := <-outcome:
		return completed.result, completed.err
	}
}

func (s *Service) mutate(ctx context.Context, taskID string, meta WriteMeta, action, actor string, command any, mutate func(*domain.TrialTask) error) (*ActionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskID) == "" {
		return nil, domain.Invalid("taskId", "任务 ID 不能为空")
	}
	if err := validateMeta(meta, false); err != nil {
		return nil, err
	}
	digest, err := requestDigest(command)
	if err != nil {
		return nil, err
	}
	lock := s.taskLock(taskID)
	lock.Lock()
	defer lock.Unlock()
	if raw, ok, err := s.store.Replay(taskID, meta.IdempotencyKey, digest); err != nil {
		return nil, err
	} else if ok {
		return decodeResult(raw)
	}
	task, err := s.store.Load(taskID)
	if err != nil {
		return nil, err
	}
	if task.Version != meta.ExpectedVersion {
		return nil, domain.Conflict(fmt.Sprintf("版本冲突：当前为 %d，提交期望为 %d", task.Version, meta.ExpectedVersion))
	}
	if err := mutate(task); err != nil {
		return nil, err
	}
	result := resultFor(task)
	if task.Credential != nil {
		result.Credential = task.Credential
	}
	raw, err := s.store.Commit(task, meta.ExpectedVersion, action, actor, meta.IdempotencyKey, digest, result)
	if err != nil {
		return nil, err
	}
	return decodeResult(raw)
}

func (s *Service) RegisterPanel(ctx context.Context, taskID string, cmd RegisterPanelCommand) (*ActionResult, error) {
	panels := append([]domain.TestPanel(nil), cmd.Panels...)
	if len(panels) == 0 && (cmd.Panel.PanelCode != "" || cmd.Panel.ID != "" || !cmd.Panel.CuringStartedAt.IsZero() || len(cmd.Panel.ScheduledCheckpoints) > 0) {
		panels = []domain.TestPanel{cmd.Panel}
	}
	formulaID := strings.TrimSpace(cmd.FormulaID)
	newFormula := formulaID == ""
	formulaRewrite := false
	if newFormula {
		if strings.TrimSpace(cmd.Formula.ID) == "" {
			cmd.Formula.ID = deterministicID("FORMULA", cmd.IdempotencyKey)
		} else {
			cmd.Formula.ID = strings.TrimSpace(cmd.Formula.ID)
		}
		formulaID = cmd.Formula.ID
	} else if cmd.Formula.ID != "" || cmd.Formula.TaskID != "" || cmd.Formula.Revision != 0 || len(cmd.Formula.Components) != 0 || cmd.Formula.WaterRatio != 0 || cmd.Formula.MixingMethod != "" || cmd.Formula.PreparedBy != "" || !cmd.Formula.PreparedAt.IsZero() || cmd.Formula.TemperatureC != 0 || cmd.Formula.HumidityPct != 0 {
		formulaRewrite = true
	}
	for i := range panels {
		panels[i].ID = strings.TrimSpace(panels[i].ID)
		if strings.TrimSpace(panels[i].ID) == "" {
			panels[i].ID = deterministicID("PANEL", fmt.Sprintf("%s:%d:%s", cmd.IdempotencyKey, i, strings.ToLower(strings.TrimSpace(panels[i].PanelCode))))
		}
	}
	cmd.Panels, cmd.Panel = panels, domain.TestPanel{}
	cmd.FormulaID = formulaID
	return s.mutate(ctx, taskID, cmd.WriteMeta, "parallel_panels_registered", cmd.Actor, cmd, func(task *domain.TrialTask) error {
		if formulaRewrite {
			return domain.Invalid("formula", "引用既有配方时不得夹带配方内容改写")
		}
		if newFormula {
			if err := policy.ValidateFormula(cmd.Formula); err != nil {
				return err
			}
		}
		if err := policy.ValidatePanelGroup(task, formulaID, !newFormula, panels); err != nil {
			return err
		}
		if newFormula {
			return task.AddPanels(&cmd.Formula, "", panels, cmd.Actor, s.now())
		}
		return task.AddPanels(nil, formulaID, panels, cmd.Actor, s.now())
	})
}

func (s *Service) RecordMeasurement(ctx context.Context, taskID string, cmd RecordMeasurementCommand) (*ActionResult, error) {
	records := append([]MeasurementRecord(nil), cmd.Measurements...)
	if len(records) == 0 && (cmd.PanelID != "" || cmd.Measurement.CheckpointDay != 0 || cmd.Measurement.ID != "") {
		records = []MeasurementRecord{{PanelID: cmd.PanelID, Measurement: cmd.Measurement}}
	}
	actor := "批量检验确认"
	for i := range records {
		records[i].PanelID = strings.TrimSpace(records[i].PanelID)
		records[i].Measurement.ID = strings.TrimSpace(records[i].Measurement.ID)
		if strings.TrimSpace(records[i].Measurement.ID) == "" {
			records[i].Measurement.ID = deterministicID("MEASURE", fmt.Sprintf("%s:%d:%s:%d", cmd.IdempotencyKey, i, records[i].PanelID, records[i].Measurement.CheckpointDay))
		}
		if i == 0 {
			actor = records[i].Measurement.MeasuredBy
		}
	}
	cmd.Measurements, cmd.PanelID, cmd.Measurement = records, "", domain.Measurement{}
	return s.mutate(ctx, taskID, cmd.WriteMeta, "measurements_batch_confirmed", actor, cmd, func(task *domain.TrialTask) error {
		inputs := make([]policy.MeasurementInput, len(records))
		for i, record := range records {
			inputs[i] = policy.MeasurementInput{PanelID: record.PanelID, Measurement: record.Measurement}
		}
		decisions, err := policy.ValidateMeasurementBatch(task, inputs, s.now())
		if err != nil {
			return err
		}
		batch := make([]domain.PanelMeasurement, len(records))
		for i, record := range records {
			batch[i] = domain.PanelMeasurement{PanelID: record.PanelID, Measurement: record.Measurement, Decisions: decisions[i]}
		}
		return task.RecordMeasurements(batch, actor, s.now())
	})
}

func (s *Service) ReviewDeviation(ctx context.Context, taskID string, cmd ReviewDeviationCommand) (*ActionResult, error) {
	return s.mutate(ctx, taskID, cmd.WriteMeta, "deviation_reviewed", cmd.Reviewer, cmd, func(task *domain.TrialTask) error {
		return task.ReviewDeviation(cmd.PanelID, cmd.DeviationID, cmd.Conclusion, cmd.Reviewer, cmd.Note, s.now())
	})
}

func (s *Service) AddRemediation(ctx context.Context, taskID string, cmd RemediationCommand) (*ActionResult, error) {
	if strings.TrimSpace(cmd.Plan.ID) == "" {
		cmd.Plan.ID = deterministicID("PLAN", cmd.IdempotencyKey)
	}
	if err := policy.ValidateRemediationDates(cmd.Plan, s.now()); err != nil {
		return nil, err
	}
	return s.mutate(ctx, taskID, cmd.WriteMeta, "remediation_planned", cmd.Actor, cmd, func(task *domain.TrialTask) error {
		return task.AddRemediation(cmd.PanelID, cmd.Plan, cmd.Actor, s.now())
	})
}

func (s *Service) RecordRetest(ctx context.Context, taskID string, cmd RetestCommand) (*ActionResult, error) {
	if strings.TrimSpace(cmd.Measurement.ID) == "" {
		cmd.Measurement.ID = deterministicID("RETEST", cmd.IdempotencyKey)
	}
	return s.mutate(ctx, taskID, cmd.WriteMeta, "retest_confirmed", cmd.Measurement.MeasuredBy, cmd, func(task *domain.TrialTask) error {
		decisions, err := policy.EvaluateMeasurement(task.AcceptanceThresholds, cmd.Measurement)
		if err != nil {
			return err
		}
		return task.RecordRetest(cmd.PanelID, cmd.PlanID, cmd.Measurement, decisions, cmd.Measurement.MeasuredBy, s.now())
	})
}

func (s *Service) Freeze(ctx context.Context, taskID string, cmd FreezeCommand) (*ActionResult, error) {
	cmd.FormulaID = strings.TrimSpace(cmd.FormulaID)
	cmd.PanelID = strings.TrimSpace(cmd.PanelID)
	cmd.ApprovedBy = strings.TrimSpace(cmd.ApprovedBy)
	cmd.EligibilityDigest = strings.TrimSpace(cmd.EligibilityDigest)
	return s.mutate(ctx, taskID, cmd.WriteMeta, "batch_frozen", cmd.ApprovedBy, cmd, func(task *domain.TrialTask) error {
		report := policy.CheckReleaseEligibility(task, cmd.FormulaID, cmd.PanelID, cmd.ApprovedBy)
		if !report.Eligible {
			return domain.Illegal("冻结资格检查失败：" + strings.Join(report.Problems, "；"))
		}
		if strings.TrimSpace(cmd.EligibilityDigest) == "" || cmd.EligibilityDigest != report.Digest {
			return domain.Conflict("冻结预检摘要已过期，请刷新候选矩阵后重试")
		}
		return task.Freeze(cmd.FormulaID, cmd.PanelID, cmd.ApprovedBy, report.Digest, report.Checks, s.now())
	})
}

func (s *Service) Release(ctx context.Context, taskID string, cmd ReleaseCommand) (*ActionResult, error) {
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()
	sequence, previous := s.store.NextCredentialSequence()
	return s.mutate(ctx, taskID, cmd.WriteMeta, "credential_issued", cmd.ApprovedBy, cmd, func(task *domain.TrialTask) error {
		_, err := task.IssueCredential(sequence, previous, cmd.ApprovedBy, s.now())
		return err
	})
}

func (s *Service) GetTrial(ctx context.Context, taskID string) (*domain.TrialTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.store.Load(taskID)
}

func (s *Service) ListTrials(ctx context.Context) ([]*domain.TrialTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.store.List()
}

func (s *Service) VerifyCredential(ctx context.Context, number string) (*CredentialView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	task, err := s.store.FindCredential(number)
	if err != nil {
		return nil, err
	}
	recomputed, err := domain.CredentialDigest(task.Credential)
	if err != nil {
		return nil, err
	}
	sequence, digest := s.store.Integrity()
	return &CredentialView{Credential: task.Credential, DigestValid: recomputed == task.Credential.ContentDigest, RecomputedDigest: recomputed, TaskStatus: task.Status, Audit: task.StableAudit(), LedgerSequence: sequence, LedgerDigest: digest, CheckedAt: s.now()}, nil
}
