package remediation_replay_expiry_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

func TestExpiredRemediationRetryReplaysCommittedResponse(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	base := &domain.TrialTask{ID: "REPLAY-PLAN", SiteName: "壁画", WallSection: "东壁", SubstrateCondition: "夯土", Owner: "负责人", Status: domain.StatusRemediation, Version: 1, CreatedAt: yesterday, UpdatedAt: yesterday, Formulas: []domain.MortarFormula{}, Panels: []domain.TestPanel{}, Audit: []domain.AuditEntry{}}
	if _, err := store.Commit(base, 0, "fixture_created", "负责人", "", "", &workflow.ActionResult{Task: base}); err != nil {
		t.Fatal(err)
	}

	cmd := workflow.RemediationCommand{
		WriteMeta: workflow.WriteMeta{ExpectedVersion: 1, IdempotencyKey: "replay-expired-plan"},
		PanelID: "P-1",
		Plan: domain.RemediationPlan{ID: "PLAN-1", DeviationIDs: []string{"DEV-1"}, Action: "调整材料", DueDate: yesterday.Format("2006-01-02"), Responsible: "研究员"},
		Actor: "保护人",
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	committed := *base
	committed.Version = 2
	committed.UpdatedAt = yesterday
	committed.Panels = []domain.TestPanel{{ID: "P-1", Remediations: []domain.RemediationPlan{cmd.Plan}}}
	wanted := &workflow.ActionResult{Task: &committed}
	if _, err := store.Commit(&committed, 1, "remediation_planned", cmd.Actor, cmd.IdempotencyKey, digest, wanted); err != nil {
		t.Fatal(err)
	}

	replayed, err := workflow.NewService(store).AddRemediation(context.Background(), base.ID, cmd)
	if err != nil {
		t.Fatalf("已提交请求在跨日期重试时没有命中幂等响应: %v", err)
	}
	if replayed.Task == nil || replayed.Task.Version != 2 {
		t.Fatalf("重放响应异常: %+v", replayed)
	}
}
