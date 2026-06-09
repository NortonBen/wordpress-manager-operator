# WordPress Manager Operator

Operator Kubernetes để **quản trị hosting WordPress nhiều site (multi-tenant)** trên một cụm dùng chung.
Mỗi "host" WordPress được khai báo bằng một Custom Resource `WordPressSite`; operator tự động dựng
**Deployment + Service + Ingress + database + user riêng**, dùng chung một volume nhưng tách thư mục con.

```
┌──────────────┐     REST/JWT      ┌─────────────────┐   watch/reconcile   ┌──────────────────────┐
│  React UI    │ ───────────────▶ │  API server (Go) │ ─── create CR ───▶ │  Operator (Go)        │
│ antd + TS    │ ◀─────────────── │  go-chi + auth   │                     │  controller-runtime   │
└──────────────┘                  └─────────────────┘                     └───────────┬──────────┘
                                                                                       │ reconcile
                                          ┌────────────────────────────────────────────┼───────────────┐
                                          ▼                ▼               ▼            ▼               ▼
                                     Deployment        Service        Ingress       Secret      MySQL DB+User
                                   (subPath/site)                  (domain→svc)  (db pass+salts)  (per-site)
                                          │
                                          ▼
                                 ┌──────────────────┐
                                 │  Shared RWX PVC   │  /var/www/html  ← subPath = <tên site>
                                 │  wordpress-shared │  ├── blog-acme/
                                 └──────────────────┘  └── shop-foo/   (mỗi site 1 folder con)
```

## Thành phần

| Thư mục | Vai trò |
|---|---|
| `api/v1alpha1/` | Định nghĩa CRD `WordPressSite` (Go types + deepcopy) |
| `internal/controller/` | Vòng reconcile: dựng tài nguyên + provision DB |
| `internal/resources/` | Builder cho Deployment / Service / Ingress / Secret + đặt tên |
| `internal/mysql/` | Tạo database + user least-privilege cho mỗi site |
| `internal/apiserver/` | REST API + auth JWT cho UI |
| `internal/metrics/` | Theo dõi CPU/RAM (prod: metrics.k8s.io; dev: tổng hợp) |
| `internal/sqlite/` | Provisioner SQLite cho dev mock (thay MySQL) |
| `cmd/operator/` | Entrypoint controller manager |
| `cmd/apiserver/` | Entrypoint REST API |
| `ui/` | React + TypeScript + Ant Design |
| `deploy/` | Toàn bộ manifest YAML (cài theo thứ tự số) |
| `config/samples/` | Ví dụ `WordPressSite` |

## Thiết kế chính

### 1. Mô hình tài nguyên — `WordPressSite`
Một CR khai báo toàn bộ một host. Operator coi nó là *nguồn sự thật* và tự đồng bộ mọi thứ phía dưới.
Xem các trường trong [`api/v1alpha1/wordpresssite_types.go`](api/v1alpha1/wordpresssite_types.go) và CRD
[`deploy/01-crd.yaml`](deploy/01-crd.yaml).

### 2. Storage dùng chung, tách folder con
Tất cả site **mount cùng một PVC `ReadWriteMany`** (`wordpress-shared`) nhưng mỗi pod mount tại
`subPath = <tên site>`. Nhờ vậy chia sẻ một volume lớn mà file mỗi host vẫn cô lập trong thư mục riêng.
> RWX cần storage backend chia sẻ: NFS / CephFS / Longhorn-RWX / EFS / Azure Files / Filestore.
> Đặt `storageClassName` trong [`deploy/04-shared-storage.yaml`](deploy/04-shared-storage.yaml).

### 3. Database tách biệt & bảo mật theo từng host
Khi tạo site, operator dùng quyền admin MySQL để:
- Tạo database `wp_<site>` (utf8mb4).
- Tạo **user riêng** `wpu_<site>` với mật khẩu ngẫu nhiên.
- `GRANT ALL` **chỉ trên đúng database đó** → một site bị lộ không truy cập được DB của site khác.

