# Hướng dẫn chi tiết: GitHub Actions → GHCR (public) → cài bằng 1 lệnh

Tài liệu này hướng dẫn từng bước để:
1. Build & push 3 image lên **GitHub Container Registry (ghcr.io)** bằng GitHub Actions.
2. Đặt package thành **Public** để cụm Kubernetes kéo image **không cần `imagePullSecret`**.
3. Cài toàn bộ hệ thống bằng **một lệnh `kubectl apply` duy nhất**.

---

## 0. Có sẵn gì trong repo

| Workflow | Trigger | Kết quả |
|---|---|---|
| [`ci.yml`](../.github/workflows/ci.yml) | push/PR `main` | test Go (`-race`) + test & build UI |
| [`docker-publish.yml`](../.github/workflows/docker-publish.yml) | **chỉ khi tag `v*`** (+ dispatch) | build 3 image multi-arch, **push GHCR** |
| [`release.yml`](../.github/workflows/release.yml) | tag `v*` | sinh `install.yaml` (trỏ image GHCR) + tạo GitHub Release |

> **Image chỉ build khi tạo tag version.** Push/PR vào `main` **không** build image — chỉ chạy CI
> (test). Muốn ra image: tạo tag `vX.Y.Z` (hoặc chạy tay `docker-publish` qua *Run workflow* với
> input `tag`).

3 image được publish:

```
ghcr.io/<owner>/wordpress-manager-operator
ghcr.io/<owner>/wordpress-manager-apiserver
ghcr.io/<owner>/wordpress-manager-ui
```

`<owner>` = user hoặc tổ chức GitHub của bạn (viết thường).

---

## 1. Đẩy code lên GitHub

```bash
git init
git add .
git commit -m "WordPress manager operator"
git branch -M main
git remote add origin https://github.com/<owner>/<repo>.git
git push -u origin main
```

