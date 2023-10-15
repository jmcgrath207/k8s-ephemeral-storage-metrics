.EXPORT_ALL_VARIABLES:

GITROOT ?= $(shell pwd)
DEPLOYMENT_NAME = ephemeral-metrics
K8S_VERSION ?= 1.26.0

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)


ginkgo:
	test -s $(LOCALBIN)/ginkgo || GOBIN=$(LOCALBIN) go install github.com/onsi/ginkgo/v2/ginkgo@v2.9.7

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

helm-docs:
	test -s $(LOCALBIN)/helm-docs || GOBIN=$(LOCALBIN) go install github.com/norwoodj/helm-docs/cmd/helm-docs@latest
	$(LOCALBIN)/helm-docs  --template-files "${GITROOT}/chart/README.md.gotmpl"
	cat "${GITROOT}/Header.md" "${GITROOT}/chart/README.md" > "${GITROOT}/README.md"

create_kind:
	./scripts/create_kind.sh

delete_kind:
	kind delete clusters "${DEPLOYMENT_NAME}-cluster"

init: fmt vet

deploy_debug: init
	ENV='debug' ./scripts/deploy.sh

deploy_e2e_debug: init
	ENV='e2e-debug' ./scripts/deploy.sh

deploy_local: init
	./scripts/deploy.sh

deploy_e2e: init ginkgo delete_kind create_kind
	ENV='e2e' ./scripts/deploy.sh

