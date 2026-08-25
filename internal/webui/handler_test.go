package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

func TestWorkbenchAndHealth(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewServer(workflow.NewService(store), nil)
	for _, route := range []string{"/workbench", "/healthz"} {
		request := httptest.NewRequest(http.MethodGet, route, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s 返回 %d", route, response.Code)
		}
	}
}
