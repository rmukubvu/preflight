package emulator

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rmukubvu/preflight/internal/config"
)

const (
	flociContainerName = "preflight-floci"
	dockerSocketPath   = "/var/run/docker.sock"
	flociNetworkName   = "preflight-floci-network"
	healthTimeout      = 30 * time.Second
	healthPollInterval = 250 * time.Millisecond
)

// Manager starts, reuses, and probes a local emulator runtime.
type Manager struct {
	cfg         config.EmulatorConfig
	endpoint    string
	displayName string
	stopOnExit  bool

	mu          sync.Mutex
	cmd         *exec.Cmd
	exitCh      chan error
	tempDataDir string
}

func NewManager(cfg config.EmulatorConfig) *Manager {
	normalized := config.Normalize(config.Config{Emulator: cfg}).Emulator
	endpoint := normalized.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("http://localhost:%d", normalized.Port)
	}

	return &Manager{
		cfg:         normalized,
		endpoint:    endpoint,
		displayName: displayName(normalized.Type),
	}
}

func (m *Manager) Endpoint() string {
	return m.endpoint
}

func (m *Manager) DisplayName() string {
	return m.displayName
}

func (m *Manager) StopOnExit() bool {
	return m.stopOnExit
}

func (m *Manager) EnsureRunning(ctx context.Context) (time.Duration, error) {
	running, err := m.IsRunning(ctx)
	if err != nil {
		return 0, err
	}
	if running {
		return 0, nil
	}
	return m.Start(ctx)
}

func (m *Manager) IsRunning(ctx context.Context) (bool, error) {
	return m.checkHealthy(ctx), nil
}

func (m *Manager) Start(ctx context.Context) (time.Duration, error) {
	begin := time.Now()

	if m.cfg.Endpoint != "" {
		return 0, fmt.Errorf("%s endpoint %s is not healthy", m.displayName, m.endpoint)
	}

	switch m.cfg.Type {
	case "floci":
		if err := m.startFloci(ctx); err != nil {
			return 0, err
		}
	case "stratus", "custom":
		if err := m.startProcess(ctx); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("unsupported emulator type %q", m.cfg.Type)
	}

	if err := m.waitHealthy(ctx); err != nil {
		_ = m.Stop(context.Background())
		return 0, fmt.Errorf("%s did not become healthy: %w", strings.ToLower(m.displayName), err)
	}

	return time.Since(begin), nil
}

func (m *Manager) Stop(ctx context.Context) error {
	if m.cfg.Type == "floci" {
		out, err := exec.CommandContext(ctx, "docker", "stop", flociContainerName).CombinedOutput()
		if err != nil && len(out) > 0 {
			return fmt.Errorf("stopping floci container: %w\n%s", err, string(out))
		}
		return nil
	}

	m.mu.Lock()
	cmd := m.cmd
	exitCh := m.exitCh
	tempDataDir := m.tempDataDir
	m.cmd = nil
	m.exitCh = nil
	m.tempDataDir = ""
	m.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		if tempDataDir != "" {
			_ = os.RemoveAll(tempDataDir)
		}
		return nil
	}

	_ = cmd.Process.Signal(os.Interrupt)

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
	case <-exitCh:
	}

	if tempDataDir != "" {
		_ = os.RemoveAll(tempDataDir)
	}
	return nil
}

func (m *Manager) startFloci(ctx context.Context) error {
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", flociContainerName).Run()
	_ = exec.CommandContext(ctx, "docker", "network", "create", flociNetworkName).Run()

	cmd := exec.CommandContext(ctx, "docker", dockerRunArgs(m.cfg.Image, m.cfg.Port, resolveDockerSocketHostPath())...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("starting floci container: %w\n%s", err, string(out))
	}
	return nil
}

func (m *Manager) startProcess(_ context.Context) error {
	args, tempDataDir, err := m.commandArgs()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("emulator command is empty")
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if m.cfg.Type == "stratus" && tempDataDir != "" {
		m.tempDataDir = tempDataDir
	}

	if err := cmd.Start(); err != nil {
		if tempDataDir != "" {
			_ = os.RemoveAll(tempDataDir)
		}
		return fmt.Errorf("starting %s process: %w", strings.ToLower(m.displayName), err)
	}

	exitCh := make(chan error, 1)
	go func() {
		exitCh <- cmd.Wait()
	}()

	m.mu.Lock()
	m.cmd = cmd
	m.exitCh = exitCh
	m.stopOnExit = true
	m.mu.Unlock()

	return nil
}

