package setup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rmukubvu/preflight/internal/config"
)

// handleGetConfig returns the current in-memory config as JSON.
// The browser uses this on load to pre-populate the form.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.cfg.Config); err != nil {
		http.Error(w, "encoding config: "+err.Error(), http.StatusInternalServerError)
	}
}

// handleSaveConfig validates the submitted config, writes it to disk,
// and triggers a graceful server shutdown so the CLI process exits cleanly.
func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var incoming config.Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if errs := config.Validate(incoming); len(errs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{"errors": errs})
		return
	}

	if err := config.Save(s.cfg.WorkDir, incoming); err != nil {
		http.Error(w, fmt.Sprintf("saving config: %v", err), http.StatusInternalServerError)
		return
	}

	// Update the in-memory copy so subsequent GET /api/config reflects the save.
	s.cfg.Config = incoming

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})

	// Trigger graceful shutdown after the response is flushed to the client.
	go func() {
		time.Sleep(200 * time.Millisecond)
		if s.shutdown != nil {
			s.shutdown()
		}
	}()
}
