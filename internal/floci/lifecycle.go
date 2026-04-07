// Package floci manages the lifecycle of the Floci local AWS emulator container.
// It shells out to the `docker` CLI so users don't need any additional setup
// beyond having Docker Desktop (or the daemon) running.
package floci

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	containerName      = "preflight-floci"
	dockerSocketPath   = "/var/run/docker.sock"
	networkName        = "preflight-floci-network"
	healthPath         = "/_floci/health"
	healthTimeout      = 30 * time.Second
	healthPollInterval = 250 * time.Millisecond
)

// Manager starts, stops, and probes the Floci Docker container.
type Manager struct {
	image    string
	port     int
	endpoint string // e.g. "http://localhost:4566"
}

// NewManager constructs a Manager for the given Floci image and host port.
func NewManager(image string, port int) *Manager {
	return &Manager{
		image:    image,
		port:     port,
		endpoint: fmt.Sprintf("http://localhost:%d", port),
	}
}

// Endpoint returns the HTTP base URL of the Floci container.
func (m *Manager) Endpoint() string {
	return m.endpoint
}

// IsRunning reports whether the Floci container is currently running.
func (m *Manager) IsRunning(ctx context.Context) (bool, error) {
	out, err := exec.CommandContext(ctx, "docker", "inspect",
		"--format", "{{.State.Status}}",
		containerName,
	).Output()
	if err != nil {
		// "docker inspect" exits non-zero when the container doesn't exist.
		return false, nil
	}
	status := strings.TrimSpace(string(out))
	return status == "running", nil
}

// EnsureRunning starts Floci if it is not already running.
// Returns the elapsed startup time. If Floci is already up, returns 0.
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

// Start pulls the image (if needed) and starts the Floci container.
// It waits until the health endpoint responds before returning.
func (m *Manager) Start(ctx context.Context) (time.Duration, error) {
	begin := time.Now()

	// Remove any existing stopped container with the same name.
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", containerName).Run()
	_ = exec.CommandContext(ctx, "docker", "network", "create", networkName).Run()

	cmd := exec.CommandContext(ctx, "docker", dockerRunArgs(m.image, m.port, resolveDockerSocketHostPath())...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("starting floci container: %w\n%s", err, string(out))
	}

	if err := m.waitHealthy(ctx); err != nil {
		return 0, fmt.Errorf("floci did not become healthy: %w", err)
	}

	return time.Since(begin), nil
}

func dockerRunArgs(image string, port int, hostDockerSocketPath string) []string {
	args := []string{
		"run",
		"--detach",
		"--name", containerName,
		"--network", networkName,
		"--network-alias", containerName,
		"--publish", fmt.Sprintf("%d:4566", port),
		"--rm",
		"--env", "FLOCI_SERVICES_DOCKER_NETWORK=" + networkName,
	}
	if hostDockerSocketPath != "" {
		args = append(args,
			"--volume", hostDockerSocketPath+":"+dockerSocketPath,
		)
	}
	args = append(args, image)
	return args
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

// Stop gracefully stops the Floci container.
func (m *Manager) Stop(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "docker", "stop", containerName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("stopping floci container: %w\n%s", err, string(out))
	}
	return nil
}

// waitHealthy polls the Floci health endpoint until it responds with 200.
func (m *Manager) waitHealthy(ctx context.Context) error {
	deadline := time.Now().Add(healthTimeout)
	hc := &http.Client{Timeout: 2 * time.Second}
	url := m.endpoint + healthPath

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for floci to become healthy after %s", healthTimeout)
		}
		if ctx.Err() != nil {
			return ctx.Err()
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

// containerInfo is used to decode docker ps output.
type containerInfo struct {
	Names  string `json:"Names"`
	Status string `json:"Status"`
	Ports  string `json:"Ports"`
}

// runningContainers lists containers matching the given name filter.
// Used internally for testing and diagnostics.
func runningContainers(ctx context.Context, nameFilter string) ([]containerInfo, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps",
		"--filter", "name="+nameFilter,
		"--format", "{{json .}}",
	).Output()
	if err != nil {
		return nil, err
	}

	var containers []containerInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var c containerInfo
		if err := json.Unmarshal([]byte(line), &c); err == nil {
			containers = append(containers, c)
		}
	}
	return containers, nil
}
