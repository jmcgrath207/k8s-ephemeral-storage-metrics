# K8s Ephemeral Storage Metrics

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Actions Status](https://github.com/jmcgrath207/k8s-ephemeral-storage-metrics/workflows/ci/badge.svg)](https://github.com/jmcgrath207/k8s-ephemeral-storage-metrics/actions)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/k8s-ephemeral-storage-metrics)](https://artifacthub.io/packages/helm/k8s-ephemeral-storage-metrics/k8s-ephemeral-storage-metrics)
![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/jmcgrath207/k8s-ephemeral-storage-metrics/total)

A prometheus ephemeral storage metric exporter for pods, containers,
nodes, and volumes.

This project was created
to address lack of monitoring in [Kubernetes](https://github.com/kubernetes/kubernetes/issues/69507)

This project does not monitor CSI backed ephemeral storage ex. [Generic ephemeral volumes](https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#generic-ephemeral-volumes)

![main image](img/screenshot.png)


## Helm Install

```bash
helm repo add k8s-ephemeral-storage-metrics https://jmcgrath207.github.io/k8s-ephemeral-storage-metrics/chart
helm repo update
helm upgrade --install my-deployment k8s-ephemeral-storage-metrics/k8s-ephemeral-storage-metrics
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| client_go_burst | int | `10` | Maximum burst for throttle. |
| client_go_qps | int | `5` | QPS indicates the maximum QPS to the master from this client. |
| containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| containerSecurityContext.privileged | bool | `false` |  |
| containerSecurityContext.readOnlyRootFilesystem | bool | `false` |  |
| containerSecurityContext.runAsNonRoot | bool | `true` |  |
| deploy_labels | object | `{}` | Set additional labels for the Deployment/Daemonset |
| deploy_type | string | `"Deployment"` | Set as Deployment for single controller to query all nodes or Daemonset |
| dev | object | `{"enabled":false,"grow":{"image":"ghcr.io/jmcgrath207/k8s-ephemeral-storage-grow-test:latest","imagePullPolicy":"IfNotPresent"},"shrink":{"image":"ghcr.io/jmcgrath207/k8s-ephemeral-storage-shrink-test:latest","imagePullPolicy":"IfNotPresent"}}` | For local development or testing that will deploy grow and shrink pods and debug service |
| image.imagePullPolicy | string | `"IfNotPresent"` |  |
| image.imagePullSecrets | list | `[]` |  |
| image.repository | string | `"ghcr.io/jmcgrath207/k8s-ephemeral-storage-metrics"` |  |
| image.tag | string | `"1.18.0"` |  |
| interval | int | `15` | Polling node rate for exporter |
| kubelet | object | `{"insecure":false,"readOnlyPort":0,"scrape":false}` | Scrape metrics through kubelet instead of kube api |
| log_level | string | `"info"` |  |
| max_node_concurrency | int | `10` | Max number of concurrent query requests to the kubernetes API. |
| metrics | object | `{"adjusted_polling_rate":false,"ephemeral_storage_container_limit_percentage":true,"ephemeral_storage_container_volume_limit_percentage":true,"ephemeral_storage_container_volume_usage":true,"ephemeral_storage_inodes":true,"ephemeral_storage_node_available":true,"ephemeral_storage_node_capacity":true,"ephemeral_storage_node_percentage":true,"ephemeral_storage_pod_usage":true,"port":9100}` | Set metrics you want to enable |
| metrics.adjusted_polling_rate | bool | `false` | Create the ephemeral_storage_adjusted_polling_rate metrics to report Adjusted Poll Rate in milliseconds. Typically used for testing. |
| metrics.ephemeral_storage_container_limit_percentage | bool | `true` | Percentage of ephemeral storage used by a container in a pod |
| metrics.ephemeral_storage_container_volume_limit_percentage | bool | `true` | Percentage of ephemeral storage used by a container's volume in a pod |
| metrics.ephemeral_storage_container_volume_usage | bool | `true` | Current ephemeral storage used by a container's volume in a pod |
| metrics.ephemeral_storage_inodes | bool | `true` | Current ephemeral inode usage of pod |
| metrics.ephemeral_storage_node_available | bool | `true` | Available ephemeral storage for a node |
| metrics.ephemeral_storage_node_capacity | bool | `true` | Capacity of ephemeral storage for a node |
| metrics.ephemeral_storage_node_percentage | bool | `true` | Percentage of ephemeral storage used on a node |
| metrics.ephemeral_storage_pod_usage | bool | `true` | Current ephemeral byte usage of pod |
| metrics.port | int | `9100` | Adjust the metric port as needed (default 9100) |
| nodeSelector | object | `{}` |  |
| podAnnotations | object | `{}` |  |
| pod_labels | object | `{}` | Set additional labels for the Pods |
| podSecurityContext.runAsNonRoot | bool | `true` |  |
| podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| pprof | bool | `false` | Enable Pprof |
| priorityClassName | string | `nil` |  |
| prometheus.additionalLabels | object | `{}` | Add labels to the PrometheusRule.Spec |
| prometheus.enable | bool | `true` |  |
| prometheus.release | string | `"kube-prometheus-stack"` |  |
| prometheus.rules.enable | bool | `false` | Create PrometheusRules firing alerts when out of ephemeral storage |
| prometheus.rules.labels | object | `{"severity":"warning"}` | What additional labels to set on alerts |
| prometheus.rules.predictFilledHours | int | `12` | How many hours in the future to predict filling up of a volume |
| prometheus.rules.predictMinCurrentUsage | float | `33.3` | What percentage of limit must be used right now to predict filling up of a volume |
| rbac | object | `{"create":true}` | RBAC configuration |
| revisionHistoryLimit | int | `10` | Revision history limit for the Deployment |
| serviceAccount | object | `{"create":true,"name":null}` | Service Account configuration |
| serviceMonitor | object | `{"additionalLabels":{},"enable":true,"metricRelabelings":[],"podTargetLabels":[],"relabelings":[],"targetLabels":[]}` | Configure the Service Monitor |
| serviceMonitor.additionalLabels | object | `{}` | Add labels to the ServiceMonitor.Spec |
| serviceMonitor.metricRelabelings | list | `[]` | Set metricRelabelings as per https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/api.md#monitoring.coreos.com/v1.RelabelConfig |
| serviceMonitor.podTargetLabels | list | `[]` | Set podTargetLabels as per https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/api.md#monitoring.coreos.com/v1.ServiceMonitorSpec |
| serviceMonitor.relabelings | list | `[]` | Set relabelings as per https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/api.md#monitoring.coreos.com/v1.RelabelConfig |
| serviceMonitor.targetLabels | list | `[]` | Set targetLabels as per https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/api.md#monitoring.coreos.com/v1.ServiceMonitorSpec |
| tolerations | list | `[]` |  |

## Prometheus alert rules

To prevent from multiple kind of alerts being fired for a single container or
emptyDir volume when both `prometheus.enable` and `prometheus.rules.enable` are
on, add the following [inhibition
rules](https://prometheus.io/docs/alerting/latest/configuration/#inhibition-related-settings)
to your Alert Manager config:

```yaml
- source_matchers:
    - alertname="EphemeralStorageVolumeFilledUp"
  target_matchers:
    - severity="warning"
    - alertname="EphemeralStorageVolumeFillingUp"
  equal:
    - pod_namespace
    - pod_name
    - volume_name
- source_matchers:
    - alertname="ContainerEphemeralStorageUsageAtLimit"
  target_matchers:
    - severity="warning"
    - alertname="ContainerEphemeralStorageUsageReachingLimit"
  equal:
    - pod_namespace
    - pod_name
    - exported_container
```

## Contribute

### Start minikube
```bash
make new_minikube
```

### Run locally
```bash
make deploy_local
```

### Run locally with Delve Debug
```bash
make deploy_debug
```
Then connect to `localhost:30002` with [delve](https://github.com/go-delve/delve) or your IDE.

### Run e2e Test
```bash
make deploy_e2e
```

### Debug e2e
```bash
make deploy_e2e_debug
```
Then run a debug against [deployment_test.go](tests/e2e/deployment_test.go)

## License

This project is licensed under the [MIT License](https://opensource.org/licenses/MIT). See the `LICENSE` file for more details.
