package floci

import "context"

// DockerClient is the interface for Docker daemon operations.
// It wraps docker/docker/client.Client to allow test mocking.
type DockerClient interface {
	// ContainerList returns running containers. opts is docker's
	// container.ListOptions but typed as any to avoid the direct import here.
	ContainerList(ctx context.Context, opts any) ([]ContainerSummary, error)

	// ContainerStart starts the named container.
	ContainerStart(ctx context.Context, containerID string) error

	// ContainerStop stops the named container gracefully.
	ContainerStop(ctx context.Context, containerID string) error
}

// ContainerSummary is a minimal projection of a Docker container's state.
type ContainerSummary struct {
	ID     string
	Names  []string
	Status string // e.g. "running", "exited"
	Ports  []PortBinding
}

// PortBinding maps a container port to a host port.
type PortBinding struct {
	HostPort      string
	ContainerPort string
}
