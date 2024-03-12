IMG=ghcr.io/awesomenix/orchestrator:latest

all: lint test app

app: orchestratorapp testapp

# Run tests
test:
	go test -timeout 120s ./... -coverprofile cover.out
	go tool cover -func=cover.out
# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

lint: fmt vet
	golangci-lint run --enable=testifylint

orchestratorapp:
	go build -race -ldflags "-extldflags '-static'" -o bin/orchestrator cmd/orchestrator/main.go

testapp:
	go build -race -ldflags "-extldflags '-static'" -o bin/testapp testapp/main.go

# Build the docker image
docker-build: test
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

