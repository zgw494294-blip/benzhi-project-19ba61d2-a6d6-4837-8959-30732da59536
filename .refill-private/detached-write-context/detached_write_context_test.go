package detached_write_context_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/webui"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

type gatedBody struct {
	payload []byte
	started chan struct{}
	release chan struct{}
	once    sync.Once
	offset  int
}

func (b *gatedBody) Read(p []byte) (int, error) {
	b.once.Do(func() {
		close(b.started)
		<-b.release
	})
	if b.offset == len(b.payload) {
		return 0, io.EOF
	}
	n := copy(p, b.payload[b.offset:])
	b.offset += n
	return n, nil
}

func (b *gatedBody) Close() error { return nil }

func TestCanceledPanelRequestDoesNotPersist(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := workflow.NewService(store)

	created, err := service.CreateTrial(context.Background(), workflow.CreateTrialCommand{
		WriteMeta:            workflow.WriteMeta{IdempotencyKey: "create-context-case"},
		ID:                   "CTX-PANEL",
		SiteName:             "北壁壁画",
		WallSection:          "北壁下层",
		SubstrateCondition:   "夯土基层",
		Owner:                "材料负责人",
		AcceptanceThresholds: domain.Thresholds{MaxColorDifference: 2, MaxShrinkagePct: 1, MinBondStrengthMPa: 0.3, MaxPowderingGrade: 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	preparedAt := time.Now().UTC().Add(-time.Hour)
	command := workflow.RegisterPanelCommand{
		WriteMeta: workflow.WriteMeta{ExpectedVersion: created.Task.Version, IdempotencyKey: "panel-context-case"},
		Formula: domain.MortarFormula{
			ID: "FORMULA-CONTEXT", Revision: 1,
			Components:   []domain.FormulaComponent{{Name: "石灰", Percentage: 70, BatchRef: "L-1"}, {Name: "砂", Percentage: 30, BatchRef: "S-1"}},
			WaterRatio:   0.4,
			MixingMethod: "低速湿拌",
			PreparedBy:   "研究员",
			PreparedAt:   preparedAt,
			TemperatureC: 20,
			HumidityPct:  55,
		},
		Panel: domain.TestPanel{ID: "PANEL-CONTEXT", PanelCode: "CTX-01", CuringStartedAt: preparedAt, ScheduledCheckpoints: []int{7}},
		Actor: "研究员",
	}
	payload, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	body := &gatedBody{payload: payload, started: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/trials/CTX-PANEL/panels", body).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		webui.NewServer(service, nil).ServeHTTP(recorder, request)
		close(done)
	}()

	<-body.started
	cancel()
	close(body.release)
	<-done

	after, err := service.GetTrial(context.Background(), created.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != created.Task.Version || len(after.Panels) != 0 {
		t.Fatalf("已取消的登记请求仍被持久化: status=%d version=%d panels=%d", recorder.Code, after.Version, len(after.Panels))
	}
}
