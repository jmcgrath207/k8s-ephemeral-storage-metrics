---
name: contribute
description: >
  Full contributor walkthrough for k8s-ephemeral-storage-metrics. Covers
  prerequisites, ENV modes, make target reference, e2e walkthrough, e2e test
  inventory, unit tests, kind alternative (cluster-only), troubleshooting,
  and release process.
  Use when user says "run e2e", "how to test", "contribute", "how to release",
  "troubleshoot", or invokes /contribute.
---

## Prerequisites

Install these once on your dev machine:

| Tool | Version | Notes |
|---|---|---|
| Docker | latest | Required for image builds (deploy.sh uses `docker build` + `crane push`) |
| minikube | latest | Cluster of choice; e2e is hardcoded to minikube node name |
| helm | 3.x | Chart install/upgrade |
| kubectl | matches K8S_VERSION | Cluster interaction |
| Go | 1.26.5 | Matches `go.mod` toolchain |

`bin/{ginkgo,crane,helm-docs,gosec,govulncheck}` are **auto-installed by the Makefile** on first use into the gitignored `bin/` directory. No manual setup needed.

## ENV modes

`scripts/deploy.sh` keys behavior off `ENV={local|debug|e2e|e2e-debug|test}`. All `make deploy_*` targets set this for you.

| Mode | Command | What it does |
|---|---|---|
| `local` (default) | `make deploy_local` | Build → helm install → port-forward :9100 → tail logs → hold |
| `debug` | `make deploy_debug` | Delve-instrumented build → patch out probes → port-forward :30002 → connect dlv |
| `e2e` | `make deploy_e2e` | Build → helm install → `ginkgo -v -r tests/e2e/...` (~15-25 min) |
| `e2e-debug` | `make deploy_e2e_debug` | Build → helm install → `sleep infinity` (run ginkgo manually, attach debugger) |
| `test` | `make deploy_test` | Helm install with `chart/test-values.yaml` overrides |

`make init` (= `fmt vet gosec`) runs before every deploy as a quality gate.

## Make targets reference

### Cluster
- `minikube_new_docker` — wipe + start minikube (calico CNI, registry addon, 3900MB)
- `minikube_new_virtualbox` — same but virtualbox driver
- `minikube_scale_up` / `minikube_scale_down` — add/remove minikube node (e2e Scaling Context)
- `minikube_node2_stop` / `minikube_node2_start` — docker stop/start minikube-m02 (e2e Node Disconnect Context)

### Deploy
- `deploy_local` / `deploy_debug` / `deploy_e2e` / `deploy_e2e_debug` / `deploy_test`
- `deploy_many_pods` / `destroy_many_pods` — fixture for e2e Scrape-Driven Eviction Context

### Test + lint
- `test-unit` — `go vet ./... && go test ./pkg/... ./cmd/...` (~2s, 61 tests, no cluster)
- `test-helm-render` — `helm template` against 8 k8s versions
- `fmt` / `vet` / `gosec` / `govulncheck`

### Docs
- `helm-docs` — regenerate `chart/README.md` + `README.md` from values.yaml

### Release (maintainer-only)
- `release-docker` / `release-helm` / `release` (chains docker+helm+helm-docs)
- `release-github` / `prerelease-github` — gh release create + upload tgz
- `github_login` — first-time gh auth with package scopes

### Tool install (auto, idempotent via `test -s`)
- `ginkgo` / `crane` / `govulncheck` / `gosec`

## E2E walkthrough

```
make minikube_new_docker    # ~2-3 min: cluster + calico + registry + CRDs
make deploy_e2e             # ~15-25 min: full ginkgo suite
```

`make deploy_e2e` chains `init test-helm-render ginkgo crane` then:
1. Builds 3 images: main + grow-test + shrink-test (each tagged with timestamp)
2. `crane push` each to minikube's in-cluster registry
3. `helm upgrade --install` with all metric flags on (rootfs/logs usage, adjusted_polling_rate, etc.)
4. Waits for main pod + grow-test + shrink-test to reach Running
5. Port-forwards :9100 to host
6. Runs `${LOCALBIN}/ginkgo -v -r tests/e2e/...`
7. On exit/err, `trap_func` cleans up: helm delete, kill port-forwards, ss -aK cleanup

Per-Context timeout: 180s. Suite teardown: ~30s.

## E2E test inventory

`tests/e2e/deployment_test.go` has **12 ginkgo Contexts** (plus a `BeforeSuite` that runs `scaleUp` to add node m02 before any test):

