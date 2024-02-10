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
	jobs -p | xargs kill -SIGSTOP
	jobs -p | xargs kill -9
  # Kill dangling port forwards if found.
  kill_main_exporter_port
	# Debug Port
	sudo ss -aK '( dport = :30002 or sport = :30002 )'
  # Prometheus Port
  sudo ss -aK '( dport = :9090 or sport = :9090 )' || true
  # Pprof Port
  sudo ss -aK '( dport = :6060 or sport = :6060 )' || true
	sudo ss -aK '( dport = :9000 or sport = :9000 )' || true
	sudo ss -aK '( dport = :3000 or sport = :3000 )' || true
	sudo ss -aK '( dport = :4040 or sport = :4040 )' || true
	} &> /dev/null
}


function add_test_clients() {
  while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l k8s-app=$DEPLOYMENT_NAME -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo  "waiting for metrics pod to start. Sleep 10" && sleep 10
done
	kubectl apply -f tests/resources/debug_service.yaml
}
