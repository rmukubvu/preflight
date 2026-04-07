package deploy

import (
	"fmt"
	"strings"
	"testing"
)

func TestFindTerraformWithLookPath_PrefersTerraformWhenCompatible(t *testing.T) {
	bin, err := findTerraformWithLookPath(
		func(name string) (string, error) {
			switch name {
			case "terraform":
				return "/opt/homebrew/bin/terraform", nil
			case "tofu":
				return "", fmt.Errorf("unexpected lookup: %s", name)
			default:
				return "", fmt.Errorf("unexpected lookup: %s", name)
			}
		},
		func(path string) (string, error) {
			if path != "/opt/homebrew/bin/terraform" {
				return "", fmt.Errorf("unexpected version path: %s", path)
			}
			return "Terraform v1.14.8\non darwin_arm64\n", nil
		},
		"darwin",
		"arm64",
	)
	if err != nil {
		t.Fatalf("findTerraformWithLookPath: %v", err)
	}
	if bin != "/opt/homebrew/bin/terraform" {
		t.Fatalf("want terraform path, got %q", bin)
	}
}

func TestFindTerraformWithLookPath_FallsBackToTofu(t *testing.T) {
	bin, err := findTerraformWithLookPath(
		func(name string) (string, error) {
			switch name {
			case "terraform":
				return "", fmt.Errorf("not found")
			case "tofu":
				return "/opt/homebrew/bin/tofu", nil
			default:
				return "", fmt.Errorf("unexpected lookup: %s", name)
			}
		},
		func(path string) (string, error) {
			if path != "/opt/homebrew/bin/tofu" {
				return "", fmt.Errorf("unexpected version path: %s", path)
			}
			return "OpenTofu v1.8.0\non darwin_arm64\n", nil
		},
		"darwin",
		"arm64",
	)
	if err != nil {
		t.Fatalf("findTerraformWithLookPath: %v", err)
	}
	if bin != "/opt/homebrew/bin/tofu" {
		t.Fatalf("want tofu path, got %q", bin)
	}
}

func TestFindTerraformWithLookPath_RejectsDarwinAMD64TerraformOnArm64Mac(t *testing.T) {
	_, err := findTerraformWithLookPath(
		func(name string) (string, error) {
			switch name {
			case "terraform":
				return "/usr/local/bin/terraform", nil
			case "tofu":
				return "", fmt.Errorf("unexpected lookup: %s", name)
			default:
				return "", fmt.Errorf("unexpected lookup: %s", name)
			}
		},
		func(path string) (string, error) {
			if path != "/usr/local/bin/terraform" {
				return "", fmt.Errorf("unexpected version path: %s", path)
			}
			return "Terraform v1.14.8\non darwin_amd64\n", nil
		},
		"darwin",
		"arm64",
	)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "darwin_amd64") {
		t.Fatalf("want architecture mismatch error, got %v", err)
	}
}

func TestParseTerraformPlatform(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "terraform",
			output: "Terraform v1.14.8\non darwin_arm64\n",
			want:   "darwin_arm64",
		},
		{
			name:   "tofu",
			output: "OpenTofu v1.8.0\non linux_amd64\n",
			want:   "linux_amd64",
		},
		{
			name:   "missing platform",
			output: "Terraform v1.14.8\n",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseTerraformPlatform(tt.output); got != tt.want {
				t.Fatalf("parseTerraformPlatform() = %q, want %q", got, tt.want)
			}
		})
	}
}
