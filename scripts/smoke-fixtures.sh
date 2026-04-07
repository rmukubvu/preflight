#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREFLIGHT_BIN="${PREFLIGHT_BIN:-$ROOT_DIR/dist/preflight}"
TARGET="${1:-all}"
FLOCI_CONTAINER_NAME="${FLOCI_CONTAINER_NAME:-preflight-floci}"
FLOCI_NETWORK_NAME="${FLOCI_NETWORK_NAME:-preflight-floci-network}"
FLOCI_ENDPOINT="${FLOCI_ENDPOINT:-http://localhost:4566}"
FLOCI_HEALTH_URL="${FLOCI_ENDPOINT}/_floci/health"
FLOCI_IMAGE="${FLOCI_IMAGE:-hectorvent/floci:latest}"
CDK_ASSET_BUCKET="${CDK_ASSET_BUCKET:-preflight-cdk-fixture-assets}"

build_preflight() {
  mkdir -p "$ROOT_DIR/dist"
  GOCACHE="$ROOT_DIR/.gocache" go build -o "$PREFLIGHT_BIN" "$ROOT_DIR/cmd/preflight"
}

reset_floci() {
  docker rm -f "$FLOCI_CONTAINER_NAME" >/dev/null 2>&1 || true
}

resolve_docker_socket() {
  if [[ "${DOCKER_HOST:-}" == unix://* ]]; then
    printf '%s\n' "${DOCKER_HOST#unix://}"
    return 0
  fi
  if [[ -S "${HOME}/.docker/run/docker.sock" ]]; then
    printf '%s\n' "${HOME}/.docker/run/docker.sock"
    return 0
  fi
  if [[ -S /var/run/docker.sock ]]; then
    printf '%s\n' /var/run/docker.sock
    return 0
  fi
  return 1
}

start_floci() {
  reset_floci
  docker network create "$FLOCI_NETWORK_NAME" >/dev/null 2>&1 || true
  local docker_args=(
    --detach
    --name "$FLOCI_CONTAINER_NAME"
    --network "$FLOCI_NETWORK_NAME"
    --network-alias "$FLOCI_CONTAINER_NAME"
    --publish 4566:4566
    --rm
    --env "FLOCI_SERVICES_DOCKER_NETWORK=$FLOCI_NETWORK_NAME"
  )
  local docker_socket
  if docker_socket="$(resolve_docker_socket)"; then
    docker_args+=(--volume "${docker_socket}:/var/run/docker.sock")
  fi
  docker run \
    "${docker_args[@]}" \
    "$FLOCI_IMAGE" >/dev/null

  for _ in $(seq 1 120); do
    if curl -fsS "$FLOCI_HEALTH_URL" >/dev/null; then
      return 0
    fi
    sleep 0.25
  done

  echo "timed out waiting for Floci to become healthy at $FLOCI_HEALTH_URL" >&2
  return 1
}

ensure_cdk_fixture_deps() {
  local fixture_dir="$ROOT_DIR/test/fixtures/cdk-http-sqs-ddb"
  if [[ ! -d "$fixture_dir/node_modules" ]]; then
    (
      cd "$fixture_dir"
      npm install
    )
  fi
}

seed_cdk_fixture_assets() {
  local fixture_dir="$ROOT_DIR/test/fixtures/cdk-http-sqs-ddb"
  local build_dir="$fixture_dir/build"
  local jobs_zip="$build_dir/jobs-handler.zip"
  local worker_zip="$build_dir/worker.zip"

  mkdir -p "$build_dir"
  rm -f "$jobs_zip" "$worker_zip"

  (
    cd "$fixture_dir/lambda/api"
    zip -q -j "$jobs_zip" index.py
  )
  (
    cd "$fixture_dir/lambda/worker"
    zip -q -j "$worker_zip" index.py
  )

  local jobs_sha
  local worker_sha
  jobs_sha="$(shasum -a 256 "$jobs_zip" | awk '{print $1}')"
  worker_sha="$(shasum -a 256 "$worker_zip" | awk '{print $1}')"
  export JOBS_HANDLER_ASSET_KEY="jobs-handler-${jobs_sha}.zip"
  export WORKER_ASSET_KEY="worker-${worker_sha}.zip"

  AWS_ACCESS_KEY_ID=test \
  AWS_SECRET_ACCESS_KEY=test \
  AWS_REGION=us-east-1 \
  aws --endpoint-url "$FLOCI_ENDPOINT" s3 mb "s3://$CDK_ASSET_BUCKET" >/dev/null 2>&1 || true

  AWS_ACCESS_KEY_ID=test \
  AWS_SECRET_ACCESS_KEY=test \
  AWS_REGION=us-east-1 \
  aws --endpoint-url "$FLOCI_ENDPOINT" s3 cp "$jobs_zip" "s3://$CDK_ASSET_BUCKET/$JOBS_HANDLER_ASSET_KEY" >/dev/null

  AWS_ACCESS_KEY_ID=test \
  AWS_SECRET_ACCESS_KEY=test \
  AWS_REGION=us-east-1 \
  aws --endpoint-url "$FLOCI_ENDPOINT" s3 cp "$worker_zip" "s3://$CDK_ASSET_BUCKET/$WORKER_ASSET_KEY" >/dev/null
}

run_cdk_fixture() {
  local fixture_dir="$ROOT_DIR/test/fixtures/cdk-http-sqs-ddb"
  ensure_cdk_fixture_deps
  start_floci
  seed_cdk_fixture_assets
  (
    cd "$fixture_dir"
    "$PREFLIGHT_BIN" deploy --stack-name SmokeFixtureStack --no-ai
  )
}

run_terraform_fixture() {
  local fixture_dir="$ROOT_DIR/test/fixtures/terraform-http-sqs-ddb"

  if ! command -v terraform >/dev/null 2>&1 && ! command -v tofu >/dev/null 2>&1; then
    echo "terraform fixture skipped: terraform/tofu not installed" >&2
    return 0
  fi

  reset_floci
  (
    cd "$fixture_dir"
    rm -f terraform.tfstate terraform.tfstate.backup
    rm -f .terraform/terraform.tfstate
    "$PREFLIGHT_BIN" deploy --stack-type terraform --no-ai
  )
}

cleanup() {
  rm -rf "$ROOT_DIR/.gocache"
}

trap cleanup EXIT

build_preflight

case "$TARGET" in
  cdk)
    run_cdk_fixture
    ;;
  terraform)
    run_terraform_fixture
    ;;
  all)
    run_cdk_fixture
    run_terraform_fixture
    ;;
  *)
    echo "usage: $0 [cdk|terraform|all]" >&2
    exit 2
    ;;
esac
