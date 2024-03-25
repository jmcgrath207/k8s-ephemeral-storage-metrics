#!/bin/bash

minikube delete || true
c=$(docker ps -q) && [[ $c ]] && docker kill $c
docker network prune -f
minikube start \
  --kubernetes-version="${K8S_VERSION}" \
  --insecure-registry "10.0.0.0/24" \
  --cpus=2 \
  --memory=3900MB
minikube addons enable registry

# Add Service Monitor CRD
kubectl apply --server-side -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.65.1/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml
