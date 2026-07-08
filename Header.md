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

## Overview

A Prometheus exporter for Kubernetes ephemeral storage. Emits per-node, per-pod, per-container, and per-volume metrics sourced from the kubelet `/stats/summary` endpoint (default) or the Kubernetes apiserver (`SCRAPE_FROM_KUBELET=false`).

### Metric groups

- **Node-level**: available / capacity / percentage of node ephemeral storage
- **Pod-level**: usage (bytes), inodes / inodes free / inodes used
- **Per-container rootfs + logs**: used / available / capacity bytes, usage percentage, inodes / inodes free / inodes used
- **Per-container volume (emptyDir)**: usage bytes, limit percentage

### Labels

Every metric carries `node_name`. Pod/container metrics add `pod_name`, `pod_namespace`, `container`. Volume metrics add `volume_name`, `mount_path`.

### DaemonSet vs Deployment

- **DaemonSet** (default): one exporter per node, scrapes local kubelet. Lighter apiserver load. Set `deploy_type: DaemonSet`.
- **Deployment**: single controller, lists all pods/nodes. Use `deploy_type: Deployment` plus optional `node_label_selector` to filter nodes (e.g. `type=virtual-kubelet` to exclude virtual nodes).

For large clusters (2K+ nodes), set `list_pods_with_cache: true` to read pod lists from the apiserver cache and reduce apiserver pressure. Pair with `deploy_type: DaemonSet` so each pod only lists its own node's pods (`spec.nodeName` fieldSelector).

### Not monitored

This project does not monitor CSI-backed ephemeral storage, e.g. [Generic ephemeral volumes](https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#generic-ephemeral-volumes).