Mật khẩu + WordPress salts được lưu trong Secret `*-wp` và **inject vào container qua env**
(`WORDPRESS_DB_*`, các `WORDPRESS_*_KEY/SALT`). Reconcile là idempotent: mật khẩu được giữ nguyên qua
các vòng lặp (không xoay vòng credential đang chạy).

### 4. Domain → Service + Ingress tự động
`spec.domain` (+ `aliases`) sinh ra Ingress trỏ về Service của site, kèm TLS qua cert-manager khi
`spec.tls.enabled`. Có thể tùy biến bằng `spec.ingressClassName` và `spec.ingressAnnotations`.

### 5. Custom cao qua YAML — sửa tay
- `spec.env`, `spec.phpConfig` (→ `WORDPRESS_CONFIG_EXTRA`, code wp-config.php), `spec.resources`,
  `spec.replicas`, `spec.image` (mặc định **`wordpress:latest`**), `spec.ingressAnnotations`.
- **`spec.phpIni`** — nội dung **php.ini** (memory_limit, upload_max_filesize, max_execution_time…).
  Operator tạo ConfigMap và mount vào `/usr/local/etc/php/conf.d/zz-wpmgr.ini`; khi **edit php.ini**,
  pod **tự rollout** (annotation hash đổi) nên PHP nạp lại cấu hình ngay. Cài qua form lúc tạo host hoặc
  sửa tay trong tab YAML của trang chi tiết. **Để trống → dùng php.ini mặc định**:
  ```ini
  file_uploads = On
  memory_limit = 256M
  upload_max_filesize = 500M
  post_max_size = 500M
  max_execution_time = 300
  extension=mysqli
  ```
- **Trang chi tiết host** (`/sites/<name>`) hiển thị thông tin + **YAML đã deploy**, cho phép **sửa tay**
  WordPressSite rồi Lưu — operator reconcile lại. Tab "Manifests đã deploy" xem read-only
  Deployment/Service/Ingress.
- Editor có **kiểm tra YAML phía client** (parse js-yaml) — báo lỗi cú pháp và **chặn Lưu** khi YAML sai.
- **Tạm dừng / Kích hoạt** host ngay trên trang chi tiết (`spec.suspend` → operator scale Deployment về 0,
  phase `Suspended`).
- API: `GET /api/v1/sites/{name}/yaml` (source CR + rendered manifests), `PUT …/yaml` (sửa tay toàn bộ
  spec), `PUT /api/v1/sites/{name}` (sửa nhanh các trường thường dùng, giữ nguyên env/resources…),
  `POST /api/v1/sites/{name}/suspend` · `…/resume`, `POST /api/v1/sites/preview` (xem trước khi tạo).

### 6. Theo dõi tài nguyên (CPU/RAM) trên UI
`GET /api/v1/metrics` trả về CPU & RAM cấp cụm — **đã dùng / capacity / allocatable / còn trống** —
kèm usage từng host. UI hiển thị **thẻ tài nguyên** (CPU, RAM với thanh % + "còn trống") ở đầu trang
quản trị và **cột CPU/RAM theo từng host** trong bảng (poll mỗi 8s).
- Production: đọc `metrics.k8s.io` (cần **metrics-server**) + capacity từ Node. Thiếu metrics-server thì
  vẫn báo capacity, usage = 0 và cờ `metricsAvailable=false` (UI hiện badge cảnh báo).
- Dev mock: `internal/metrics` tổng hợp số liệu từ các site (used tăng/available giảm khi tạo thêm host).

### 7. Auth cho admin
API bảo vệ bằng **JWT** (HS256). Đăng nhập tại `POST /api/v1/login`; danh tính admin lấy từ env
(`ADMIN_USERNAME`, `ADMIN_PASSWORD` hoặc `ADMIN_PASSWORD_HASH` bcrypt). Mọi route `/api/v1/*` còn lại
yêu cầu `Authorization: Bearer <token>`.

## Cài đặt

