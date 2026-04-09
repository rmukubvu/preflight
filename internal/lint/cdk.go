package lint

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	lookPath           = exec.LookPath
	execCommandContext = exec.CommandContext
)

func synthCDK(ctx context.Context, dir, stackName, cdkApp string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "preflight-lint-*")
	if err != nil {
		return "", fmt.Errorf("creating synth directory: %w", err)
	}

	cdkBin, prefixArgs, err := findCDK(dir)
	if err != nil {
		return "", err
	}

	args := []string{"synth"}
	if stackName != "" {
		args = append(args, stackName)
	}
	args = append(args, "--quiet", "--output", tmpDir)
	if cdkApp != "" {
		args = append([]string{"--app", cdkApp}, args...)
	}
	args = append(prefixArgs, args...)

	cmd := execCommandContext(ctx, cdkBin, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), deployEnv("http://127.0.0.1:4566")...)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cdk synth failed: %w", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "manifest.json")); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("checking cdk output: %w", err)
	}
	return tmpDir, nil
}

func findCDK(dir string) (string, []string, error) {
	localCDK := filepath.Join(dir, "node_modules", ".bin", "cdk")
	if fileExists(localCDK) {
		return localCDK, nil, nil
	}
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

func deployEnv(endpoint string) []string {
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
