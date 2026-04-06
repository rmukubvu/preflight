// Package floci manages the lifecycle of the Floci local AWS emulator container.
package floci

import "context"

// Manager starts, stops, and inspects the Floci Docker container.
type Manager struct {
	image   string
	port    int
	client  DockerClient
}

// NewManager constructs a Manager.
func NewManager(image string, port int, client DockerClient) *Manager {
	return &Manager{image: image, port: port, client: client}
}

// IsRunning reports whether a Floci container is currently running.
func (m *Manager) IsRunning(ctx context.Context) (bool, error) {
	containers, err := m.client.ContainerList(ctx, nil)
	if err != nil {
		return false, err
	}
	for _, c := range containers {
		for _, name := range c.Names {
			if name == "/floci" || name == "floci" {
				return c.Status == "running", nil
			}
		}
	}
	return false, nil
}

// EnsureRunning starts Floci if it is not already running.
// It is idempotent: calling it when Floci is already up is a no-op.
func (m *Manager) EnsureRunning(ctx context.Context) error {
	running, err := m.IsRunning(ctx)
	if err != nil {
		return err
	}
	if running {
		return nil
	}
	return m.Start(ctx)
}

// Start pulls the image (if needed) and starts the Floci container.
func (m *Manager) Start(_ context.Context) error {
	// TODO: implement in M1 milestone
	return nil
}

// Stop gracefully stops the running Floci container.
func (m *Manager) Stop(_ context.Context) error {
	// TODO: implement in M1 milestone
	return nil
}
