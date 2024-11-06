#!/bin/bash

set -e

CURRENT_DIR="$(realpath $(dirname ${BASH_SOURCE[0]}))"
cd "${CURRENT_DIR}" || true

source helpers.sh

function main() {
  local image_tag
  local dockerfile
  local common_set_values
  local common_set_values_arr
  local grow_repo_image
  local shrink_repo_image
  local main_repo_image
  local e2e_values_arr
  local external_registry
  local internal_registry
  local time_tag

  trap 'trap_func' EXIT ERR

  # Need both. External to push and internal for pods to pull from registry in cluster
  external_registry="$(minikube ip):5000"
  internal_registry="$(kubectl get service -n kube-system registry --template='{{.spec.clusterIP}}')"

  ### Build and Load Images ###

  time_tag=$(date +%Y%m%d%H%M%S)

  grow_repo_image="k8s-ephemeral-storage-grow-test:${time_tag}"
  dev_image_builder "${grow_repo_image}" "DockerfileTestGrow" "${external_registry}" "${internal_registry}"

  shrink_repo_image="k8s-ephemeral-storage-shrink-test:${time_tag}"
  dev_image_builder "${shrink_repo_image}" "DockerfileTestShrink" "${external_registry}" "${internal_registry}"


  if [[ $ENV == "debug" ]]; then
    image_tag="debug-${time_tag}"
    dockerfile="DockerfileDebug"
  else
    image_tag="${time_tag}"
    dockerfile="Dockerfile"
  fi

  # Main image
  main_repo_image="${DEPLOYMENT_NAME}:${image_tag}"
  dev_image_builder "${main_repo_image}" "${dockerfile}" "${external_registry}" "${internal_registry}"

  ### Install Chart ###

  common_set_values_arr=(
    "image.repository=${internal_registry}/$DEPLOYMENT_NAME"
    "image.tag=${image_tag}"
    "deploy_type=Deployment"
    "log_level=debug"
    "dev.enabled=true"
    "dev.shrink.image=${internal_registry}/${shrink_repo_image}"
    "dev.grow.image=${internal_registry}/${grow_repo_image}"
    "metrics.adjusted_polling_rate=true"
    "pprof=true"
    "prometheus.rules.enable=true"
  )

  if [[ $ENV =~ "e2e" ]]; then

    e2e_values_arr=("interval=5")
    common_set_values_arr+=("${e2e_values_arr[@]}")
  fi

  common_set_values=$(echo ${common_set_values_arr[*]} | sed 's/, /,/g' | sed 's/ /,/g')

  if [[ $ENV == "test" ]]; then
    helm upgrade --install "${DEPLOYMENT_NAME}" ../chart \
      -f ../chart/test-values.yaml \
      --create-namespace \
      --namespace "${DEPLOYMENT_NAME}"
  else
    # All other deployments
    helm upgrade --install "${DEPLOYMENT_NAME}" ../chart \
      --set "${common_set_values}" \
      --create-namespace \
      --namespace "${DEPLOYMENT_NAME}"
  fi

  # Patch deploy so minikube image upload works.
  if [[ $ENV == "debug" ]]; then
    # Disable for Debugging of Delve.
    kubectl patch deployments.apps -n "${DEPLOYMENT_NAME}" k8s-ephemeral-storage-metrics -p \
      '{ "spec": {"template": { "spec":{"securityContext": null, "containers":[{"name":"metrics", "livenessProbe": null, "readinessProbe": null, "securityContext": null, "command": null, "args": null  }]}}}}'
  fi

  # Start Exporter Port Forward
  (
    sleep 10
    printf "\n\n" && while :; do kubectl port-forward -n $DEPLOYMENT_NAME service/k8s-ephemeral-storage-metrics 9100:9100 || kill_main_exporter_port && sleep 5; done
  ) &

  wait_pods

  if [[ $ENV == "debug" ]]; then
    follow_main_logs
    kubectl port-forward -n $DEPLOYMENT_NAME services/debug 30002:30002

  elif [[ $ENV == "e2e" ]]; then
    ${LOCALBIN}/ginkgo -v -r ../tests/e2e/...
  elif [[ $ENV == "e2e-debug" ]]; then
    sleep infinity
  elif [[ $ENV == "observability" ]]; then
    deploy_observability
    follow_main_logs
    sleep infinity
  else
    # Assume ENV=local deploy
    follow_main_logs
    sleep infinity
  fi
}

main "$@"