> ⚠️ Đổi mọi giá trị `change-me-*` trong `deploy/03-mysql.yaml` và `deploy/06-apiserver.yaml` trước khi
> dùng thật.

### Cách nhanh nhất — **một lệnh `kubectl apply`**

Sau khi image đã có trên registry (xem [hướng dẫn GitHub Actions](docs/GITHUB_ACTIONS.md)):

```bash
# Từ GitHub Release (install.yaml đã trỏ sẵn image GHCR của tag đó):
kubectl apply -f https://github.com/<owner>/<repo>/releases/latest/download/install.yaml

# Hoặc từ file trong repo:
make install                     # = ./hack/gen-install.sh && kubectl apply -f install.yaml
```

`install.yaml` gói toàn bộ control plane theo đúng thứ tự phụ thuộc (Namespaces → CRD → RBAC → MySQL →
PVC dùng chung → Operator → API → UI). Sinh lại bất cứ lúc nào: `make install.yaml`.
Trỏ sang image GHCR: `IMAGE_REGISTRY=ghcr.io/<owner> IMAGE_TAG=latest ./hack/gen-install.sh`.

### Local dev MOCK — không cần Kubernetes, không cần MySQL

Chạy toàn bộ API + UI quản trị **offline** trên máy: dùng **in-memory fake cluster** (mock K8s API) và
**SQLite** thay cho MySQL. Bật bằng `DEV_MODE=true`.

```bash
make dev-api    # API mock trên :8090  (SQLite tại .dev/sqlite, k8s in-memory)
make dev-ui     # UI trên :5173, proxy /api -> :8090
# đăng nhập: admin / admin
```

Khi tạo host ở chế độ này, API server tự chạy reconcile **in-process**: tạo file SQLite cho từng site
(`wp_<site>.db`), dựng Deployment/Service/Ingress/Secret trong cluster ảo, set `status.phase=Ready`. Nhờ
vậy UI hoạt động đầy đủ (tạo/sửa/xóa/preview YAML) mà không cần cụm thật.

| | Production | Dev mock (`DEV_MODE=true`) |
|---|---|---|
| Database | MySQL (`internal/mysql`) | SQLite (`internal/sqlite`) |
| Kubernetes | cụm thật (operator riêng) | fake client in-memory, reconcile in-process |
| Owned objects | Server-Side Apply | client-side create/update |

### Cách thủ công (build cục bộ + apply theo thư mục)

```bash
# 1. Build image
make docker                      # operator + apiserver + ui

# (kind) nạp image vào cụm, ví dụ:
#   kind load docker-image wordpress-manager/operator:latest
#   kind load docker-image wordpress-manager/apiserver:latest
#   kind load docker-image wordpress-manager/ui:latest

# 2. Sửa storageClassName trong deploy/04-shared-storage.yaml (cần RWX) và
#    ingress host trong deploy/07-ui.yaml.

# 3. Triển khai theo thứ tự
make deploy                      # = kubectl apply -f deploy/

# 4. Tạo một host
kubectl apply -f config/samples/wordpresssite-sample.yaml
kubectl get wordpresssites -n wordpress-sites
```

Yêu cầu cụm: một **Ingress controller** (mặc định `nginx`) và, nếu bật TLS, **cert-manager**.

## Phát triển cục bộ (dùng kubeconfig hiện tại)

```bash
make run-operator     # chạy controller ngoài cụm
make run-api          # chạy REST API (cần JWT_SECRET, ADMIN_PASSWORD)
make ui-dev           # Vite dev server, proxy /api → :8090
```

## Vòng đời reconcile (tóm tắt)

1. Đảm bảo finalizer → đảm bảo Secret (sinh mật khẩu/salts nếu chưa có).
2. `EnsureDatabase` trên MySQL (idempotent): database + user + grant theo phạm vi.
3. Apply Deployment + Service + Ingress (set ownerRef để GC tự dọn khi xóa site).
4. Cập nhật `.status` (phase, url, databaseName/User).
5. Khi xóa: tùy `DROP_DATA_ON_DELETE` mà drop DB; tài nguyên con bị GC qua ownerRef.

