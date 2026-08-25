package workflow

import (
	"context"
	"testing"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
)

func TestBoundedSelfcheck(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	view, err := RunBoundedSelfcheck(context.Background(), NewService(store))
	if err != nil {
		t.Fatal(err)
	}
	if !view.DigestValid || len(view.Audit) < 8 {
		t.Fatalf("自检轨迹不完整: %+v", view)
	}
}
