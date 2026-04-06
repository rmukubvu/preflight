# preflight

Validate CDK/Terraform stacks against Floci before deploying to AWS.

## Stack

Go 1.23 · Cobra · yaml.v3 · lipgloss · aws-sdk-go-v2 · Docker SDK

## Commands

| Command | Purpose |
|---------|---------|
| `make build` | Compile to `dist/preflight` |
| `make test` | All tests with `-race` |
| `make test-unit` | Unit tests only (`-short`) |
| `make lint` | `go vet` + `staticcheck` |
| `make run-setup` | Build and run `preflight setup` |
| `make fmt` | Format all Go source |

## Architecture

```
cmd/preflight/         Cobra root + subcommand wiring (no logic here)
internal/config/       Config struct, YAML load/save, validation
internal/setup/        `preflight setup` command + embedded HTTP server + web UI
internal/floci/        Docker lifecycle for local AWS emulator
internal/deploy/       cdk deploy / terraform apply wrappers
internal/assertions/   Parallel assertion runner + individual checks
internal/diagnosis/    LLM provider interface + implementations
internal/report/       Terminal (lipgloss) and JSON output
pkg/aws/               aws-sdk-go-v2 client pointed at Floci endpoint
```

## Key Patterns

- Interface-first: assertions, diagnosis, floci, deploy all behind interfaces
- Table-driven tests throughout; always use `t.TempDir()` for filesystem ops
- Atomic config writes: write to `.tmp` then `os.Rename`
- `go:embed` for web assets in `internal/setup/web/`
- No global state; all dependencies injected via constructors
- `SilenceUsage: true` on cobra root
- Go 1.22+ method-prefixed routes (`"GET /api/config"`) — no extra router

## Config

Config lives in `.preflight.yaml` in the project directory.
Run `preflight setup` to configure via browser UI.
