package projection_content_trust_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

func TestProjectionTamperingCannotOverrideEventReplay(t *testing.T) {
	directory := t.TempDir()
	store, err := ledger.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	service := workflow.NewService(store)
	const taskID = "PROJECTION-TRUST-1"
	const originalOwner = "壁画保护负责人"
	_, err = service.CreateTrial(context.Background(), workflow.CreateTrialCommand{
		WriteMeta:          workflow.WriteMeta{IdempotencyKey: "projection-create-0001"},
		ID:                 taskID,
		SiteName:           "正殿壁画",
		WallSection:        "东壁中段",
		SubstrateCondition: "夿土地仗层",
		Owner:              originalOwner,
		AcceptanceThresholds: domain.Thresholds{
			MaxColorDifference: 2,
			MaxShrinkagePct:    1,
			MinBondStrengthMPa: 0.3,
			MaxPowderingGrade:  1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	projectionPath := filepath.Join(directory, "projection.json")
	raw, err := os.ReadFile(projectionPath)
	if err != nil {
		t.Fatal(err)
	}
	var projection map[string]any
	if err := json.Unmarshal(raw, &projection); err != nil {
		t.Fatal(err)
	}
	tasks := projection["tasks"].(map[string]any)
	task := tasks[taskID].(map[string]any)
	task["owner"] = "未经事件授权的篡改者"
	tampered, err := json.MarshalIndent(projection, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectionPath, tampered, 0o640); err != nil {
		t.Fatal(err)
	}

	reopened, err := ledger.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err := workflow.NewService(reopened).GetTrial(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Owner != originalOwner {
		t.Fatalf("投影内容覆盖了事件重放结果: owner=%q", recovered.Owner)
	}
}
