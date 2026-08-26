package credentialsequencefork_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

type releaseBarrierContext struct {
	context.Context
	once    sync.Once
	entered chan<- struct{}
	release <-chan struct{}
}

func (c *releaseBarrierContext) Err() error {
	c.once.Do(func() {
		c.entered <- struct{}{}
		<-c.release
	})
	return nil
}

func frozenTask(id string, now time.Time) *domain.TrialTask {
	return &domain.TrialTask{
		ID:          id,
		SiteName:    "永乐宫壁画",
		WallSection: "东壁",
		Owner:       "材料研究员",
		Status:      domain.StatusFrozen,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
		FrozenBatch: &domain.BatchSnapshot{
			TaskID:      id,
			ApprovedBy:  "施工负责人",
			ApprovedAt:  now,
			TaskVersion: 1,
		},
	}
}

func persistFrozenTask(t *testing.T, store *ledger.Store, task *domain.TrialTask, key string) {
	t.Helper()
	if _, err := store.Commit(task, 0, "batch_frozen", "施工负责人", key, "setup-"+task.ID, map[string]string{"taskId": task.ID}); err != nil {
		t.Fatalf("写入冻结任务失败: %v", err)
	}
}

func TestConcurrentServicesAllocateUniqueCredentialSequence(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	persistFrozenTask(t, store, frozenTask("FROZEN-A", now), "setup-frozen-a")
	persistFrozenTask(t, store, frozenTask("FROZEN-B", now), "setup-frozen-b")

	serviceA := workflow.NewService(store)
	serviceB := workflow.NewService(store)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	type outcome struct {
		result *workflow.ActionResult
		err    error
	}
	results := make(chan outcome, 2)

	start := func(service *workflow.Service, taskID, key string) {
		ctx := &releaseBarrierContext{Context: context.Background(), entered: entered, release: release}
		result, callErr := service.Release(ctx, taskID, workflow.ReleaseCommand{
			WriteMeta:  workflow.WriteMeta{ExpectedVersion: 1, IdempotencyKey: key},
			ApprovedBy: "施工负责人",
		})
		results <- outcome{result: result, err: callErr}
	}
	go start(serviceA, "FROZEN-A", "release-frozen-a")
	go start(serviceB, "FROZEN-B", "release-frozen-b")

	<-entered
	<-entered
	close(release)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("并发签发未同时成功: first=%v second=%v", first.err, second.err)
	}
	if first.result.Credential.Sequence == second.result.Credential.Sequence || first.result.Credential.CredentialNo == second.result.Credential.CredentialNo {
		t.Fatalf("两个已确认凭据复用了序号和编号: first=%s/%d second=%s/%d",
			first.result.Credential.CredentialNo, first.result.Credential.Sequence,
			second.result.Credential.CredentialNo, second.result.Credential.Sequence)
	}
	if second.result.Credential.PreviousDigest != first.result.Credential.ContentDigest && first.result.Credential.PreviousDigest != second.result.Credential.ContentDigest {
		t.Fatal("两个凭据未形成单一的 previousDigest 链")
	}
}
