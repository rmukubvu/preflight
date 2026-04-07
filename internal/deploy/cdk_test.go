package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFindCDK_PrefersProjectLocalBinary(t *testing.T) {
	dir := t.TempDir()
	localCDK := filepath.Join(dir, "node_modules", ".bin")
	if err := os.MkdirAll(localCDK, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localCDK, "cdk"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bin, prefixArgs, err := findCDK(dir)
	if err != nil {
		t.Fatalf("findCDK: %v", err)
	}
	if bin != filepath.Join(localCDK, "cdk") {
		t.Fatalf("want local cdk binary, got %q", bin)
	}
	if len(prefixArgs) != 0 {
		t.Fatalf("want no prefix args for local cdk, got %v", prefixArgs)
	}
}

func TestFindCDKWithLookPath_PrefersNpxBeforeGlobalCDK(t *testing.T) {
	bin, prefixArgs, err := findCDKWithLookPath(func(name string) (string, error) {
		switch name {
		case "npx":
			return "/usr/local/bin/npx", nil
		case "cdk":
			return "/usr/local/bin/cdk", nil
		default:
			return "", fmt.Errorf("unexpected lookup: %s", name)
		}
	})
	if err != nil {
		t.Fatalf("findCDKWithLookPath: %v", err)
	}
	if bin != "/usr/local/bin/npx" {
		t.Fatalf("want npx, got %q", bin)
	}
	if len(prefixArgs) != 1 || prefixArgs[0] != "cdk" {
		t.Fatalf("want prefix args [cdk], got %v", prefixArgs)
	}
}

func TestFindCDKWithLookPath_FallsBackToGlobalCDK(t *testing.T) {
	bin, prefixArgs, err := findCDKWithLookPath(func(name string) (string, error) {
		switch name {
		case "npx":
			return "", fmt.Errorf("not found")
		case "cdk":
			return "/usr/local/bin/cdk", nil
		default:
			return "", fmt.Errorf("unexpected lookup: %s", name)
		}
	})
	if err != nil {
		t.Fatalf("findCDKWithLookPath: %v", err)
	}
	if bin != "/usr/local/bin/cdk" {
		t.Fatalf("want global cdk, got %q", bin)
	}
	if len(prefixArgs) != 0 {
		t.Fatalf("want no prefix args for global cdk, got %v", prefixArgs)
	}
}
