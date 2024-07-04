#!/bin/bash

readonly CRD_BASE_URL=https://raw.githubusercontent.com/prometheus-operator/prometheus-operator
: "${PROMETHEUS_OPERATOR_VERSION:=v0.65.1}"

minikube delete || true
c=$(docker ps -q) && [[ $c ]] && docker kill $c
docker network prune -f
minikube start \
  --kubernetes-version="${K8S_VERSION}" \
  --insecure-registry "10.0.0.0/24" \
  --cpus=2 \
  --memory=3900MB
minikube addons enable registry

# Deploy Service Monitor and Prometheus Rule CRDs
for crd in monitoring.coreos.com_{servicemonitors,prometheusrules}.yaml; do
    kubectl apply --server-side -f \
        "$CRD_BASE_URL/${PROMETHEUS_OPERATOR_VERSION}/example/prometheus-operator-crd/$crd" || exit 1
done
