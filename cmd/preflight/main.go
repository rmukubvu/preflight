package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rmukubvu/preflight/internal/deploy"
	"github.com/rmukubvu/preflight/internal/lint"
	"github.com/rmukubvu/preflight/internal/setup"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := newRootCommand().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "preflight",
		Short: "Validate CDK and Terraform stacks against a local AWS emulator before deploying to AWS",
		Long: `preflight runs your infrastructure stack against a local AWS emulator,
executes structural, wiring, IAM, and behavioural assertions, and provides
AI-assisted diagnosis when checks fail.

Get started:
  preflight setup     Configure via browser UI
  preflight lint      Run static readiness checks
  preflight deploy    Deploy and validate your stack`,
		Version:      version,
		SilenceUsage: true,
	}

	root.AddCommand(setup.NewCommand())
	root.AddCommand(lint.NewCommand())
	root.AddCommand(deploy.NewCommand())

	return root
}
