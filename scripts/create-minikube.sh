#!/bin/bash

readonly CRD_BASE_URL=https://raw.githubusercontent.com/prometheus-operator/prometheus-operator
: "${PROMETHEUS_OPERATOR_VERSION:=v0.65.1}"

minikube delete || true
c=$(docker ps -q) && [[ $c ]] && docker kill $c
docker network prune -f
minikube start \
  --kubernetes-version="${K8S_VERSION}" \
  --insecure-registry "0.0.0.0/0" \
  --addons="registry" \
  --cpus=2 \
  --cni='calico' \
  --memory=3900MB \
  --driver="${DRIVER}"

# minikube registry-proxy doesn't work well on other nodes.
# kubectl patch daemonset -n kube-system registry-proxy -p '{"spec":{"template":{"spec":{"nodeSelector":{"kubernetes.io/hostname":"minikube"}}}}}'

# Wait until registry pod come up
while [ "$(kubectl get pods -n kube-system -l actual-registry=true -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
  echo "Waiting for registry pod to start. Sleep 10" && sleep 10
done

# Wait until registry-proxy pod come up
while [ "$(kubectl get pods -n kube-system -l registry-proxy=true -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
  echo "Waiting for registry proxy pod to start. Sleep 10" && sleep 10
done

# Use a while loop to repeatedly check the registry endpoint until health
while true; do
 # Send a GET request to the endpoint and capture the HTTP status code
 status_code=$(curl -s -o /dev/null -w "%{http_code}" "http://$(minikube ip):5000/v2/_catalog")

 # Check if the status code is 200
 if [ "$status_code" -eq 200 ]; then
    echo "Registry endpoint is healthy. Status code: $status_code"
    break
 else
    echo "Registry endpoint is not healthy. Status code: $status_code. Retrying..."
    sleep 5 # Wait for 5 seconds before retrying
 fi
done



# Deploy Service Monitor and Prometheus Rule CRDs
for crd in monitoring.coreos.com_{servicemonitors,prometheusrules}.yaml; do
    kubectl apply --server-side -f \
        "$CRD_BASE_URL/${PROMETHEUS_OPERATOR_VERSION}/example/prometheus-operator-crd/$crd" || exit 1
done
