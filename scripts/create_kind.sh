#!/bin/bash

readonly CRD_BASE_URL=https://raw.githubusercontent.com/prometheus-operator/prometheus-operator
: "${PROMETHEUS_OPERATOR_VERSION:=v0.65.1}"

function trap_func_kind() {
  kind export logs
  exit 1
}

trap 'trap_func_kind' ERR


kind delete clusters "${DEPLOYMENT_NAME}-cluster"

kind create cluster \
  --verbosity=6 \
  --config scripts/kind.yaml \
  --retain \
  --name "${DEPLOYMENT_NAME}-cluster" \
  --image "kindest/node:v${K8S_VERSION}"

kubectl config set-context "${DEPLOYMENT_NAME}-cluster"
echo "Kubernetes cluster:"
kubectl get nodes -o wide

# Deploy Service Monitor and Prometheus Rule CRDs
for crd in monitoring.coreos.com_{servicemonitors,prometheusrules}.yaml; do
    kubectl apply --server-side -f \
        "$CRD_BASE_URL/${PROMETHEUS_OPERATOR_VERSION}/example/prometheus-operator-crd/$crd" || exit 1
done
