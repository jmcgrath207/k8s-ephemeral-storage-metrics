#!/bin/bash

function kill_main_exporter_port {
  # Main Exporter Port
  {
  sudo ss -aK '( dport = :9100 or sport = :9100 )' || true
	} &> /dev/null
}

function trap_func() {
  set +e
  {
    helm delete $DEPLOYMENT_NAME -n $DEPLOYMENT_NAME
    helm delete observability -n observability || true
    jobs -p | xargs kill -SIGSTOP
    jobs -p | xargs kill -9
    # Kill dangling port forwards if found.
    kill_main_exporter_port
    # Debug Port
    sudo ss -aK '( dport = :30002 or sport = :30002 )' || true
    # Prometheus
    sudo ss -aK '( dport = :9090 or sport = :9090 )' || true
    # Pprof
    sudo ss -aK '( dport = :6060 or sport = :6060 )' || true
    # Prometheus
    sudo ss -aK '( dport = :9000 or sport = :9000 )' || true
    # Grafana
    sudo ss -aK '( dport = :3000 or sport = :3000 )' || true
    # Pryoscope
    sudo ss -aK '( dport = :4040 or sport = :4040 )' || true
  } &>/dev/null
}

function add_test_clients() {
  while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l k8s-app=$DEPLOYMENT_NAME -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo "waiting for metrics pod to start. Sleep 10" && sleep 10
  done
  kubectl apply -f tests/resources/debug_service.yaml
}

function deploy_observability() {
  local chart_path
  chart_path="../tests/charts/observability"

  cd "${chart_path}" || exit
  helm dep up
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
    printf "\n\n" && while :; do kubectl port-forward -n observability service/prometheus-operated 9090:9090 || sleep 5; done
  ) &

  # Start Grafana Port Forward
  (
    sleep 10
    printf "\n\n" && while :; do kubectl port-forward -n observability service/grafana-service 3000:3000 || sleep 5; done
  ) &

  # Start Pyroscope Port Forward
  (
    sleep 10
    printf "\n\n" && while :; do kubectl port-forward -n observability service/observability-pyroscope 4040:4040 || sleep 5; done
  ) &
}

function follow_main_logs() {

  # Background log following for manager
  (
    sleep 10
    printf "\n\n" && while :; do kubectl logs -n $DEPLOYMENT_NAME -l app.kubernetes.io/name=k8s-ephemeral-storage-metrics -f || sleep 5; done
  ) &
}

function wait_pods() {

  # Wait until main pod comes up
  while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l app.kubernetes.io/name=k8s-ephemeral-storage-metrics -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo "Waiting for k8s-ephemeral-storage-metrics pod to start. Sleep 10" && sleep 10
  done

  # Wait until grow-test comes up
  while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l name=grow-test -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo "Waiting for grow-test pod to start. Sleep 10" && sleep 10
  done

  # Wait until shrink-test comes up
  while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l name=shrink-test -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo "Waiting for shrink-test pod to start. Sleep 10" && sleep 10
  done

  if [[ $ENV == "observability" ]]; then
    echo ""
  fi
}
