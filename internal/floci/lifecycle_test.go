package floci

import (
	"reflect"
	"testing"
)

func TestDockerRunArgs_WithDockerSocketMount(t *testing.T) {
	got := dockerRunArgs("hectorvent/floci:latest", 4566, "/Users/robson/.docker/run/docker.sock")
	want := []string{
		"run",
		"--detach",
		"--name", containerName,
		"--network", networkName,
		"--network-alias", containerName,
		"--publish", "4566:4566",
		"--rm",
		"--env", "FLOCI_SERVICES_DOCKER_NETWORK=" + networkName,
		"--volume", "/Users/robson/.docker/run/docker.sock:" + dockerSocketPath,
		"hectorvent/floci:latest",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dockerRunArgs() = %v, want %v", got, want)
	}
}

func TestDockerRunArgs_WithoutDockerSocketMount(t *testing.T) {
	got := dockerRunArgs("hectorvent/floci:latest", 4566, "")
	want := []string{
		"run",
		"--detach",
		"--name", containerName,
		"--network", networkName,
		"--network-alias", containerName,
		"--publish", "4566:4566",
		"--rm",
		"--env", "FLOCI_SERVICES_DOCKER_NETWORK=" + networkName,
		"hectorvent/floci:latest",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dockerRunArgs() = %v, want %v", got, want)
	}
}

func TestDockerHostSocketPathFromEnv(t *testing.T) {
	if got := dockerHostSocketPathFromEnv("unix:///Users/robson/.docker/run/docker.sock"); got != "/Users/robson/.docker/run/docker.sock" {
		t.Fatalf("dockerHostSocketPathFromEnv() = %q", got)
	}
	if got := dockerHostSocketPathFromEnv("tcp://127.0.0.1:2375"); got != "" {
		t.Fatalf("dockerHostSocketPathFromEnv() = %q, want empty", got)
	}
}