Ngay khi push, tab **Actions** chạy `CI` (test Go + UI). **Image chưa build** ở bước này —
image chỉ được build & push khi bạn tạo **tag version** (xem [B5](#5-cài-bằng-1-lệnh-kubectl-apply)).

---

## 2. Cấp quyền cho Actions ghi package

Workflow đã khai báo sẵn `permissions: packages: write`, dùng `GITHUB_TOKEN` tự động — **không cần
tạo secret**. Chỉ cần đảm bảo tổ chức/repo không chặn:

1. Repo → **Settings → Actions → General**.
2. Mục **Workflow permissions** → chọn **Read and write permissions** → **Save**.

> Nếu repo thuộc **Organization**: vào **Org → Settings → Packages** đảm bảo cho phép thành viên tạo
> package; và **Org → Settings → Actions** cho phép workflow chạy.

---

## 3. Kiểm tra image đã lên GHCR

Sau khi job `Build & Publish images` xanh:

- **User**: `https://github.com/<owner>?tab=packages`
- **Org**: `https://github.com/orgs/<org>/packages`

Bạn sẽ thấy 3 package `wordpress-manager-operator`, `-apiserver`, `-ui`. Mặc định chúng **Private**.

---

## 4. ⭐ Đặt package thành PUBLIC (để kéo không cần secret)

GHCR tạo package **private** theo mặc định. Để `kubectl apply` kéo được mà không cần
`imagePullSecret`, đổi sang Public cho **cả 3 package**:

### Cách A — qua giao diện (làm 1 lần)
Với **mỗi** package:
1. Mở package → **Package settings** (menu bên phải).
2. Kéo xuống **Danger Zone** → **Change visibility** → chọn **Public** → gõ tên xác nhận.

### Cách B — qua API (nhanh, cần PAT có scope `write:packages`)
```bash
# Tạo classic PAT: GitHub → Settings → Developer settings → Tokens (classic)
# scope: write:packages, read:packages  (và delete:packages nếu cần)
export GH_TOKEN=ghp_xxx
export OWNER=<owner>     # user hoặc org (viết thường)

for pkg in wordpress-manager-operator wordpress-manager-apiserver wordpress-manager-ui; do
  curl -X PATCH \
    -H "Authorization: Bearer $GH_TOKEN" \
    -H "Accept: application/vnd.github+json" \
    "https://api.github.com/user/packages/container/$pkg" \
    -d '{"visibility":"public"}'
done
# Với Organization, đổi endpoint thành:
#   https://api.github.com/orgs/$OWNER/packages/container/$pkg
```

Sau khi Public, URL `ghcr.io/<owner>/wordpress-manager-operator:latest` kéo được ẩn danh.

> **Nếu muốn GIỮ PRIVATE**: bỏ qua bước này và làm theo [mục 8 — Image private](#8-tùy-chọn-image-private--tạo-imagepullsecret).

---

## 5. Cài bằng 1 lệnh `kubectl apply`

### 5a. Từ Release (khuyến nghị — image đã trỏ đúng GHCR)
Tạo một bản phát hành:

```bash
git tag v1.0.0
git push origin v1.0.0
```

`release.yml` sẽ sinh `install.yaml` (image = `ghcr.io/<owner>/...:1.0.0`) và đính vào Release. Cài:

```bash
kubectl apply -f https://github.com/<owner>/<repo>/releases/download/v1.0.0/install.yaml
# hoặc luôn lấy bản mới nhất:
kubectl apply -f https://github.com/<owner>/<repo>/releases/latest/download/install.yaml
```

### 5b. Từ file trong repo (image GHCR `latest`)
File [`install.yaml`](../install.yaml) commit sẵn dùng tên image **local**. Sinh lại bản trỏ GHCR:

```bash
IMAGE_REGISTRY=ghcr.io/<owner> IMAGE_TAG=latest ./hack/gen-install.sh install.yaml
kubectl apply -f install.yaml
```

Cả hai cách đều cài: 2 Namespace → CRD → RBAC → MySQL → PVC dùng chung → Operator → API → UI
(đúng thứ tự phụ thuộc, gói trong 1 file).

> Tạo host WordPress **sau khi** CRD đã sẵn sàng:
> `kubectl apply -f config/samples/wordpresssite-sample.yaml`

---

## 6. Trước khi dùng thật — sửa 3 thứ

`install.yaml` mang giá trị mặc định/placeholder. Sửa **trước** khi apply ở môi trường thật:

1. **Secrets** `change-me-*`: mật khẩu MySQL root, `ADMIN_PASSWORD`, `JWT_SECRET`.
2. **storageClassName** (PVC `wordpress-shared`): phải là class **ReadWriteMany**
   (NFS/CephFS/Longhorn-RWX/EFS/Azure Files/Filestore).
3. **Ingress host** của UI (`admin.wordpress.local`) → tên miền quản trị của bạn.

Yêu cầu cụm: có **Ingress controller** (mặc định `nginx`); nếu bật TLS thì có **cert-manager**.

---

## 7. Cập nhật về sau

- Cách chuẩn: tạo **tag version mới** (`v1.0.1`) → image mới được build + Release mới có `install.yaml`.
  Apply lại `install.yaml` của release đó (tag cố định, dễ rollback):
  ```bash
  kubectl apply -f https://github.com/<owner>/<repo>/releases/download/v1.0.1/install.yaml
  ```
- Nếu chỉ đổi env/secret (không đổi image): sửa rồi rollout lại:
  ```bash
  kubectl -n wordpress-system rollout restart deploy/wordpress-operator deploy/wordpress-apiserver deploy/wordpress-ui
  ```

---

## 8. (Tùy chọn) Image private → tạo `imagePullSecret`

Nếu giữ package **Private**, cụm cần thông tin đăng nhập GHCR:

```bash
kubectl -n wordpress-system create secret docker-registry ghcr-creds \
  --docker-server=ghcr.io \
  --docker-username=<owner> \
  --docker-password=<PAT có scope read:packages>

# Gắn vào các ServiceAccount để pod tự dùng:
kubectl -n wordpress-system patch serviceaccount wordpress-operator \
  -p '{"imagePullSecrets":[{"name":"ghcr-creds"}]}'
kubectl -n wordpress-system patch serviceaccount wordpress-apiserver \
  -p '{"imagePullSecrets":[{"name":"ghcr-creds"}]}'
kubectl -n wordpress-system patch serviceaccount default \
  -p '{"imagePullSecrets":[{"name":"ghcr-creds"}]}'   # UI dùng SA default
```

---

## 9. Xử lý sự cố

| Triệu chứng | Nguyên nhân | Cách xử lý |
|---|---|---|
| Pod `ImagePullBackOff`, `denied` | Package vẫn Private | Làm [mục 4](#4--đặt-package-thành-public-để-kéo-không-cần-secret) hoặc [mục 8](#8-tùy-chọn-image-private--tạo-imagepullsecret) |
| Job push lỗi `403 / installation not allowed` | Thiếu quyền write | [Mục 2](#2-cấp-quyền-cho-actions-ghi-package): bật *Read and write* |
| `manifest unknown` khi apply release | Image của tag chưa push xong | Đợi job `Build & Publish images` của tag đó xanh rồi apply lại |
| Pod chạy nhưng pull sai kiến trúc | Cụm ARM/AMD khác | Image đã multi-arch `amd64+arm64`; nếu cần thêm, sửa `platforms:` trong `docker-publish.yml` |
| `kubectl apply` báo namespace not found | Hiếm gặp do tạo namespace bất đồng bộ | Chạy lại `kubectl apply -f install.yaml` lần nữa (idempotent) |