1. **Observe labels** — all expected metric names + label values present in `/metrics` output
2. **Test Polling speed** — `ephemeral_storage_adjusted_polling_rate` between 4000-5000ms
3. **pod_usage grow/shrink** — `ephemeral_storage_pod_usage` ±100k bytes (grow-test adds 12MB then decrements; shrink-test adds 12MB then drops 12k/sec)
4. **container_limit_percentage grow/shrink** — ±0.2%
5. **container_volume_limit_percentage grow/shrink** — ±0.2% (mount_path=/cache, volume_name=cache-volume-1)
6. **container_volume_usage grow/shrink** — ±100k bytes
7. **container_rootfs_used_bytes grow/shrink** — ±100k bytes
8. **pct not over 100** — node / container / container_volume all <100%
9. **Test Scaling** — asserts m02 metrics present (scale-up already done by BeforeSuite)
10. **Test Node Disconnect Inode Leak** — `docker stop minikube-m02` → 10s wait → assert inode metrics absent → assert evicted metrics absent (regression test for Bug 1 / PR #194)
11. **Test Scrape-Driven Eviction** — deploy 50+ pods → verify metrics → delete pods → wait 90s → verify evicted from Prometheus
12. **Test Scale Down** — reconnect m02 → `minikube node delete m02` → assert all m02 metrics absent

Watch helpers in `deployment_test.go`: `WatchEphemeralSize` (generic), `WatchContainerPercentage`, `WatchContainerVolumePercentage`, `WatchNodePercentage`, `WatchPollingRate`. Getters: `getPodUsageSize`, `getContainerLimitPercentage`, `getContainerVolumeLimitPercentage`, `getContainerVolumeUsage`, `getContainerRootfsUsedBytes`.

## Unit tests

```
make test-unit    # ~2s, 61 tests, no cluster
```

Coverage by package: pkg/dev 73.8%, pkg/pod 64.3%, pkg/node 25.8%, cmd/app 0% (untestable without source mods — `setMetrics` calls `Node.Query` which needs real clientset; cannot mock `Node` since it's concrete not interface). **Total: 47.2%.**

Ceiling ~47% is blocked by: `Query` (needs real kubelet), `Watch` (infinite loop), `gcMetrics` (time.Ticker), `NewCollector` (os.Exit + createMetrics double-register), `initGetPodsData`/`podWatch` (needs dev.Clientset), `EnablePprof` (server), `setMetrics`/`getMetrics`/`main` (call Query or flag-parse).

To reach 80% requires source mods: Node/Pod as interfaces, replace os.Exit with error returns, context cancellation on loops, extract setupServer() for testable main. **Out of scope** unless maintainer requests.

## kind alternative

`scripts/create_kind.sh` exists (no Makefile wrapper) for cluster bring-up using `kind` instead of minikube. **Use it for dev iteration only — e2e will fail.**

Why: e2e hardcodes `node_name="minikube"` in:
- `WatchNodePercentage:163` — `re := regexp.MustCompile(\`ephemeral_storage_node_percentage\{node_name="minikube"}.+`)` 
- `checkSlice:328-430` — every `node_name="minikube"` literal

If you need a kind-passing e2e, those regexes need an env var. Out of scope here.

## Label cardinality

Each metric label multiplies time-series count and raises Prometheus memory/ingestion cost. Keep the exporter's label surface minimal.

**Do not mirror Kubernetes node/pod labels** (`agentpool`, `nodegroup`, `zone`, `instance-type`, `node-role`, `os`, ...) into exporter metrics. Direct users to join `kube_node_labels` / `kube_pod_labels` from [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics) at query time:

```
ephemeral_storage_node_percentage
  * on (node_name) group_left(label_agentpool)
    kube_node_labels{label_agentpool!=""}
```

This keeps the exporter lean, works for any node/pod label, and sidesteps cloud-provider label-key naming debates. See `AGENTS.md` rule #5 and issue #131 for precedent.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Port conflict on :9100 / :30002 / :9090 / :6060 | Previous run left a port-forward | `sudo ss -aK '( sport = :PORT )'` (or `trap_func` in `scripts/helpers.sh` if it ran) |
| e2e hang at "Waiting for k8s-ephemeral-storage-metrics pod to start" | Insufficient minikube memory, or image push failed | Minikube needs 3900MB floor. Check `docker exec minikube crictl images` for the just-pushed image |
| e2e hang at "Waiting for grow-test pod" | grow-test image not pushed to minikube registry | Check `deploy.sh` `crane push` output; minikube registry addon must be Running |
| "registry endpoint is not healthy" during minikube_new_docker | Normal startup race | `create-minikube.sh` retries every 5s; if stuck >2min, `minikube delete && make minikube_new_docker` |
| `ginkgo` panics on run | Makefile pin out of sync with go.mod | Delete `bin/ginkgo` and rerun `make ginkgo`. Pin must match `github.com/onsi/ginkgo/v2` in go.mod (currently v2.32.0) |
| `e2e` flakes on shrink-test | Known: shrinking reflects slowly from Node API (up to 5min) | `tests/e2e/deployment_test.go` waits for currentPodSize ≥ 11MB before testing |
