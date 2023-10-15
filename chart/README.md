## Helm Install

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| deploy_type | string | `"DaemonSet"` |  |
| dev.enabled | bool | `false` |  |
| image.imagePullPolicy | string | `"ifNotPresent"` |  |
| image.repository | string | `"registry.lab.com/k8s-ephemeral-storage-metrics"` |  |
| image.tag | string | `"asdf"` |  |
| interval | int | `15` |  |
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