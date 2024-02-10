#!/bin/bash

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

# Deploy Service Monitor CRD
kubectl apply --server-side -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.65.1/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml
