package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
)

func TestCommitReplayAndTailRecovery(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	task, _ := domain.NewTrialTask("T-1", "殿堂", "东壁", "土坯", "负责人", domain.Thresholds{MaxColorDifference: 2, MaxShrinkagePct: 1, MinBondStrengthMPa: .2, MaxPowderingGrade: 1}, time.Now())
	raw, err := store.Commit(task, 0, "trial_created", "负责人", "create-key-0001", "digest-a", map[string]string{"id": task.ID})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil || got["id"] != "T-1" {
		t.Fatalf("响应异常: %s %v", raw, err)
	}
	replayed, ok, err := store.Replay("T-1", "create-key-0001", "digest-a")
	if err != nil || !ok || string(replayed) != string(raw) {
		t.Fatalf("幂等重放失败: %v %v", ok, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(dir, "events.frames"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte{0, 0})
	_ = file.Close()
	recovered, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if _, err := recovered.Load("T-1"); err != nil {
		t.Fatal(err)
	}
}
