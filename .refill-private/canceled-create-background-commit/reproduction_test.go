package canceled_create_background_commit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

type blockingResponse struct {
	entered chan struct{}
	release chan struct{}
}

func (r blockingResponse) MarshalJSON() ([]byte, error) {
	close(r.entered)
	<-r.release
	return []byte(`{"blocked":true}`), nil
}

type observedContext struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (c *observedContext) Err() error {
	c.once.Do(func() { close(c.checked) })
	return c.Context.Err()
}

func validThresholds() domain.Thresholds {
	return domain.Thresholds{MaxColorDifference: 2, MaxShrinkagePct: 1, MinBondStrengthMPa: 0.3, MaxPowderingGrade: 1}
}

func createCommand(key string) workflow.CreateTrialCommand {
	return workflow.CreateTrialCommand{
		WriteMeta:            workflow.WriteMeta{IdempotencyKey: key},
		ID:                   "CANCEL-CREATE-1",
		SiteName:             "永安寺壁画",
		WallSection:          "东壁下层",
		SubstrateCondition:   "夯土基底",
		Owner:                "材料研究员",
		AcceptanceThresholds: validThresholds(),
	}
}

func TestCanceledCreateDoesNotCommitInBackground(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := workflow.NewService(store)

	blocker, err := domain.NewTrialTask("STORE-BLOCKER", "壁画", "西壁", "砖石基底", "测试员", validThresholds(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	blockerDone := make(chan error, 1)
	go func() {
		_, commitErr := store.Commit(blocker, 0, "trial_created", "测试员", "blocker-create-0001", "blocker-digest", blockingResponse{entered: entered, release: release})
		blockerDone <- commitErr
	}()
	<-entered

	base, cancel := context.WithCancel(context.Background())
	ctx := &observedContext{Context: base, checked: make(chan struct{})}
	canceledDone := make(chan error, 1)
	go func() {
		_, createErr := service.CreateTrial(ctx, createCommand("canceled-create-0001"))
		canceledDone <- createErr
	}()
	<-ctx.checked
	cancel()
	if err := <-canceledDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller did not receive context cancellation: %v", err)
	}

	close(release)
	if err := <-blockerDone; err != nil {
		t.Fatalf("failed to release controlled store barrier: %v", err)
	}

	created, err := service.CreateTrial(context.Background(), createCommand("replacement-create-0001"))
	if err != nil {
		t.Fatalf("replacement create was rejected because canceled request committed in background: %v", err)
	}
	if created.Task.Version != 1 {
		t.Fatalf("replacement create returned unexpected version: %d", created.Task.Version)
	}
}
