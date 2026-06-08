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

install.yaml: ## Regenerate the single-file installer from deploy/
	./hack/gen-install.sh install.yaml

install: install.yaml ## One-shot install of the whole control plane
	kubectl apply -f install.yaml

uninstall:
	kubectl delete -f install.yaml --ignore-not-found

## --- Local dev MOCK (no Kubernetes, no MySQL) ---
# In-memory fake cluster + SQLite. The API + UI are fully functional offline.
dev-api: ## Run the API in dev-mock mode on :8090 (UI proxies here)
	DEV_MODE=true JWT_SECRET=dev-secret ADMIN_USERNAME=admin ADMIN_PASSWORD=admin \
	SITES_NAMESPACE=wordpress-sites SQLITE_DIR=.dev/sqlite API_ADDR=:8090 \
	go run ./cmd/apiserver

dev-ui: ## Run the React UI dev server (proxies /api -> :8090)
	cd ui && npm install && npm run dev

dev: ## Print how to run the two dev-mock processes
	@echo "Run in two terminals:"
	@echo "  make dev-api   # mock API on :8090 (SQLite + in-memory k8s)"
	@echo "  make dev-ui    # UI on :5173, proxies /api -> :8090"
	@echo "Login with admin / admin."

e2e: ## Full-flow test on a throwaway kind cluster
	./hack/e2e.sh

## --- Local dev against a REAL cluster (uses your current kubeconfig) ---
run-operator:
	MYSQL_HOST=127.0.0.1 MYSQL_ADMIN_PASSWORD=change-me-root-password \
	LEADER_ELECT=false LOG_DEV=true go run ./cmd/operator

run-api:
	JWT_SECRET=dev-secret ADMIN_PASSWORD=admin SITES_NAMESPACE=wordpress-sites \
	go run ./cmd/apiserver
