# Disable make's implicit rules, which are not useful for golang, and slow down the build
# considerably.
.SUFFIXES:

REV=v1.0.0

NAME=krome-manager

CURRENT_DIR=$(shell pwd)

all: fmt build test

# Run tests
test: generate fmt vet
	go test ./pkg/... ./cmd/... -coverprofile cover.out

build:
	mkdir -p ./_output
	echo "Building server..."
	CGO_ENABLED=0 GOOS=linux go build -v -i -ldflags '-X main.version=$(REV) ' -o ./_output/${NAME} ./cmd/manager/$*

clean:
	rm -rf ./output

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy:
	kubectl apply -f config/crds

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Build the docker image
image:
	docker build -t ${NAME}:${REV} .

# Push the docker image
docker-push:
	docker push ${NAME}:${REV}
