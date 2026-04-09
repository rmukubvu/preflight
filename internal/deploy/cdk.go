package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CDKRunner deploys a CDK stack to the configured emulator by overriding AWS endpoint environment
// variables so that `cdk deploy` targets localhost instead of real AWS.
type CDKRunner struct {
	dir              string
	stackName        string
	emulatorEndpoint string
	cdkApp           string // optional: override the CDK app command
}

// NewCDKRunner constructs a CDKRunner.
// stackName can be empty to deploy all stacks in the app.
func NewCDKRunner(dir, stackName, emulatorEndpoint, cdkApp string) *CDKRunner {
	return &CDKRunner{
		dir:              dir,
		stackName:        stackName,
		emulatorEndpoint: emulatorEndpoint,
		cdkApp:           cdkApp,
	}
}

func (r *CDKRunner) StackName() string { return r.stackName }

// Deploy runs `cdk deploy` with emulator endpoint overrides.
// It streams stdout/stderr to the terminal so users see CDK's output.
func (r *CDKRunner) Deploy(ctx context.Context) error {
	args := []string{"deploy", "--require-approval", "never"}
	if r.stackName != "" {
		args = append(args, r.stackName)
	}

	cdkBin, prefixArgs, err := findCDK(r.dir)
	if err != nil {
		return err
	}

	if r.cdkApp != "" {
		args = append([]string{"--app", r.cdkApp}, args...)
	}
	args = append(prefixArgs, args...)

	cmd := exec.CommandContext(ctx, cdkBin, args...)
	cmd.Dir = r.dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), emulatorEnv(r.emulatorEndpoint)...)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cdk deploy failed: %w", err)
	}
	return nil
}

// findCDK locates the cdk executable.
// Preference order:
// 1. project-local node_modules/.bin/cdk
// 2. npx cdk
// 3. global cdk install
func findCDK(dir string) (string, []string, error) {
	localCDK := filepath.Join(dir, "node_modules", ".bin", "cdk")
	if fileExists(localCDK) {
		return localCDK, nil, nil
	}

	return findCDKWithLookPath(exec.LookPath)
}

func findCDKWithLookPath(lookPath func(string) (string, error)) (string, []string, error) {
	if path, err := lookPath("npx"); err == nil {
		return path, []string{"cdk"}, nil
	}
	if path, err := lookPath("cdk"); err == nil {
		return path, nil, nil
	}
	return "", nil, fmt.Errorf("cdk not found — install via: npm install -g aws-cdk")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// emulatorEnv returns the environment variables needed to redirect AWS SDK
// calls to the local emulator endpoint.
func emulatorEnv(endpoint string) []string {
	return []string{
		"AWS_ENDPOINT_URL=" + endpoint,
		"AWS_ACCESS_KEY_ID=test",
		"AWS_SECRET_ACCESS_KEY=test",
		"AWS_DEFAULT_REGION=us-east-1",
		"CDK_DEFAULT_ACCOUNT=000000000000",
		"CDK_DEFAULT_REGION=us-east-1",
		"EMULATOR_ENDPOINT=" + endpoint,
	}
}
