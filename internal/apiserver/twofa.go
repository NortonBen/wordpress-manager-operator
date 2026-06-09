package apiserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"maps"
	"net/http"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TwoFA manages admin TOTP two-factor auth, persisted in a Kubernetes Secret.
type TwoFA interface {
	Enabled(ctx context.Context) bool
	Setup(ctx context.Context) (secret, otpauthURL, qrDataURI string, err error)
	Enable(ctx context.Context, code string) error
	Disable(ctx context.Context, code string) error
	Validate(ctx context.Context, code string) bool
}

const (
	twoFAKeySecret  = "secret"  // active base32 TOTP secret
	twoFAKeyEnabled = "enabled" // "true" when active
	twoFAKeyPending = "pending" // base32 secret awaiting verification
)

var errCodeInvalid = errors.New("invalid 2FA code")

// KubeTwoFA stores the TOTP secret in a Secret so it survives restarts and the
// stateless admin model. Active 2FA is untouched until a new secret is verified.
type KubeTwoFA struct {
	K8s        client.Client
	Namespace  string
	SecretName string
	Issuer     string
	Account    string
}

func (t *KubeTwoFA) load(ctx context.Context) (*corev1.Secret, error) {
	s := &corev1.Secret{}
	err := t.K8s.Get(ctx, client.ObjectKey{Namespace: t.Namespace, Name: t.SecretName}, s)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	return s, err
}

func (t *KubeTwoFA) save(ctx context.Context, existing *corev1.Secret, data map[string][]byte) error {
	if existing == nil {
		return t.K8s.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: t.SecretName, Namespace: t.Namespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       data,
		})
	}
	existing.Data = data
	return t.K8s.Update(ctx, existing)
}

func (t *KubeTwoFA) Enabled(ctx context.Context) bool {
	s, err := t.load(ctx)
	if err != nil || s == nil {
		return false
	}
	return string(s.Data[twoFAKeyEnabled]) == "true" && len(s.Data[twoFAKeySecret]) > 0
}

// Setup generates a new pending secret (does not affect active 2FA) and returns
// the secret, the otpauth:// URL and a PNG QR data URI for scanning.
func (t *KubeTwoFA) Setup(ctx context.Context) (string, string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: t.Issuer, AccountName: t.Account})
	if err != nil {
		return "", "", "", err
	}
	s, err := t.load(ctx)
	if err != nil {
		return "", "", "", err
	}
	data := map[string][]byte{}
	if s != nil {
		maps.Copy(data, s.Data)
	}
	data[twoFAKeyPending] = []byte(key.Secret())
	if err := t.save(ctx, s, data); err != nil {
		return "", "", "", err
	}
	qr, err := qrDataURI(key)
	if err != nil {
		return "", "", "", err
	}
	return key.Secret(), key.URL(), qr, nil
}

// Enable verifies a code against the pending secret and promotes it to active.
func (t *KubeTwoFA) Enable(ctx context.Context, code string) error {
	s, err := t.load(ctx)
	if err != nil {
		return err
	}
	if s == nil || len(s.Data[twoFAKeyPending]) == 0 {
		return errors.New("run setup first")
	}
	pending := string(s.Data[twoFAKeyPending])
	if !totp.Validate(code, pending) {
		return errCodeInvalid
	}
	return t.save(ctx, s, map[string][]byte{
		twoFAKeySecret:  []byte(pending),
		twoFAKeyEnabled: []byte("true"),
	})
}

// Disable verifies a code against the active secret and turns 2FA off.
func (t *KubeTwoFA) Disable(ctx context.Context, code string) error {
	s, err := t.load(ctx)
	if err != nil {
		return err
	}
	if s == nil || len(s.Data[twoFAKeySecret]) == 0 {
		return nil // already off
	}
	if !totp.Validate(code, string(s.Data[twoFAKeySecret])) {
		return errCodeInvalid
	}
	return t.save(ctx, s, map[string][]byte{twoFAKeyEnabled: []byte("false")})
}

// Validate checks a login code against the active secret.
func (t *KubeTwoFA) Validate(ctx context.Context, code string) bool {
	s, err := t.load(ctx)
	if err != nil || s == nil {
		return false
	}
	return totp.Validate(code, string(s.Data[twoFAKeySecret]))
}

func qrDataURI(key *otp.Key) (string, error) {
	img, err := key.Image(220, 220)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// ---- HTTP handlers (all authenticated) ----

func (s *Server) twoFAStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": s.TwoFA != nil && s.TwoFA.Enabled(r.Context())})
}

func (s *Server) twoFASetup(w http.ResponseWriter, r *http.Request) {
	if s.TwoFA == nil {
		writeError(w, http.StatusServiceUnavailable, "2FA not configured")
		return
	}
	secret, url, qr, err := s.TwoFA.Setup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "otpauthUrl": url, "qr": qr})
}

func (s *Server) twoFAEnable(w http.ResponseWriter, r *http.Request) {
	s.twoFAMutate(w, r, func(code string) error { return s.TwoFA.Enable(r.Context(), code) })
}

func (s *Server) twoFADisable(w http.ResponseWriter, r *http.Request) {
	s.twoFAMutate(w, r, func(code string) error { return s.TwoFA.Disable(r.Context(), code) })
}

func (s *Server) twoFAMutate(w http.ResponseWriter, r *http.Request, fn func(code string) error) {
	if s.TwoFA == nil {
		writeError(w, http.StatusServiceUnavailable, "2FA not configured")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := fn(body.Code); err != nil {
		if errors.Is(err, errCodeInvalid) {
			writeError(w, http.StatusBadRequest, "mã 2FA không đúng")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": s.TwoFA.Enabled(r.Context())})
}
