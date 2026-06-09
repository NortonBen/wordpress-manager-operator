package apiserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestTwoFAStoreLifecycle(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()
	store := &KubeTwoFA{K8s: k8s, Namespace: "wordpress-system", SecretName: "admin-2fa", Issuer: "WP", Account: "admin"}
	ctx := context.Background()

	if store.Enabled(ctx) {
		t.Fatal("2FA should start disabled")
	}

	secret, url, qr, err := store.Setup(ctx)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if secret == "" || !strings.HasPrefix(url, "otpauth://") || !strings.HasPrefix(qr, "data:image/png;base64,") {
		t.Fatalf("unexpected setup output: url=%q qr-prefix-ok=%v", url, strings.HasPrefix(qr, "data:image/png"))
	}
	if store.Enabled(ctx) {
		t.Error("pending setup must not enable 2FA yet")
	}

	// Wrong code does not enable.
	if err := store.Enable(ctx, "000000"); err == nil {
		t.Error("enable with wrong code should fail")
	}
	if store.Enabled(ctx) {
		t.Error("2FA should still be disabled after wrong code")
	}

	// Correct code enables.
	code, _ := totp.GenerateCode(secret, time.Now())
	if err := store.Enable(ctx, code); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !store.Enabled(ctx) {
		t.Fatal("2FA should be enabled")
	}
	code2, _ := totp.GenerateCode(secret, time.Now())
	if !store.Validate(ctx, code2) {
		t.Error("valid code should pass")
	}
	if store.Validate(ctx, "111111") {
		t.Error("invalid code should fail")
	}

	// Disable requires a valid code.
	if err := store.Disable(ctx, "000000"); err == nil {
		t.Error("disable with wrong code should fail")
	}
	code3, _ := totp.GenerateCode(secret, time.Now())
	if err := store.Disable(ctx, code3); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if store.Enabled(ctx) {
		t.Error("2FA should be disabled after Disable")
	}
}

func TestTwoFALoginFlow(t *testing.T) {
	h := newTestServer(t)
	tok := loginToken(t, h) // logs in before 2FA is enabled

	// Enroll: setup → enable.
	w := req(t, h, "POST", "/api/v1/2fa/setup", tok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("setup: %d", w.Code)
	}
	var setup struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &setup)
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if w := req(t, h, "POST", "/api/v1/2fa/enable", tok, `{"code":"`+code+`"}`); w.Code != http.StatusOK {
		t.Fatalf("enable: %d (%s)", w.Code, w.Body.String())
	}

	// Now login without a code → 401 totp_required.
	w = req(t, h, "POST", "/api/v1/login", "", `{"username":"admin","password":"s3cret"}`)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "totp_required") {
		t.Fatalf("expected totp_required, got %d %s", w.Code, w.Body.String())
	}

	// Wrong code → 401.
	if w := req(t, h, "POST", "/api/v1/login", "", `{"username":"admin","password":"s3cret","totp":"000000"}`); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong totp should be 401, got %d", w.Code)
	}

	// Correct code → 200 with token.
	code2, _ := totp.GenerateCode(setup.Secret, time.Now())
	w = req(t, h, "POST", "/api/v1/login", "", `{"username":"admin","password":"s3cret","totp":"`+code2+`"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "token") {
		t.Fatalf("login with code should succeed, got %d %s", w.Code, w.Body.String())
	}
}

func TestTwoFAStatusEndpoint(t *testing.T) {
	h := newTestServer(t)
	tok := loginToken(t, h)
	w := req(t, h, "GET", "/api/v1/2fa", tok, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Errorf("expected enabled:false, got %d %s", w.Code, w.Body.String())
	}
}
