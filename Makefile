.EXPORT_ALL_VARIABLES:
.ONESHELL: # Applies to every targets in the file!

GITROOT ?= $(shell pwd)
DEPLOYMENT_NAME = ephemeral-metrics
K8S_VERSION ?= 1.27.0

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)


ginkgo:
	test -s $(LOCALBIN)/ginkgo || GOBIN=$(LOCALBIN) go install github.com/onsi/ginkgo/v2/ginkgo@v2.17.1

crane:
	test -s $(LOCALBIN)/crane || GOBIN=$(LOCALBIN) go install github.com/google/go-containerregistry/cmd/crane@latest

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

minikube_new:
	./scripts/create-minikube.sh

minikube_scale_up:
	minikube node add

minikube_scale_down:
	minikube node delete m02

init: fmt vet

deploy_debug: init
	ENV='debug' ./scripts/deploy.sh

deploy_e2e_debug: init
	ENV='e2e-debug' ./scripts/deploy.sh

deploy_local: init
	./scripts/deploy.sh

deploy_observability:
	ENV='observability' ./scripts/deploy.sh

deploy_e2e: init ginkgo crane minikube_new
	ENV='e2e' ./scripts/deploy.sh

deploy_e2e_dirty: init
	ENV='e2e' ./scripts/deploy.sh

deploy_many_pods:
	helm install many-pods ./tests/charts/many-pods -n many-pods --create-namespace

destroy_many_pods:
	helm delete -n many-pods many-pods

release-docker:
	GITHUB_TOKEN="${GITHUB_TOKEN}" VERSION="${VERSION}" ./scripts/release-docker.sh

release-helm:
	cd chart
	sed -i "s/tag:.*/tag: ${VERSION}/g" values.yaml
	sed -i "s/version:.*/version: ${VERSION}/g" Chart.yaml
	sed -i "s/appVersion:.*/appVersion: ${VERSION}/g" Chart.yaml
	helm package .
	helm repo index --merge index.yaml .
	sed -i "s!k8s-ephemeral-storage-metrics-${VERSION}.tgz!https://github.com/jmcgrath207/k8s-ephemeral-storage-metrics/releases/download/${VERSION}/k8s-ephemeral-storage-metrics-${VERSION}.tgz!g" index.yaml
	cd ..

release: release-docker release-helm helm-docs
	# Prod release
	# ex. make VERSION=1.6.2 release
	# Prerelease Candidate
	# ex. make VERSION=1.6.3-rc01 release

prerelease-github:
	# Prerelease Candidate
	# ex. make VERSION=1.6.3-rc01 prerelease-github
	gh release create ${VERSION} --generate-notes --prerelease
	gh release upload ${VERSION} "chart/k8s-ephemeral-storage-metrics-${VERSION}.tgz"
	rm chart/k8s-ephemeral-storage-metrics-*.tgz

release-github:
	# Prod release
	# ex. make VERSION=1.6.2 release-github
	gh release create ${VERSION} --generate-notes
	gh release upload ${VERSION} "chart/k8s-ephemeral-storage-metrics-${VERSION}.tgz"
	rm chart/k8s-ephemeral-storage-metrics-*.tgz

github_login:
	gh auth login --web --scopes=read:packages,write:packages