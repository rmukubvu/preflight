package floci

// This file is retained for the DockerClient interface used in integration
// tests where the real Docker daemon is unavailable and we need a stub.
// For production use, the Manager shells out to the docker CLI directly.

// DockerClient is a thin interface over the docker CLI operations that
// Manager performs. Implement this to inject a fake in integration tests.
type DockerClient interface {
	// RunContainer starts a container and returns its ID.
	RunContainer(image, name string, portBinding string) (string, error)

	// StopContainer stops the named container.
	StopContainer(name string) error

	// ContainerStatus returns "running", "exited", or "" if not found.
	ContainerStatus(name string) (string, error)
}
