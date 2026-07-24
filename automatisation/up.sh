#!/usr/bin/env bash
# One-command kind deployment of the ai-finops-operator with all its
# dependencies: kind cluster, Prometheus + Grafana (kube-prometheus-stack),
# the operator chart, demo test applications, and the FinOps dashboard.
#
# Usage:  ./up.sh            # full stack
#         SKIP_MONITORING=1 ./up.sh   # operator + test apps only
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require kind
require kubectl
require helm
require docker

# 1. Cluster ------------------------------------------------------------------
if kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
  log "kind cluster '${CLUSTER_NAME}' already exists; reusing it."
else
  log "creating kind cluster '${CLUSTER_NAME}'..."
  kind create cluster --name "${CLUSTER_NAME}"
fi
kind export kubeconfig --name "${CLUSTER_NAME}"

# 2. Operator image -----------------------------------------------------------
log "building operator image ${IMAGE_NAME}:${IMAGE_TAG}..."
docker build -f "${DOCKERFILE}" -t "${IMAGE_NAME}:${IMAGE_TAG}" "${BUILD_CONTEXT}"
kind load docker-image "${IMAGE_NAME}:${IMAGE_TAG}" --name "${CLUSTER_NAME}"

# 2b. Console image (graphical console, deployed alongside the operator) -------
log "building console image console-api:${IMAGE_TAG}..."
docker build -f "${BUILD_CONTEXT}/Dockerfile.console-api" -t "console-api:${IMAGE_TAG}" "${BUILD_CONTEXT}"
kind load docker-image "console-api:${IMAGE_TAG}" --name "${CLUSTER_NAME}"

# 3. Monitoring (Prometheus Operator + Grafana) --------------------------------
if [ -z "${SKIP_MONITORING:-}" ]; then
  log "installing kube-prometheus-stack in namespace ${MONITORING_NAMESPACE}..."
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
  helm repo update prometheus-community >/dev/null
  helm upgrade --install monitoring prometheus-community/kube-prometheus-stack \
    --namespace "${MONITORING_NAMESPACE}" --create-namespace \
    --set grafana.sidecar.dashboards.enabled=true \
    --set grafana.sidecar.dashboards.searchNamespace=ALL \
    --set grafana.adminPassword=admin \
    --wait --timeout 10m
  SM_ARGS=(--set metrics.serviceMonitor.enabled=true --set metrics.serviceMonitor.labels.release=monitoring)
else
  warn "SKIP_MONITORING set — Prometheus/Grafana not installed."
  SM_ARGS=()
fi

# 4. Operator chart ------------------------------------------------------------
log "installing ai-finops-operator chart (release ${HELM_RELEASE})..."
helm upgrade --install "${HELM_RELEASE}" "${CHART_DIR}" \
  --namespace "${NAMESPACE}" --create-namespace \
  --set image.repository="${IMAGE_NAME}" \
  --set image.tag="${IMAGE_TAG}" \
  --set image.pullPolicy=Never \
  --set console.image.repository=console-api \
  --set console.image.tag="${IMAGE_TAG}" \
  --set console.image.pullPolicy=Never \
  "${SM_ARGS[@]}" \
  --wait --timeout 5m

# 5. Test applications ----------------------------------------------------------
log "applying test applications in namespace ${DEMO_NAMESPACE}..."
kubectl create namespace "${DEMO_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace "${DEMO_NAMESPACE}" ai-finops/enabled=true --overwrite
kubectl -n "${DEMO_NAMESPACE}" apply -f "${HERE}/test-apps/"

# 6. Grafana dashboard -----------------------------------------------------------
if [ -z "${SKIP_MONITORING:-}" ]; then
  log "importing the FinOps Grafana dashboard..."
  kubectl -n "${MONITORING_NAMESPACE}" create configmap finops-overview-dashboard \
    --from-file=finops-overview.json="${HERE}/dashboards/finops-overview.json" \
    --dry-run=client -o yaml | kubectl apply -f -
  kubectl -n "${MONITORING_NAMESPACE}" label configmap finops-overview-dashboard \
    grafana_dashboard="1" --overwrite
fi

# 7. Summary ----------------------------------------------------------------------
log "waiting for the demo report to be produced..."
sleep 20
kubectl -n "${DEMO_NAMESPACE}" get aiprov,aimodel,aigw,aibudget,aisov,aireport 2>/dev/null || true

cat <<EOF

============================================================
 ai-finops-operator is up on kind cluster '${CLUSTER_NAME}'.

 Verify:
   kubectl -n ${NAMESPACE} get deploy
   kubectl -n ${DEMO_NAMESPACE} get aireport monthly-demo-report -o yaml
   kubectl -n ${DEMO_NAMESPACE} get configmap monthly-demo-report-report -o jsonpath='{.data.report\\.md}'

 Console (graphical UI, running IN the cluster next to the operator):
   kubectl -n ${NAMESPACE} port-forward svc/${HELM_RELEASE}-ai-finops-operator-console 8090:8090
   -> http://localhost:8090

 Metrics:
   kubectl -n ${NAMESPACE} port-forward svc/${HELM_RELEASE}-ai-finops-operator-metrics 8080:8080
   curl -s localhost:8080/metrics | grep ai_finops_

 Grafana (admin / admin):
   kubectl -n ${MONITORING_NAMESPACE} port-forward svc/monitoring-grafana 3000:80
   -> http://localhost:3000  (dashboard: "AI FinOps Operator — Overview")

 Tear down:  ./down.sh
============================================================
EOF