## CI/CD (GitHub Actions)

> 📘 Hướng dẫn từng bước (đẩy code, đặt package **public**, cài 1 lệnh, xử lý sự cố):
> [docs/GITHUB_ACTIONS.md](docs/GITHUB_ACTIONS.md).

| Workflow | Khi nào chạy | Việc làm |
|---|---|---|
| [`.github/workflows/ci.yml`](.github/workflows/ci.yml) | push/PR vào `main` | `go build/vet/test -race` + UI `npm test` + `npm run build` |
| [`.github/workflows/docker-publish.yml`](.github/workflows/docker-publish.yml) | **chỉ khi tag `v*`** (+ dispatch) | build 3 image (matrix) multi-arch, **push lên GHCR** |
| [`.github/workflows/release.yml`](.github/workflows/release.yml) | tag `v*` | sinh `install.yaml` (trỏ image GHCR) + tạo **GitHub Release** đính kèm |
| [`.github/workflows/e2e.yml`](.github/workflows/e2e.yml) | thủ công / lịch tuần | **full luồng** trên kind: cài → tạo host → assert DB/Ingress/API |

### Kiểm thử

```bash
go test ./...          # unit: resources builders + REST API (mock cluster fake client)
cd ui && npm test      # unit: UI quản trị (Vitest + Testing Library, mock api/client)
./hack/e2e.sh          # full luồng end-to-end trên kind (xem bên dưới)
```

`hack/e2e.sh` dựng một kind cluster tạm rồi chạy **đúng luồng thật**: build 3 image → load →
cài `install.yaml` (MySQL + operator + API + UI) → cài ingress-nginx → tạo một `WordPressSite` →
**kiểm chứng**: CR `Ready`, Secret/Service/Ingress được sinh, database `wp_*` + user `wpu_*` tạo trong
MySQL (grant đúng phạm vi), pod WordPress **phục vụ qua Ingress**, và REST API login/create/list hoạt động.
Giữ cluster để soi: `KEEP=1 ./hack/e2e.sh`.

Image xuất bản (multi-arch `amd64`+`arm64`) lên GitHub Container Registry:

```
ghcr.io/<owner>/wordpress-manager-operator
ghcr.io/<owner>/wordpress-manager-apiserver
ghcr.io/<owner>/wordpress-manager-ui
```

Tag tự sinh: tên branch, `pr-<n>`, `sha-xxxxxxx`, SemVer (khi push tag `v1.2.3`), và `latest` ở branch mặc định.
Không cần secret thêm — workflow dùng `GITHUB_TOKEN` sẵn có (cần bật *Read and write* cho Actions, hoặc
để mặc định với `permissions: packages: write`).

Sau khi push, trỏ manifest về image GHCR (thay `<owner>`):

```bash
sed -i '' "s#wordpress-manager/operator:latest#ghcr.io/<owner>/wordpress-manager-operator:latest#"   deploy/05-operator.yaml
sed -i '' "s#wordpress-manager/apiserver:latest#ghcr.io/<owner>/wordpress-manager-apiserver:latest#" deploy/06-apiserver.yaml
sed -i '' "s#wordpress-manager/ui:latest#ghcr.io/<owner>/wordpress-manager-ui:latest#"               deploy/07-ui.yaml
```
> Nếu package GHCR để **private**, tạo `imagePullSecret` và gắn vào các ServiceAccount tương ứng.

## Bảo mật — lưu ý production
- Thay admin MySQL `root` bằng user quản trị riêng; cân nhắc TLS tới MySQL.
- Cung cấp `ADMIN_PASSWORD_HASH` (bcrypt) thay vì mật khẩu thô; đặt `JWT_SECRET` đủ dài & ngẫu nhiên.
- Siết `CORS_ORIGINS` về đúng origin của UI.
- Cân nhắc `NetworkPolicy` để chỉ operator/site được nói chuyện với MySQL.
```

## License

[MIT](LICENSE) © 2026 NortonBen
