package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// TerraformRunner deploys a Terraform module to Floci by overriding the
// AWS provider endpoint via environment variables.
type TerraformRunner struct {
	dir           string
	stackName     string
	flociEndpoint string
}

// NewTerraformRunner constructs a TerraformRunner.
// stackName is used for display purposes only (Terraform doesn't have stack names).
func NewTerraformRunner(dir, stackName, flociEndpoint string) *TerraformRunner {
	return &TerraformRunner{
		dir:           dir,
		stackName:     stackName,
		flociEndpoint: flociEndpoint,
	}
}

func (r *TerraformRunner) StackName() string { return r.stackName }

// Deploy runs `terraform init` (if needed) then `terraform apply -auto-approve`
// with Floci endpoint overrides.
func (r *TerraformRunner) Deploy(ctx context.Context) error {
	tfBin, err := findTerraform()
	if err != nil {
		return err
	}

	env := append(os.Environ(), flociEnv(r.flociEndpoint)...)
	env = append(env,
		"TF_INPUT=0",
		"TF_PLUGIN_TIMEOUT=2m",
	)

	// Always run init so provider plugins stay in sync with the current
	// binary, lock file, and module graph. This avoids stale plugin caches
	// when the user upgrades Terraform or switches machine architecture.
	initCmd := exec.CommandContext(ctx, tfBin, "init")
	initCmd.Dir = r.dir
	initCmd.Stdout = os.Stdout
	initCmd.Stderr = os.Stderr
	initCmd.Env = env
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("terraform init failed: %w", err)
	}

	applyCmd := exec.CommandContext(ctx, tfBin, "apply", "-auto-approve")
	applyCmd.Dir = r.dir
	applyCmd.Stdout = os.Stdout
	applyCmd.Stderr = os.Stderr
	applyCmd.Env = env
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("terraform apply failed: %w", err)
	}

	return nil
}

func findTerraform() (string, error) {
	return findTerraformWithLookPath(exec.LookPath, terraformVersionOutput, runtime.GOOS, runtime.GOARCH)
}

func findTerraformWithLookPath(
	lookPath func(string) (string, error),
	versionOutput func(string) (string, error),
	hostOS, hostArch string,
) (string, error) {
	for _, candidate := range []string{"terraform", "tofu"} {
		path, err := lookPath(candidate)
		if err != nil {
			continue
		}
		if err := validateTerraformBinary(path, versionOutput, hostOS, hostArch); err != nil {
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("terraform not found — install from https://developer.hashicorp.com/terraform/install")
}

func validateTerraformBinary(path string, versionOutput func(string) (string, error), hostOS, hostArch string) error {
	if hostOS != "darwin" || hostArch != "arm64" {
		return nil
	}

	out, err := versionOutput(path)
	if err != nil {
		return nil
	}

	if parseTerraformPlatform(out) != "darwin_amd64" {
		return nil
	}

	return fmt.Errorf(
		"terraform binary %q is darwin_amd64 on an arm64 Mac; install an arm64 terraform/tofu build because the AWS provider can hang on startup under Rosetta",
		path,
	)
}

func terraformVersionOutput(path string) (string, error) {
	out, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func parseTerraformPlatform(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "on ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "on "))
		}
	}
	return ""
}
