.EXPORT_ALL_VARIABLES:

GITROOT ?= $(shell pwd)
DEPLOYMENT_NAME = ephemeral-metrics
K8S_VERSION ?= 1.27.0

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

new_kind:
	./scripts/create_kind.sh

init: fmt vet

deploy_debug: init
	ENV='debug' ./scripts/deploy.sh

deploy_e2e_debug: init
	ENV='e2e-debug' ./scripts/deploy.sh

deploy_local: init
	./scripts/deploy.sh

deploy_e2e: init ginkgo new_kind
	ENV='e2e' ./scripts/deploy.sh

release-docker:
	GITHUB_TOKEN="${GITHUB_TOKEN}" VERSION="${VERSION}" ./scripts/release-docker.sh

release: github_login release-docker helm-docs
	# ex. make VERSION=1.0.1 release

	helm package chart --destination chart
	helm repo index --merge index.yaml chart/.

release-github: github_login
	# ex. make VERSION=1.0.1 release-github
	gh release create ${VERSION} --generate-notes

github_login:
	gh auth login --web --scopes=read:packages,write:packages