# Disable make's implicit rules, which are not useful for golang, and slow down the build
# considerably.
.SUFFIXES:

REV=v1.0.0

NAME=krome-manager

CURRENT_DIR=$(shell pwd)

all: fmt build-exec image apply

# Run tests
test: fmt
	go test ./pkg/controller/... -coverprofile cover.out

build-exec: fmt
	mkdir -p ./_output
	echo "Building server..."
	CGO_ENABLED=0 GOOS=linux go build -v -i -ldflags '-X main.version=$(REV) ' -o ./_output/${NAME} ./cmd/manager/$*

clean:
	kubectl delete -f examples
	kubectl delete -f deploy
	rm -rf ./output

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
apply:
	kubectl apply -f deploy/

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
