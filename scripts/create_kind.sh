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

if [[ ! $ENV =~ "e2e" ]]; then
  # Deploy Prometheus
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  helm repo update
  helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack --version 46.8.0 -n "${DEPLOYMENT_NAME}" --create-namespace \
  --set grafana.enabled=false \
  --set kubeApiServer.enabled=false \
  --set kubernetesServiceMonitors.enabled=false \
  --set kubelet.enabled=false \
  --set kubeControllerManager.enabled=false \
  --set kube-state-metrics.enabled=false \
  --set prometheus-node-exporter.enabled=false \
  --set alertmanager.enabled=false\
  --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
  --set prometheus.prometheusSpec.podMonitorSelectorNilUsesHelmValues=false \
  --set prometheus.prometheusSpec.probeSelectorNilUsesHelmValues=false
fi
