# Disable make's implicit rules, which are not useful for golang, and slow down the build
# considerably.
.SUFFIXES:

REV=v1.0.0

NAME=krome-manager

CURRENT_DIR=$(shell pwd)

all: fmt build test

# Run tests
test: generate fmt vet manifests
	go test ./pkg/... ./cmd/... -coverprofile cover.out

build:
	mkdir -p bin
	echo "Building server..."
	CGO_ENABLED=0 GOOS=linux go build -v -i -ldflags '-X main.version=$(REV) ' -o ./bin/${NAME} ./cmd/$*

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Build the docker image
docker-build: #test
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml ./config/manager/all_in_one.yaml

# Push the docker image
docker-push:
	docker push ${IMG}