func (m *Manager) commandArgs() ([]string, string, error) {
	command := strings.TrimSpace(m.cfg.Command)
	if command == "" {
		return nil, "", nil
	}

	args := strings.Fields(command)
	if len(args) == 0 {
		return nil, "", nil
	}

	tempDataDir := ""
	if m.cfg.Type == "stratus" {
		if !hasFlag(args, "--port") {
			args = append(args, "--port", fmt.Sprintf("%d", m.cfg.Port))
		}

		if !hasFlag(args, "--data-dir") {
			dataDir := strings.TrimSpace(m.cfg.DataDir)
			if dataDir == "" {
				dir, err := os.MkdirTemp("", "preflight-stratus-*")
				if err != nil {
					return nil, "", fmt.Errorf("creating temp stratus data dir: %w", err)
				}
				dataDir = dir
				tempDataDir = dir
			}
			args = append(args, "--data-dir", dataDir)
		}
	}

	return args, tempDataDir, nil
}

func (m *Manager) waitHealthy(ctx context.Context) error {
	deadline := time.Now().Add(healthTimeout)
	hc := &http.Client{Timeout: 2 * time.Second}
	url := m.endpoint + healthPath(m.cfg)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s to become healthy after %s", strings.ToLower(m.displayName), healthTimeout)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := m.processExitError(); err != nil {
			return err
		}

		resp, err := hc.Get(url) //nolint:noctx
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}

		time.Sleep(healthPollInterval)
	}
}

func (m *Manager) checkHealthy(ctx context.Context) bool {
	hc := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.endpoint+healthPath(m.cfg), nil)
	if err != nil {
		return false
	}
	resp, err := hc.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (m *Manager) processExitError() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case err := <-m.exitCh:
		if err == nil {
			return fmt.Errorf("%s exited before becoming healthy", strings.ToLower(m.displayName))
		}
		return fmt.Errorf("%s exited before becoming healthy: %w", strings.ToLower(m.displayName), err)
	default:
		return nil
	}
}

func displayName(kind string) string {
	switch kind {
	case "floci":
		return "Floci"
	case "stratus":
		return "Stratus"
	default:
		return "Emulator"
	}
}

func healthPath(cfg config.EmulatorConfig) string {
	if cfg.HealthPath != "" {
		return cfg.HealthPath
	}

	switch cfg.Type {
	case "floci":
		return "/_floci/health"
	case "stratus":
		return "/_stratus/health"
	default:
		return ""
	}
}

func dockerRunArgs(image string, port int, hostDockerSocketPath string) []string {
	args := []string{
		"run",
		"--detach",
		"--name", flociContainerName,
		"--network", flociNetworkName,
		"--network-alias", flociContainerName,
		"--publish", fmt.Sprintf("%d:4566", port),
		"--rm",
		"--env", "FLOCI_SERVICES_DOCKER_NETWORK=" + flociNetworkName,
	}
	if hostDockerSocketPath != "" {
		args = append(args, "--volume", hostDockerSocketPath+":"+dockerSocketPath)
	}
	args = append(args, image)
	return args
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

func resolveDockerSocketHostPath() string {
	if hostPath := dockerHostSocketPathFromEnv(os.Getenv("DOCKER_HOST")); isSocketPath(hostPath) {
		return hostPath
	}

	if home, err := os.UserHomeDir(); err == nil {
		hostPath := filepath.Join(home, ".docker", "run", "docker.sock")
		if isSocketPath(hostPath) {
			return hostPath
		}
	}

	if isSocketPath(dockerSocketPath) {
		return dockerSocketPath
	}

	return ""
}

func dockerHostSocketPathFromEnv(dockerHost string) string {
	const unixPrefix = "unix://"
	if strings.HasPrefix(dockerHost, unixPrefix) {
		return strings.TrimPrefix(dockerHost, unixPrefix)
	}
	return ""
}

func isSocketPath(path string) bool {
	if path == "" {
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}
