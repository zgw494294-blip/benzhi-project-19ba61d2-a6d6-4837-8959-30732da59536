package webui

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/workflow"
)

type Server struct {
	service       *workflow.Service
	assets        map[string]asset
	selfcheck     func() error
	selfcheckOnly bool
}

func NewServer(service *workflow.Service, selfcheck func() error) http.Handler {
	s := &Server{service: service, assets: loadAssets(), selfcheck: selfcheck, selfcheckOnly: selfcheck != nil}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.HealthHandler)
	mux.HandleFunc("GET /", s.RootHandler)
	mux.HandleFunc("GET /workbench", s.WorkbenchHandler)
	mux.HandleFunc("GET /assets/{name}", s.AssetHandler)
	mux.HandleFunc("GET /api/v1/trials", s.ListTrialsHandler)
	mux.HandleFunc("POST /api/v1/trials", s.CreateTrialHandler)
	mux.HandleFunc("GET /api/v1/trials/{id}", s.TrialHandler)
	mux.HandleFunc("POST /api/v1/trials/{id}/panels", s.RegisterPanelHandler)
	mux.HandleFunc("POST /api/v1/trials/{id}/measurements", s.MeasurementHandler)
	mux.HandleFunc("POST /api/v1/trials/{id}/reviews", s.ReviewHandler)
	mux.HandleFunc("POST /api/v1/trials/{id}/remediations", s.RemediationHandler)
	mux.HandleFunc("POST /api/v1/trials/{id}/retests", s.RetestHandler)
	mux.HandleFunc("POST /api/v1/trials/{id}/freeze", s.FreezeHandler)
	mux.HandleFunc("POST /api/v1/trials/{id}/release", s.ReleaseHandler)
	mux.HandleFunc("GET /api/v1/credentials/{number}", s.CredentialHandler)
	if s.selfcheckOnly {
		mux.HandleFunc("POST /_selfcheck", s.SelfcheckHandler)
	}
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; style-src 'self'; script-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	item, ok := s.assets[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", item.contentType)
	w.Header().Set("ETag", item.etag)
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("If-None-Match") == item.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(item.body)
}

func (s *Server) RootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/workbench", http.StatusTemporaryRedirect)
}

func (s *Server) WorkbenchHandler(w http.ResponseWriter, r *http.Request) {
	s.serveAsset(w, r, "index.html")
}

func (s *Server) AssetHandler(w http.ResponseWriter, r *http.Request) {
	name := path.Base(r.PathValue("name"))
	if name != "app.css" && name != "extensions.css" && name != "app.js" {
		http.NotFound(w, r)
		return
	}
	s.serveAsset(w, r, name)
}

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]string{"status": "ok", "service": "MuralMortarGate"})
}

func (s *Server) ListTrialsHandler(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.service.ListTrials(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tasks)
}

func (s *Server) CreateTrialHandler(w http.ResponseWriter, r *http.Request) {
	var cmd workflow.CreateTrialCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.service.CreateTrial(context.WithoutCancel(r.Context()), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (s *Server) TrialHandler(w http.ResponseWriter, r *http.Request) {
	task, err := s.service.GetTrialViewForApprover(r.Context(), r.PathValue("id"), r.URL.Query().Get("approvedBy"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, task)
}

func (s *Server) RegisterPanelHandler(w http.ResponseWriter, r *http.Request) {
	var cmd workflow.RegisterPanelCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.service.RegisterPanel(context.WithoutCancel(r.Context()), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (s *Server) MeasurementHandler(w http.ResponseWriter, r *http.Request) {
	var cmd workflow.RecordMeasurementCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.service.RecordMeasurement(context.WithoutCancel(r.Context()), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (s *Server) ReviewHandler(w http.ResponseWriter, r *http.Request) {
	var cmd workflow.ReviewDeviationCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.service.ReviewDeviation(context.WithoutCancel(r.Context()), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (s *Server) RemediationHandler(w http.ResponseWriter, r *http.Request) {
	var cmd workflow.RemediationCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.service.AddRemediation(context.WithoutCancel(r.Context()), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (s *Server) RetestHandler(w http.ResponseWriter, r *http.Request) {
	var cmd workflow.RetestCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.service.RecordRetest(context.WithoutCancel(r.Context()), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (s *Server) FreezeHandler(w http.ResponseWriter, r *http.Request) {
	var cmd workflow.FreezeCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.service.Freeze(context.WithoutCancel(r.Context()), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (s *Server) ReleaseHandler(w http.ResponseWriter, r *http.Request) {
	var cmd workflow.ReleaseCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.service.Release(context.WithoutCancel(r.Context()), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (s *Server) CredentialHandler(w http.ResponseWriter, r *http.Request) {
	view, err := s.service.VerifyCredential(r.Context(), r.PathValue("number"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, view)
}

func (s *Server) SelfcheckHandler(w http.ResponseWriter, r *http.Request) {
	if s.selfcheck == nil {
		http.NotFound(w, r)
		return
	}
	if err := s.selfcheck(); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"completed": true})
}

var errUnsupportedMedia = errors.New("Content-Type 必须为 application/json")

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	media := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if media != "application/json" {
		return errUnsupportedMedia
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := jsonDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domainInvalidJSON(err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return domainInvalidJSON(errors.New("请求体只能包含一个 JSON 对象"))
		}
		return domainInvalidJSON(err)
	}
	return nil
}
