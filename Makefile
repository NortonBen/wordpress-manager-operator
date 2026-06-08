REGISTRY ?= wordpress-manager
TAG ?= latest

.PHONY: build vet test tidy \
        docker-operator docker-apiserver docker-ui docker \
        deploy undeploy run-operator run-api ui-dev

## --- Go ---
build: ## Compile all Go binaries
	go build ./...

vet:
	go vet ./...

test:
	go test ./...

tidy:
	go mod tidy

## --- Docker images ---
docker-operator:
	docker build -f Dockerfile.operator -t $(REGISTRY)/operator:$(TAG) .

docker-apiserver:
	docker build -f Dockerfile.apiserver -t $(REGISTRY)/apiserver:$(TAG) .

docker-ui:
	docker build -f ui/Dockerfile -t $(REGISTRY)/ui:$(TAG) ui

docker: docker-operator docker-apiserver docker-ui

## --- Kubernetes ---
deploy: ## Apply all manifests in order
	kubectl apply -f deploy/

undeploy:
	kubectl delete -f deploy/ --ignore-not-found

## --- Local dev (uses your current kubeconfig) ---
run-operator:
	MYSQL_HOST=127.0.0.1 MYSQL_ADMIN_PASSWORD=change-me-root-password \
	LEADER_ELECT=false LOG_DEV=true go run ./cmd/operator

run-api:
	JWT_SECRET=dev-secret ADMIN_PASSWORD=admin SITES_NAMESPACE=wordpress-sites \
	go run ./cmd/apiserver

ui-dev:
	cd ui && npm install && npm run dev
