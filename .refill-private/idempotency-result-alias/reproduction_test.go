package idempotency_result_alias_test

import (
	"context"
	"testing"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

func TestIdempotentRetryIsolatedFromReturnedResult(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatalf("打开测试账本失败: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("关闭测试账本失败: %v", err)
		}
	})
	service := workflow.NewService(store)
	command := workflow.CreateTrialCommand{
		WriteMeta:            workflow.WriteMeta{ExpectedVersion: 0, IdempotencyKey: "idem-alias-create-001"},
		ID:                   "TRIAL-IDEMPOTENCY-ALIAS",
		SiteName:             "毗卢寺",
		WallSection:          "东壁中段",
		SubstrateCondition:   "地仗层稳定",
		Owner:                "材料研究员甲",
		AcceptanceThresholds: domain.Thresholds{MaxColorDifference: 3, MaxShrinkagePct: 1, MinBondStrengthMPa: 0.5, MaxPowderingGrade: 1},
	}

	first, err := service.CreateTrial(context.Background(), command)
	if err != nil {
		t.Fatalf("首次建档失败: %v", err)
	}
	first.Task.Owner = "未授权缓存污染者"
	first.Task.Version = 99
	first.Task.Audit[0].Summary = "被调用方改写的审计"
	persisted, err := store.Load(command.ID)
	if err != nil {
		t.Fatalf("读取账本任务失败: %v", err)
	}
	if persisted.Owner != command.Owner || persisted.Version != 1 {
		t.Fatalf("前置条件不成立，调用方修改意外污染了账本: owner=%q version=%d", persisted.Owner, persisted.Version)
	}

	replayed, err := service.CreateTrial(context.Background(), command)
	if err != nil {
		t.Fatalf("幂等重放失败: %v", err)
	}
	if replayed.Task.Owner != command.Owner || replayed.Task.Version != 1 || replayed.Task.Audit[0].Summary == "被调用方改写的审计" {
		t.Fatalf("幂等重放复用了被调用方污染的响应对象: owner=%q version=%d audit=%q", replayed.Task.Owner, replayed.Task.Version, replayed.Task.Audit[0].Summary)
	}
}
