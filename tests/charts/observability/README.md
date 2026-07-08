## Helm Install

```bash
helm repo add k8s-ephemeral-storage-metrics https://jmcgrath207.github.io/k8s-ephemeral-storage-metrics/chart
helm repo update
helm upgrade --install my-deployment k8s-ephemeral-storage-metrics/k8s-ephemeral-storage-metrics
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| grafana-operator.enabled | bool | `true` |  |
| kube-prometheus-stack.alertmanager.enabled | bool | `false` |  |
| kube-prometheus-stack.enabled | bool | `true` |  |
| kube-prometheus-stack.grafana.enabled | bool | `false` |  |
| kube-prometheus-stack.kube-state-metrics.enabled | bool | `false` |  |
| kube-prometheus-stack.kubeApiServer.enabled | bool | `false` |  |
| kube-prometheus-stack.kubeControllerManager.enabled | bool | `false` |  |
| kube-prometheus-stack.kubelet.enabled | bool | `false` |  |
| kube-prometheus-stack.kubernetesServiceMonitors.enabled | bool | `false` |  |
| kube-prometheus-stack.prometheus-node-exporter.enabled | bool | `false` |  |
| kube-prometheus-stack.prometheus.prometheusSpec.podMonitorSelectorNilUsesHelmValues | bool | `false` |  |
| kube-prometheus-stack.prometheus.prometheusSpec.probeSelectorNilUsesHelmValues | bool | `false` |  |
| kube-prometheus-stack.prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues | bool | `false` |  |
| pyroscope.enabled | bool | `true` |  |

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
