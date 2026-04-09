package load

import (
	"context"

	"github.com/rmukubvu/preflight/internal/deploy"
)

func runCDKDeploy(ctx context.Context, dir, stackName, endpoint, cdkApp string) error {
	return deploy.NewCDKRunner(dir, stackName, endpoint, cdkApp).Deploy(ctx)
}

func runTerraformDeploy(ctx context.Context, dir, stackName, endpoint string) error {
	return deploy.NewTerraformRunner(dir, stackName, endpoint).Deploy(ctx)
}
