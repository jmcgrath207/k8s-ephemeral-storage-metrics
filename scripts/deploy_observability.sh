#!/bin/bash

set -e

source scripts/helpers.sh

trap 'trap_func' EXIT ERR

cd observability
#helm dep up
helm upgrade --install observability . \
  --create-namespace \
  --namespace observability

helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/
helm repo update
helm upgrade --install --set args={--kubelet-insecure-tls} metrics-server metrics-server/metrics-server --namespace kube-system

# Start Pprof Forward
(
  sleep 10
  printf "\n\n" && while :; do kubectl port-forward -n $DEPLOYMENT_NAME service/pprof 6060:6060 || sleep 5; done
) &


# Start Prometheus Port Forward
(
  sleep 10
  printf "\n\n" && while :; do kubectl port-forward -n observability service/prometheus-operated  9090:9090 || sleep 5; done
) &

# Start Grafana Port Forward
(
  sleep 10
  printf "\n\n" && while :; do kubectl port-forward -n observability service/grafana-service  3000:3000 || sleep 5; done
) &

# Start Pyroscope Port Forward
(
  sleep 10
  printf "\n\n" && while :; do kubectl port-forward -n observability service/observability-pyroscope  4040:4040 || sleep 5; done
) &



sleep infinity

