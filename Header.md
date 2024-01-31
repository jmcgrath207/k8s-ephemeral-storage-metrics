# K8s Ephemeral Storage Metrics

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Actions Status](https://github.com/jmcgrath207/k8s-ephemeral-storage-metrics/workflows/ci/badge.svg)](https://github.com/jmcgrath207/k8s-ephemeral-storage-metrics/actions)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/k8s-ephemeral-storage-metrics)](https://artifacthub.io/packages/helm/k8s-ephemeral-storage-metrics/k8s-ephemeral-storage-metrics)

A prometheus ephemeral storage metric exporter for pods, containers,
nodes, and volumes.

This project was created
to address lack of monitoring in [Kubernetes](https://github.com/kubernetes/kubernetes/issues/69507)

This project does not monitor CSI backed ephemeral storage ex. [Generic ephemeral volumes](https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#generic-ephemeral-volumes)

![main image](img/screenshot.png)


