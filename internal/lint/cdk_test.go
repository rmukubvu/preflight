package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindCDKPrefersProjectBinary(t *testing.T) {
	dir := t.TempDir()
	localBin := filepath.Join(dir, "node_modules", ".bin")
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	localCDK := filepath.Join(localBin, "cdk")
	if err := os.WriteFile(localCDK, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write local cdk: %v", err)
	}

	gotBin, gotArgs, err := findCDK(dir)
	if err != nil {
		t.Fatalf("findCDK returned error: %v", err)
	}
	if gotBin != localCDK {
		t.Fatalf("want local cdk %q, got %q", localCDK, gotBin)
	}
	if len(gotArgs) != 0 {
		t.Fatalf("want no prefix args, got %#v", gotArgs)
	}
}

func TestFindCDKFallsBackToNpx(t *testing.T) {
	origLookPath := lookPath
	t.Cleanup(func() {
		lookPath = origLookPath
	})
	lookPath = func(file string) (string, error) {
		if file == "npx" {
			return "/usr/bin/npx", nil
		}
		return "", os.ErrNotExist
	}

	gotBin, gotArgs, err := findCDK(t.TempDir())
	if err != nil {
		t.Fatalf("findCDK returned error: %v", err)
	}
	if gotBin != "/usr/bin/npx" {
		t.Fatalf("want npx path, got %q", gotBin)
	}
	if len(gotArgs) != 1 || gotArgs[0] != "cdk" {
		t.Fatalf("want [cdk], got %#v", gotArgs)
	}
}
