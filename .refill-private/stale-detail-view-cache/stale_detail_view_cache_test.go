package stale_detail_view_cache_test

import (
	"context"
	"testing"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

func TestDetailViewCacheReflectsCommittedMutation(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := workflow.NewService(store)
	created, err := service.CreateTrial(context.Background(), workflow.CreateTrialCommand{
		WriteMeta: workflow.WriteMeta{ExpectedVersion: 0, IdempotencyKey: "cache-create-0001"},
		ID:        "CACHE-TRIAL", SiteName: "东壁", WallSection: "画心", SubstrateCondition: "夯土基底", Owner: "材料研究员",
		AcceptanceThresholds: domain.Thresholds{MaxColorDifference: 2, MaxShrinkagePct: 1, MinBondStrengthMPa: 0.3, MaxPowderingGrade: 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	cached, err := service.GetTrialView(context.Background(), created.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cached.Version != 1 || cached.Status != domain.StatusDraft {
		t.Fatalf("unexpected initial view: version=%d status=%s", cached.Version, cached.Status)
	}

	prepared := time.Now().UTC().AddDate(0, 0, -8)
	committed, err := service.RegisterPanel(context.Background(), created.Task.ID, workflow.RegisterPanelCommand{
		WriteMeta: workflow.WriteMeta{ExpectedVersion: created.Task.Version, IdempotencyKey: "cache-panel-00001"},
		Formula: domain.MortarFormula{
			ID: "CACHE-F-1", Revision: 1,
			Components: []domain.FormulaComponent{{Name: "熟石灰", Percentage: 70, BatchRef: "L-1"}, {Name: "细砂", Percentage: 30, BatchRef: "S-1"}},
			WaterRatio: 0.42, MixingMethod: "低速湿拌", PreparedBy: "材料研究员", PreparedAt: prepared, TemperatureC: 21, HumidityPct: 55,
		},
		Panel: domain.TestPanel{ID: "CACHE-P-1", PanelCode: "CACHE-P01", CuringStartedAt: prepared, ScheduledCheckpoints: []int{7}},
		Actor: "材料研究员",
	})
	if err != nil {
		t.Fatal(err)
	}
	if committed.Task.Version != 2 || committed.Task.Status != domain.StatusCuring {
		t.Fatalf("mutation was not committed: version=%d status=%s", committed.Task.Version, committed.Task.Status)
	}

	view, err := service.GetTrialView(context.Background(), created.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Version != committed.Task.Version || view.Status != committed.Task.Status || len(view.Panels) != 1 {
		t.Fatalf("committed mutation is hidden by stale detail cache: got version=%d status=%s panels=%d, want version=%d status=%s panels=1", view.Version, view.Status, len(view.Panels), committed.Task.Version, committed.Task.Status)
	}
}
