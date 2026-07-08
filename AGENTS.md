# AGENTS.md

Project-local instructions for opencode agents working in this repo. Auto-loaded into every agent in this repo. Be concise — keep this file lean.

## Using opencode in this repo

- **Project-local skill**: `/contribute` — full contributor walkthrough (prereqs, e2e, observability, release, troubleshooting). Invoke via `/contribute` or ask "how do I run e2e" / "how to release" / "troubleshoot". Definition: `.opencode/skills/contribute/SKILL.md`.
- **Global user skills** (also available):
  - `caveman` — terse output mode
  - `golang-skills/*` — Go style, testing, error-handling, concurrency, context, interfaces
  - `cavecrew` — subagent delegation (investigator/builder/reviewer) for context savings
  - `superpowers-*` — brainstorming/spec/planning/execution/verification workflows
- **Pre-PR gate** (always run before declaring a PR ready):
  - `make fmt` — gofmt
  - `make vet` — go vet
  - `make test-unit` — 61 unit tests, ~2s, no cluster
  - `make test-helm-render` — chart renders cleanly across 8 k8s versions
- **Do NOT commit** (gitignored but agents should not stage):
  - `bin/` — Makefile-installed tools (ginkgo, crane, helm-docs, gosec, govulncheck)
  - `.idea/`, `.vscode/` — editor state
- **Coverage baseline**: 47.2% total. Ceiling ~47% without source mods (Query/Watch/gcMetrics/NewCollector/os.Exit unreachable from tests). Do not propose refactors to "raise coverage" unless user asks.
- **`/commit` available** — caveman-commit skill auto-triggers on staging, produces Conventional Commits format.

## Contribute rules

### Metric stability (don't break downstream Prometheus users)

1. **Never rename an existing metric.** Breaks dashboards, alerts, recording rules.
2. **Never remove a label from an existing metric.** Breaks PromQL `by(...)` aggregations.
3. **Never change a metric type** (Gauge→Counter, etc.). Breaks PromQL functions.
4. **Adding new metrics is OK** (additive). Follow naming: `ephemeral_storage_<scope>_<metric>_<unit>` where unit ∈ `{bytes, percentage, inodes}`.
5. **Adding new labels to an existing metric is OK** (additive, backward compatible).
6. **Adding a new metric requires updating** `tests/e2e/deployment_test.go` `checkSlice` (Observe labels Context) so e2e asserts presence.

### Versioning (maintainer-only)

**Never bump versions of any kind.** Maintainer handles all version bumps. If a contributor needs a version bump, open an issue or PR request — the maintainer will do it.

- **No Go version bumps** in `go.mod`, `Dockerfile`, `DockerfileDebug`, `DockerfileTestGrow`, `DockerfileTestShrink`, `.github/workflows/test.yaml`
- **No module version bumps** in `go.mod` / `go.sum` (direct or indirect) — including `go get -u`, `go get <module>@<ver>`, `go mod tidy` of updated deps
- **No chart version bumps** in `chart/values.yaml` (`image.tag`) or `chart/Chart.yaml` (`version`, `appVersion`)
- **No tool version bumps** in `Makefile` (e.g. ginkgo pin must track go.mod ginkgo version, but the bump is maintainer-only)
- **No GitHub Actions version bumps** in `.github/workflows/test.yaml` (e.g. `actions/checkout@v3`, `setup-go@v4`)

If a dependency is needed, request it. If a security advisory surfaces, the maintainer runs the bump.

### Code style

- `make fmt vet gosec` must all pass clean.
- No new direct dependency without justification in the PR description.
- Follow Go idioms — leverage `golang-skills` (go-naming, go-error-handling, go-testing, go-interfaces, go-concurrency, go-context, go-defensive).
- Match existing file conventions: receiver names, error wrapping (`fmt.Errorf("...: %w", err)`), package-level docs only for exported types/funcs.
- Comments only when necessary — no narration of obvious code.

### Commit conventions

- **Conventional Commits** format: `feat:`, `fix:`, `chore:`, `refactor:`, `docs:`, `test:`, `ci:`, `perf:`, `build:`
- Subject ≤50 chars. Body explains WHY not WHAT.
- Reference PR/issue number in body when applicable (`Closes #123`, `Refs #456`).
- No `Co-Authored-By:` line for AI unless the maintainer asks.

## Steps to contribute

1. **Prereqs installed**: Docker, minikube, helm, kubectl, Go 1.26.5
2. **Fork repo**, branch from `master`
3. **Local validation** (without cluster):
   - `make test-unit` — ~2s, 61 tests
4. **Local validation** (with cluster):
   - `make minikube_new_docker` — fresh minikube (calico + registry addon, 3900MB)
   - `make deploy_e2e` — builds 3 images, helm install, ginkgo e2e (~15-25 min)
5. **Make changes** following the rules above
6. **Pre-PR gate**: `make fmt vet test-unit test-helm-render`
7. **If `chart/values.yaml` changed**: run `make helm-docs` and commit regenerated `README.md` + `chart/README.md`
8. **Open PR** — CI auto-runs `unit` job (go vet + go test) and `e2e` job (minikube + ginkgo)

## Command quick ref

| Task | Command |
|---|---|
| Unit tests | `make test-unit` |
| e2e tests | `make deploy_e2e` |
| Lint + format | `make fmt vet gosec` |
| Helm render check | `make test-helm-render` |
| Regenerate docs | `make helm-docs` |
| Fresh minikube | `make minikube_new_docker` |
| Local deploy | `make deploy_local` |
| Local debug (Delve) | `make deploy_debug` |
