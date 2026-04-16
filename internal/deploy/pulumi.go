package deploy

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
)

// PulumiRunner deploys a Pulumi project to the configured emulator by using a
// local Pulumi backend and emulator-oriented AWS provider settings supplied by
// the project itself.
type PulumiRunner struct {
	dir              string
	stackName        string
	emulatorEndpoint string
}

func NewPulumiRunner(dir, stackName, emulatorEndpoint string) *PulumiRunner {
	return &PulumiRunner{
		dir:              dir,
		stackName:        stackName,
		emulatorEndpoint: emulatorEndpoint,
	}
}

func (r *PulumiRunner) StackName() string { return r.stackName }

func (r *PulumiRunner) Deploy(ctx context.Context) error {
	pulumiBin, err := findPulumi()
	if err != nil {
		return err
	}

	stackName := r.stackName
	if stackName == "" {
		stackName = "preflight"
	}

	backendDir := filepath.Join(r.dir, ".pulumi-state")
	if err := os.MkdirAll(backendDir, 0o755); err != nil {
		return fmt.Errorf("creating pulumi backend dir: %w", err)
	}
	backendURL := (&url.URL{Scheme: "file", Path: backendDir}).String()

	env := append(os.Environ(), emulatorEnv(r.emulatorEndpoint)...)
	env = append(env,
		"PULUMI_HOME="+filepath.Join(r.dir, ".pulumi"),
		"PULUMI_BACKEND_URL="+backendURL,
		"PULUMI_CONFIG_PASSPHRASE=",
		"PULUMI_SKIP_UPDATE_CHECK=true",
	)

	loginCmd := exec.CommandContext(ctx, pulumiBin, "login", backendURL)
	loginCmd.Dir = r.dir
	loginCmd.Stdout = os.Stdout
	loginCmd.Stderr = os.Stderr
	loginCmd.Env = env
	if err := loginCmd.Run(); err != nil {
		return fmt.Errorf("pulumi login failed: %w", err)
	}

	selectCmd := exec.CommandContext(ctx, pulumiBin, "stack", "select", stackName, "--create", "--non-interactive")
	selectCmd.Dir = r.dir
	selectCmd.Stdout = os.Stdout
	selectCmd.Stderr = os.Stderr
	selectCmd.Env = env
	if err := selectCmd.Run(); err != nil {
		return fmt.Errorf("pulumi stack select failed: %w", err)
	}

	upCmd := exec.CommandContext(ctx, pulumiBin, "up", "--yes", "--skip-preview", "--non-interactive", "--stack", stackName)
	upCmd.Dir = r.dir
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr
	upCmd.Env = env
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("pulumi up failed: %w", err)
	}

	return nil
}

func findPulumi() (string, error) {
	return findPulumiWithLookPath(exec.LookPath)
}

func findPulumiWithLookPath(lookPath func(string) (string, error)) (string, error) {
	path, err := lookPath("pulumi")
	if err == nil {
		return path, nil
	}
	return "", fmt.Errorf("pulumi not found — install via: brew install pulumi")
}
