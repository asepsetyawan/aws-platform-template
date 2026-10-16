#!/usr/bin/env bash
set -euo pipefail
if [[ $# != 5 ]]; then
  echo "Usage: $0 CONTEXT CLUSTER_NAME REGION VPC_ID AUTOSCALER_IMAGE_TAG" >&2
  exit 2
fi
LAB_CONTEXT=$1
LAB_CLUSTER=$2
LAB_REGION=$3
LAB_VPC=$4
LAB_AUTOSCALER_TAG=$5
LAB_ROOT=$(cd "$(dirname "$0")/.." && pwd)
python - "$LAB_ROOT/platform/argocd.yaml" <<'PYTHON'
import sys, yaml
cm = yaml.safe_load(open(sys.argv[1]))["configs"]["cm"]
if not cm.get("oidc.config") and not cm.get("dex.config"):
    raise SystemExit("Configure Argo CD SSO in platform/argocd.yaml before bootstrap")
PYTHON
python "$LAB_ROOT/scripts/download_charts.py"
# Helm CLI performs real deployment. Use an operator with private API access for prod.
kubectl --context "$LAB_CONTEXT" get nodes
kubectl --context "$LAB_CONTEXT" apply -f "$LAB_ROOT/platform/storage.yaml"
helm upgrade --install metrics-server "$LAB_ROOT/.generated/charts/metrics-server.tgz" \
  --kube-context "$LAB_CONTEXT" --namespace kube-system --wait --timeout 10m \
  -f "$LAB_ROOT/platform/metrics-server.yaml"
helm upgrade --install aws-load-balancer-controller "$LAB_ROOT/.generated/charts/aws-load-balancer-controller.tgz" \
  --kube-context "$LAB_CONTEXT" --namespace kube-system --wait --timeout 10m \
  -f "$LAB_ROOT/platform/alb-controller.yaml" \
  --set-string "clusterName=$LAB_CLUSTER,region=$LAB_REGION,vpcId=$LAB_VPC"
helm upgrade --install cluster-autoscaler "$LAB_ROOT/.generated/charts/cluster-autoscaler.tgz" \
  --kube-context "$LAB_CONTEXT" --namespace kube-system --wait --timeout 10m \
  -f "$LAB_ROOT/platform/autoscaler.yaml" \
  --set-string "autoDiscovery.clusterName=$LAB_CLUSTER,awsRegion=$LAB_REGION,image.tag=$LAB_AUTOSCALER_TAG"
# Customize SSO in this values file before installation; bootstrap admin is disabled.
helm upgrade --install argocd "$LAB_ROOT/.generated/charts/argo-cd.tgz" \
  --kube-context "$LAB_CONTEXT" --namespace argocd --create-namespace --wait --timeout 15m \
  -f "$LAB_ROOT/platform/argocd.yaml"
