# K8s Ephemeral Storage Metrics.


The goal of this project is to export ephemeral storage metric usage per pod to Prometheus that is address in this 
issue [Here](https://github.com/kubernetes/kubernetes/issues/69507)

Currently, this image is not being hosted and so you have to build it yourself at the moment. 


![main image](img/screenshot.png)


## Helm Install

```bash
helm repo add par https://jmcgrath207.github.io/par/chart
helm install par par/Par
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| deploy_type | string | `"DaemonSet"` |  |
| image.imagePullPolicy | string | `"ifNotPresent"` |  |
| image.repository | string | `"registry.lab.com/k8s-ephemeral-storage-metrics"` |  |
| image.tag | string | `""` |  |
| interval | string | `"15s"` |  |
| log_level | string | `"info"` |  |
| prometheus.release | string | `"kube-prometheus"` |  |

## Contribute

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