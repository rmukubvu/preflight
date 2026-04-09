package emulator

import (
	"reflect"
	"testing"

	"github.com/rmukubvu/preflight/internal/config"
)

func TestDockerRunArgs_WithDockerSocketMount(t *testing.T) {
	got := dockerRunArgs("hectorvent/floci:latest", 4566, "/Users/robson/.docker/run/docker.sock")
	want := []string{
		"run",
		"--detach",
		"--name", flociContainerName,
		"--network", flociNetworkName,
		"--network-alias", flociContainerName,
		"--publish", "4566:4566",
		"--rm",
		"--env", "FLOCI_SERVICES_DOCKER_NETWORK=" + flociNetworkName,
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
		"--name", flociContainerName,
		"--network", flociNetworkName,
		"--network-alias", flociContainerName,
		"--publish", "4566:4566",
		"--rm",
		"--env", "FLOCI_SERVICES_DOCKER_NETWORK=" + flociNetworkName,
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

func TestHealthPath_DefaultsByBackend(t *testing.T) {
	if got := healthPath(config.EmulatorConfig{Type: "floci"}); got != "/_floci/health" {
		t.Fatalf("want floci health path, got %q", got)
	}
	if got := healthPath(config.EmulatorConfig{Type: "stratus"}); got != "/_stratus/health" {
		t.Fatalf("want stratus health path, got %q", got)
	}
	if got := healthPath(config.EmulatorConfig{Type: "custom"}); got != "" {
		t.Fatalf("want empty custom health path, got %q", got)
	}
}

func TestCommandArgs_StratusAddsDefaultPortAndDataDir(t *testing.T) {
	mgr := NewManager(config.EmulatorConfig{
		Type:    "stratus",
		Command: "stratus",
		Port:    4567,
	})

	args, dataDir, err := mgr.commandArgs()
	if err != nil {
		t.Fatalf("commandArgs: %v", err)
	}
	if dataDir == "" {
		t.Fatal("want temp data dir")
	}
	if !reflect.DeepEqual(args[:3], []string{"stratus", "--port", "4567"}) {
		t.Fatalf("unexpected args prefix: %v", args)
	}
}

func TestCommandArgs_StratusPreservesExplicitFlags(t *testing.T) {
	mgr := NewManager(config.EmulatorConfig{
		Type:    "stratus",
		Command: "stratus --port 4570 --data-dir ./tmp",
		Port:    4567,
	})

	args, dataDir, err := mgr.commandArgs()
	if err != nil {
		t.Fatalf("commandArgs: %v", err)
	}
	if dataDir != "" {
		t.Fatalf("want no temp data dir, got %q", dataDir)
	}
	want := []string{"stratus", "--port", "4570", "--data-dir", "./tmp"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("commandArgs() = %v, want %v", args, want)
	}
}
