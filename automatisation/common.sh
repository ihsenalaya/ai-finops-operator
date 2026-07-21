#!/usr/bin/env bash
# Shared configuration for the ai-finops-operator kind automation.
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-finops-operator}"
NAMESPACE="${NAMESPACE:-finops-system}"
DEMO_NAMESPACE="${DEMO_NAMESPACE:-finops-demo}"
MONITORING_NAMESPACE="${MONITORING_NAMESPACE:-monitoring}"
IMAGE_NAME="${IMAGE_NAME:-finops-operator}"
IMAGE_TAG="${IMAGE_TAG:-dev}"
HELM_RELEASE="${HELM_RELEASE:-finops}"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${HERE}/.." && pwd)"
BUILD_CONTEXT="${REPO_ROOT}"
CHART_DIR="${REPO_ROOT}/charts/ai-finops-operator"
DOCKERFILE="${REPO_ROOT}/Dockerfile"

log()  { printf '\033[36m[finops-kind]\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[finops-kind] WARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[31m[finops-kind] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

require() {
  command -v "$1" >/dev/null 2>&1 || die "'$1' is required but not installed"
}
