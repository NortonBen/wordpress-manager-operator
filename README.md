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

### 5. Custom cao qua YAML
- `spec.env`, `spec.phpConfig` (→ `WORDPRESS_CONFIG_EXTRA`), `spec.resources`, `spec.replicas`,
  `spec.image`, `spec.ingressAnnotations` cho phép tùy biến sâu.
- UI có nút **Preview YAML** (`POST /api/v1/sites/preview`) hiển thị đúng manifest operator sẽ sinh ra
  trước khi tạo.

### 6. Auth cho admin
API bảo vệ bằng **JWT** (HS256). Đăng nhập tại `POST /api/v1/login`; danh tính admin lấy từ env
(`ADMIN_USERNAME`, `ADMIN_PASSWORD` hoặc `ADMIN_PASSWORD_HASH` bcrypt). Mọi route `/api/v1/*` còn lại
yêu cầu `Authorization: Bearer <token>`.

## Cài đặt

> ⚠️ Đổi mọi giá trị `change-me-*` trong `deploy/03-mysql.yaml` và `deploy/06-apiserver.yaml` trước khi
> dùng thật.

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

## Bảo mật — lưu ý production
- Thay admin MySQL `root` bằng user quản trị riêng; cân nhắc TLS tới MySQL.
- Cung cấp `ADMIN_PASSWORD_HASH` (bcrypt) thay vì mật khẩu thô; đặt `JWT_SECRET` đủ dài & ngẫu nhiên.
- Siết `CORS_ORIGINS` về đúng origin của UI.
- Cân nhắc `NetworkPolicy` để chỉ operator/site được nói chuyện với MySQL.
```
