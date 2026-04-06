package deploy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rmukubvu/preflight/internal/deploy"
)

func TestDetectStackType_CDK(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cdk.json"), []byte(`{"app":"npx ts-node bin/app.ts"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := deploy.DetectStackType(dir); got != deploy.StackTypeCDK {
		t.Errorf("want cdk, got %q", got)
	}
}

func TestDetectStackType_Terraform(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`resource "aws_s3_bucket" "b" {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := deploy.DetectStackType(dir); got != deploy.StackTypeTerraform {
		t.Errorf("want terraform, got %q", got)
	}
}

func TestDetectStackType_Unknown_WhenEmpty(t *testing.T) {
	dir := t.TempDir()
	if got := deploy.DetectStackType(dir); got != deploy.StackTypeUnknown {
		t.Errorf("want unknown, got %q", got)
	}
}

func TestDetectStackType_CDK_TakesPriorityOverTerraform(t *testing.T) {
	dir := t.TempDir()
	// Both cdk.json and a .tf file exist — CDK wins.
	if err := os.WriteFile(filepath.Join(dir, "cdk.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`provider "aws" {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := deploy.DetectStackType(dir); got != deploy.StackTypeCDK {
		t.Errorf("want cdk when both present, got %q", got)
	}
}
