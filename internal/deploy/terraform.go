package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

	// Run init only if .terraform directory is absent.
	if !fileExists(r.dir + "/.terraform") {
		initCmd := exec.CommandContext(ctx, tfBin, "init")
		initCmd.Dir = r.dir
		initCmd.Stdout = os.Stdout
		initCmd.Stderr = os.Stderr
		initCmd.Env = env
		if err := initCmd.Run(); err != nil {
			return fmt.Errorf("terraform init failed: %w", err)
		}
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
	if path, err := exec.LookPath("terraform"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("tofu"); err == nil {
		return path, nil // OpenTofu is compatible
	}
	return "", fmt.Errorf("terraform not found — install from https://developer.hashicorp.com/terraform/install")
}
