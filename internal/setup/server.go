// Package setup implements the `preflight setup` command, which serves a
// browser-based one-pager for configuring LLM providers and API keys.
package setup

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/rmukubvu/preflight/internal/config"
)

//go:embed web
var webFS embed.FS

// ServerConfig holds construction-time options for the setup server.
type ServerConfig struct {
	Port    int
	WorkDir string
	Config  config.Config
}

// Server is the embedded HTTP server that serves the setup one-pager.
type Server struct {
	cfg      ServerConfig
	mux      *http.ServeMux
	shutdown context.CancelFunc // triggers graceful shutdown after save
}

// NewServer constructs a Server. It does not start listening.
func NewServer(cfg ServerConfig) *Server {
	s := &Server{cfg: cfg}
	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s
}

// ServeHTTP implements http.Handler, enabling use with httptest in tests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Run binds to the configured port, prints the URL, opens a browser,
// and blocks until the user saves the config or ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.shutdown = cancel
	defer cancel()

	addr := fmt.Sprintf("127.0.0.1:%d", s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("binding to %s: %w", addr, err)
	}

	url := fmt.Sprintf("http://%s", ln.Addr())
	fmt.Printf("\npreflight setup\n")
	fmt.Printf("  Running at %s\n\n", url)

	httpSrv := &http.Server{
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	servErr := make(chan error, 1)
	go func() {
		servErr <- httpSrv.Serve(ln)
	}()

	go func() {
		time.Sleep(150 * time.Millisecond)
		if !openBrowser(url) {
			fmt.Printf("  Open the URL above in your browser to continue.\n\n")
		}
	}()

	select {
	case <-ctx.Done():
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutCancel()
		_ = httpSrv.Shutdown(shutCtx)
		return nil
	case err := <-servErr:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (s *Server) registerRoutes() {
	// Serve the embedded web/ directory at the root.
	stripped, err := fs.Sub(webFS, "web")
	if err != nil {
		// This can only fail if the embed path is wrong — a programmer error.
		panic("setup: embedded web FS misconfigured: " + err.Error())
	}
	s.mux.Handle("/", http.FileServer(http.FS(stripped)))

	// API endpoints use Go 1.22+ method-prefixed patterns.
	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("POST /api/save", s.handleSaveConfig)
}
