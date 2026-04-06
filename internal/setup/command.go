package setup

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rmukubvu/preflight/internal/config"
)

// NewCommand returns the cobra.Command for `preflight setup`.
func NewCommand() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure preflight via an interactive browser UI",
		Long: `Opens a local web interface to configure your LLM provider,
API keys, Floci settings, and stack type.

Saves configuration to .preflight.yaml in the current directory.
The server shuts down automatically after you click Save.

Set BROWSER=echo to print the URL without opening a browser (useful in CI).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			cfg, err := config.Load(workDir)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			srv := NewServer(ServerConfig{
				Port:    port,
				WorkDir: workDir,
				Config:  cfg,
			})

			return srv.Run(cmd.Context())
		},
	}

	cmd.Flags().IntVar(&port, "port", 7337, "Port for the setup web server")

	return cmd
}
