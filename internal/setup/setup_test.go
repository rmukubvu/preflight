package setup_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rmukubvu/preflight/internal/config"
	"github.com/rmukubvu/preflight/internal/setup"
)

// newTestServer creates a Server backed by a temp directory.
func newTestServer(t *testing.T) (*setup.Server, string) {
	t.Helper()
	dir := t.TempDir()
	srv := setup.NewServer(setup.ServerConfig{
		Port:    0,
		WorkDir: dir,
		Config:  config.DefaultConfig(),
	})
	return srv, dir
}

// ── GET /api/config ──────────────────────────────────────────────────────────

func TestHandleGetConfig_ReturnsCurrentConfig(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	var got config.Config
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if got.LLM.Provider != "auto" {
		t.Errorf("want provider auto, got %q", got.LLM.Provider)
	}
	if got.Floci.Port != 4566 {
		t.Errorf("want floci port 4566, got %d", got.Floci.Port)
	}
}

func TestHandleGetConfig_ContentTypeJSON(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("want Content-Type application/json, got %q", ct)
	}
}

// ── POST /api/save ───────────────────────────────────────────────────────────

func TestHandleSaveConfig_ValidConfig_WritesFile(t *testing.T) {
	srv, dir := newTestServer(t)

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "none"
	cfg.Stack.Type = "cdk"
	cfg.Stack.Dir = "./infra"
	cfg.Assertions.Behavioural.HTTP = []config.HTTPCheckConfig{
		{
			API:            "JobsApi",
			Method:         "POST",
			Path:           "/jobs",
			ExpectedStatus: 202,
			Body:           `{"id":"job-123"}`,
		},
	}
	cfg.Assertions.Behavioural.SQSToLambdaToDynamo = []config.SQSToLambdaToDynamoDBConfig{
		{
			Queue:       "JobQueue",
			Table:       "JobsTable",
			MessageBody: `{"id":"job-123"}`,
			ExpectedKey: map[string]string{"id": "job-123"},
		},
	}

	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the file was written to disk.
	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("loading saved config: %v", err)
	}
	if loaded.LLM.Provider != "none" {
		t.Errorf("want saved provider none, got %q", loaded.LLM.Provider)
	}
	if loaded.Stack.Type != "cdk" {
		t.Errorf("want stack.type cdk, got %q", loaded.Stack.Type)
	}
	if len(loaded.Assertions.Behavioural.HTTP) != 1 {
		t.Fatalf("want 1 saved HTTP behavioural assertion, got %d", len(loaded.Assertions.Behavioural.HTTP))
	}
	if len(loaded.Assertions.Behavioural.SQSToLambdaToDynamo) != 1 {
		t.Fatalf("want 1 saved SQS behavioural assertion, got %d", len(loaded.Assertions.Behavioural.SQSToLambdaToDynamo))
	}
}

func TestHandleSaveConfig_ValidConfig_ReturnsSavedStatus(t *testing.T) {
	srv, _ := newTestServer(t)

	body, _ := json.Marshal(config.DefaultConfig())
	req := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp["status"] != "saved" {
		t.Errorf("want status saved, got %q", resp["status"])
	}
}

func TestHandleSaveConfig_InvalidProvider_Returns422(t *testing.T) {
	srv, _ := newTestServer(t)

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "not-a-real-provider"

	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding error response: %v", err)
	}
	if _, ok := resp["errors"]; !ok {
		t.Error("want errors key in response body")
	}
}

func TestHandleSaveConfig_MissingAPIKeyForClaude_Returns422(t *testing.T) {
	srv, _ := newTestServer(t)

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "claude"
	cfg.LLM.Claude.APIKey = "" // missing

	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", w.Code)
	}
}

func TestHandleSaveConfig_InvalidBehaviouralAssertion_Returns422(t *testing.T) {
	srv, _ := newTestServer(t)

	cfg := config.DefaultConfig()
	cfg.Assertions.Behavioural.HTTP = []config.HTTPCheckConfig{
		{
			Method:         "GET",
			Path:           "/health",
			ExpectedStatus: 200,
		},
	}

	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", w.Code)
	}
}

func TestHandleSaveConfig_MalformedJSON_Returns400(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/save",
		bytes.NewReader([]byte(`{not valid json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ── GET / (static assets) ────────────────────────────────────────────────────

func TestServeIndex_Returns200(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct == "" {
		t.Error("want Content-Type header, got empty")
	}
}

// ── After save, GET /api/config reflects updated state ───────────────────────

func TestHandleSaveConfig_UpdatesInMemoryConfig(t *testing.T) {
	srv, dir := newTestServer(t)
	_ = dir // silence unused warning

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "ollama"
	cfg.LLM.Ollama.BaseURL = "http://localhost:11434"
	cfg.LLM.Ollama.Model = "llama3"

	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(httptest.NewRecorder(), req)

	// Now GET /api/config should reflect the updated provider.
	req2 := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	var got config.Config
	if err := json.NewDecoder(w2.Body).Decode(&got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got.LLM.Provider != "ollama" {
		t.Errorf("want provider ollama after save, got %q", got.LLM.Provider)
	}
}

// ── WorkDir isolation ─────────────────────────────────────────────────────────

func TestHandleSaveConfig_WritesToWorkDir(t *testing.T) {
	dir := t.TempDir()
	otherDir := t.TempDir()

	srv := setup.NewServer(setup.ServerConfig{
		Port:    0,
		WorkDir: dir,
		Config:  config.DefaultConfig(),
	})

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "none"
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(httptest.NewRecorder(), req)

	// File should exist in dir.
	if _, err := os.Stat(filepath.Join(dir, config.Filename)); err != nil {
		t.Errorf("want config in workdir, got: %v", err)
	}

	// File should NOT exist in otherDir.
	if _, err := os.Stat(filepath.Join(otherDir, config.Filename)); err == nil {
		t.Error("config should not exist in unrelated directory")
	}
}
