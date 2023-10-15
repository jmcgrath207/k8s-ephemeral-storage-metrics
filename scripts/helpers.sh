
function trap_func() {
  set +e
  {
	helm delete $DEPLOYMENT_NAME -n $DEPLOYMENT_NAME
	jobs -p | xargs kill -SIGSTOP
	jobs -p | xargs kill -9
	sudo ss -aK '( dport = :9100 or sport = :9100 )'
	} &> /dev/null
}


function add_test_clients() {
  while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l k8s-app=$DEPLOYMENT_NAME -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo  "waiting for metrics pod to start. Sleep 10" && sleep 10
done
	kubectl apply -f tests/resources/debug_service.yaml
}
