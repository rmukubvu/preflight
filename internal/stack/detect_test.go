package stack_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rmukubvu/preflight/internal/stack"
)

func TestDetectCDK(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cdk.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := stack.Detect(dir); got != stack.TypeCDK {
		t.Fatalf("Detect() = %q, want %q", got, stack.TypeCDK)
	}
}

func TestDetectTerraform(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("terraform {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := stack.Detect(dir); got != stack.TypeTerraform {
		t.Fatalf("Detect() = %q, want %q", got, stack.TypeTerraform)
	}
}

func TestDetectUnknownWhenEmpty(t *testing.T) {
	if got := stack.Detect(t.TempDir()); got != stack.TypeUnknown {
		t.Fatalf("Detect() = %q, want unknown", got)
	}
}

func TestDetectPrefersCDKWhenBothExist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cdk.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("terraform {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := stack.Detect(dir); got != stack.TypeCDK {
		t.Fatalf("Detect() = %q, want %q", got, stack.TypeCDK)
	}
}
