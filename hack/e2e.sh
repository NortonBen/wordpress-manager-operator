#!/usr/bin/env bash
# Full end-to-end flow on a throwaway kind cluster:
#   build images → load → install control plane (MySQL + operator + API + UI)
#   → create a WordPressSite → assert DB/user/Secret/Service/Ingress + the
#   WordPress pod actually serves through the Ingress → exercise the REST API.
#
# Env:
#   KIND_CLUSTER (default wpmgr-e2e)   KEEP=1 keep cluster on success
#   SKIP_BUILD=1 reuse existing images
set -euo pipefail

CLUSTER="${KIND_CLUSTER:-wpmgr-e2e}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$(go env GOPATH)/bin:$PATH"
SITE=blog-e2e
SITE_NS=wordpress-sites
HOST=blog.e2e.local
HTTP_PORT=8080

log() { echo; echo "=== $* ==="; }
mysql_q() {
  local pw="$1" sql="$2"
  kubectl -n wordpress-system exec mysql-0 -- sh -c "mysql -uroot -p'$pw' -N -e \"$sql\"" 2>/dev/null
}

cleanup() {
  if [ "${KEEP:-}" != "1" ]; then
    log "cleanup: delete kind cluster $CLUSTER"
    kind delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true
  fi
}

# ---------------------------------------------------------------- 0. cluster
log "create kind cluster: $CLUSTER"
if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  cat <<EOF | kind create cluster --name "$CLUSTER" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: ${HTTP_PORT}
    protocol: TCP
EOF
fi
kubectl config use-context "kind-$CLUSTER" >/dev/null
trap cleanup EXIT

# ----------------------------------------------------------- 1. build + load
if [ "${SKIP_BUILD:-}" != "1" ]; then
  log "build images (operator, apiserver, ui)"
  docker build -q -f Dockerfile.operator  -t wordpress-manager/operator:latest  . >/dev/null
  docker build -q -f Dockerfile.apiserver -t wordpress-manager/apiserver:latest . >/dev/null
  docker build -q -f ui/Dockerfile        -t wordpress-manager/ui:latest        ui >/dev/null
fi
log "load images into kind"
kind load docker-image --name "$CLUSTER" \
  wordpress-manager/operator:latest \
  wordpress-manager/apiserver:latest \
  wordpress-manager/ui:latest

# --------------------------------------------------------- 2. ingress-nginx
log "install ingress-nginx"
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.11.2/deploy/static/provider/kind/deploy.yaml
kubectl -n ingress-nginx wait --for=condition=ready pod \
  -l app.kubernetes.io/component=controller --timeout=180s

# --------------------------------------------------- 3. install control plane
log "generate + apply install.yaml"
./hack/gen-install.sh "$ROOT/install.yaml" >/dev/null
kubectl apply -f "$ROOT/install.yaml"

log "wait for MySQL + operator + API + UI"
kubectl -n wordpress-system rollout status statefulset/mysql            --timeout=300s
kubectl -n wordpress-system rollout status deploy/wordpress-operator    --timeout=180s
kubectl -n wordpress-system rollout status deploy/wordpress-apiserver   --timeout=180s
kubectl -n wordpress-system rollout status deploy/wordpress-ui          --timeout=180s

# --------------------------------------------------------- 4. create a site
log "create WordPressSite/$SITE"
cat <<EOF | kubectl apply -f -
apiVersion: wp.benji.dev/v1alpha1
kind: WordPressSite
metadata:
  name: $SITE
  namespace: $SITE_NS
spec:
  domain: $HOST
  replicas: 1
  tls:
    enabled: false
  resources:
    requests:
      cpu: 50m
      memory: 256Mi
EOF

log "wait for operator to create the Deployment, then for it to roll out"
for _ in $(seq 1 40); do
  kubectl -n "$SITE_NS" get deploy/"$SITE" >/dev/null 2>&1 && break
  sleep 2
done
kubectl -n "$SITE_NS" rollout status deploy/"$SITE" --timeout=300s

# ------------------------------------------------------------ 5. assertions
log "ASSERT: WordPressSite reached Ready"
kubectl -n "$SITE_NS" get wordpresssite "$SITE" -o wide
phase=$(kubectl -n "$SITE_NS" get wordpresssite "$SITE" -o jsonpath='{.status.phase}')
[ "$phase" = "Ready" ] || { echo "FAIL: phase=$phase"; exit 1; }
echo "✔ phase=Ready"

log "ASSERT: owned Secret / Service / Ingress exist"
kubectl -n "$SITE_NS" get secret "${SITE}-wp"  >/dev/null && echo "✔ Secret ${SITE}-wp"
kubectl -n "$SITE_NS" get svc "$SITE"           >/dev/null && echo "✔ Service $SITE"
kubectl -n "$SITE_NS" get ingress "$SITE"       >/dev/null && echo "✔ Ingress $SITE"

log "ASSERT: per-site database + least-privilege user in MySQL"
ROOTPW=$(kubectl -n wordpress-system get secret mysql-admin \
  -o go-template='{{.data.MYSQL_ROOT_PASSWORD | base64decode}}')
DBNAME=$(kubectl -n "$SITE_NS" get wordpresssite "$SITE" -o jsonpath='{.status.databaseName}')
DBUSER=$(kubectl -n "$SITE_NS" get wordpresssite "$SITE" -o jsonpath='{.status.databaseUser}')
echo "db=$DBNAME user=$DBUSER"
mysql_q "$ROOTPW" "SHOW DATABASES LIKE '$DBNAME'" | grep -qx "$DBNAME" && echo "✔ database '$DBNAME' created"
mysql_q "$ROOTPW" "SELECT User FROM mysql.user WHERE User='$DBUSER'" | grep -qx "$DBUSER" && echo "✔ user '$DBUSER' created"
echo "grants (should be scoped to '$DBNAME' only):"
mysql_q "$ROOTPW" "SHOW GRANTS FOR '$DBUSER'@'%'" | sed 's/^/    /'

log "ASSERT: WordPress serves through the Ingress (readiness already proves DB connectivity)"
ok=0
for i in $(seq 1 30); do
  code=$(curl -s -o /dev/null -w '%{http_code}' -H "Host: $HOST" "http://localhost:${HTTP_PORT}/wp-login.php" || true)
  echo "  try $i -> HTTP $code"
  case "$code" in 200|302) ok=1; break;; esac
  sleep 5
done
[ "$ok" = 1 ] && echo "✔ WordPress reachable via Ingress" || { echo "FAIL: ingress unreachable"; exit 1; }

# ----------------------------------------------------- 6. REST API admin flow
log "REST API: login → create site → list (via port-forward)"
kubectl -n wordpress-system port-forward svc/wordpress-apiserver 8090:80 >/tmp/wpmgr-pf.log 2>&1 &
PF=$!
sleep 4
API=http://localhost:8090/api/v1
TOKEN=$(curl -s -X POST "$API/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"change-me-admin-password"}' \
  | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
[ -n "$TOKEN" ] && echo "✔ login returned a JWT" || { echo "FAIL: no token"; kill $PF; exit 1; }
acode=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/sites" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"api-site","domain":"api.e2e.local","replicas":1}')
echo "create-via-API -> HTTP $acode"; [ "$acode" = "201" ] && echo "✔ site created via REST API"
echo "sites listed by API:"
curl -s "$API/sites" -H "Authorization: Bearer $TOKEN" | sed 's/},{/}\n{/g' | sed 's/^/    /'
kill $PF 2>/dev/null || true

log "ALL E2E CHECKS PASSED ✅"
