package list_snapshot_alias_test

import (
	"context"
	"testing"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

func TestListResultMutationDoesNotCorruptLedgerState(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatalf("打开账本失败: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	createdAt := time.Date(2026, time.August, 25, 8, 0, 0, 0, time.UTC)
	task, err := domain.NewTrialTask(
		"TRIAL-LIST-ALIAS",
		"东壁壁画",
		"下层修复区",
		"夯土基底",
		"材料研究员",
		domain.Thresholds{
			MaxColorDifference: 2,
			MaxShrinkagePct:    1,
			MinBondStrengthMPa: 0.3,
			MaxPowderingGrade:  1,
		},
		createdAt,
	)
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if _, err := store.Commit(task, 0, "trial_created", task.Owner, "list-alias-create", "digest-list-alias", map[string]string{"taskId": task.ID}); err != nil {
		t.Fatalf("提交任务失败: %v", err)
	}
	service := workflow.NewService(store)

	listed, err := service.ListTrials(context.Background())
	if err != nil {
		t.Fatalf("列出任务失败: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("期望一个任务，实际为 %d", len(listed))
	}

	// 列表结果属于调用方；修改响应模型不得绕过 Commit 改写仓储状态。
	listed[0].Owner = "越权修改者"
	listed[0].Status = domain.StatusReleased
	listed[0].Version = 99
	listed[0].Audit[0].Summary = "伪造审计记录"

	reloaded, err := service.GetTrial(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("重新加载任务失败: %v", err)
	}
	if reloaded.Owner != "材料研究员" || reloaded.Status != domain.StatusDraft || reloaded.Version != 1 || reloaded.Audit[0].Summary != "创建灰浆试配任务草案" {
		t.Fatalf("List 返回值污染了未提交的账本状态: owner=%q status=%q version=%d audit=%q", reloaded.Owner, reloaded.Status, reloaded.Version, reloaded.Audit[0].Summary)
	}
}
