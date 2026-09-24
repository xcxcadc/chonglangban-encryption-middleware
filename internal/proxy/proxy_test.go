package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/xcxcadc/chonglangban-encryption-middleware/internal/config"
	"github.com/xcxcadc/chonglangban-encryption-middleware/internal/crypto"
)

func TestEncryptedRequestReachesV2BoardPath(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user/info" || r.URL.RawQuery != "view=full" {
			t.Errorf("backend received %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("authorization header was not forwarded")
		}
		w.Header().Set("Access-Control-Allow-Origin", "backend-should-be-removed")
		_, _ = io.WriteString(w, `{"data":{"ok":true}}`)
	}))
	defer backend.Close()
	backendURL, _ := url.Parse(backend.URL)
	cfg := config.Config{
		BackendAPIURL:      backendURL,
		AESKey:             "0123456789abcdef",
		PathPrefix:         "/clb/clb",
		APIPrefix:          "/api/v1",
		SubscriptionPrefix: "/sub",
		AllowedOrigins:     []string{"*"},
		MaxBodyBytes:       1024 * 1024,
		RequestTimeout:     5 * 1000 * 1000000,
	}
	h := NewHandler(cfg)
	iv := "0123456789abcdef"
	token, err := crypto.EncodePath("/user/info?view=full", cfg.AESKey, iv)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/clb/clb/"+token, nil)
	req.Header.Set("Origin", "https://panel.example.com")
	req.Header.Set("X-IV", iv)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://panel.example.com" {
		t.Fatalf("cors origin=%q", got)
	}
}

func TestAEADRequestReachesV2BoardPathWithoutXIV(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user/info" || r.URL.RawQuery != "view=full" {
			t.Errorf("backend received %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"data":{"ok":true}}`)
	}))
	defer backend.Close()
	backendURL, _ := url.Parse(backend.URL)
	key := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	cfg := config.Config{
		BackendAPIURL:      backendURL,
		AEADKey:            key,
		EncryptionProtocol: "aead",
		PathPrefixes:       []string{"/clb/clb"},
		APIPrefix:          "/api/v1",
		AllowedOrigins:     []string{"*"},
		MaxBodyBytes:       1024 * 1024,
		RequestTimeout:     5 * time.Second,
	}
	h := NewHandler(cfg)
	token, err := crypto.EncodeAEADPath("/user/info?view=full", key)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/clb/clb/"+token, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPaymentNotifyCanBypassEncryptionWhenWhitelisted(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/guest/payment/notify/Stripe/demo" {
			t.Errorf("backend received %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()
	backendURL, _ := url.Parse(backend.URL)
	cfg := config.Config{BackendAPIURL: backendURL, PathPrefix: "/clb/clb", AllowedPaymentNotifyPaths: []string{"/api/v1/guest/payment/notify/*"}, AllowedOrigins: []string{"*"}, MaxBodyBytes: 1024 * 1024, RequestTimeout: 5 * 1000 * 1000000}
	h := NewHandler(cfg)
	req := httptest.NewRequest(http.MethodPost, "/clb/clb/api/v1/guest/payment/notify/Stripe/demo", strings.NewReader("trade_no=demo"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlainV2BoardSubscriptionCanUseItsOwnPath(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/client/subscribe" || r.URL.Query().Get("token") != "demo" {
			t.Errorf("backend received %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	backendURL, _ := url.Parse(backend.URL)
	cfg := config.Config{
		BackendAPIURL:           backendURL,
		PathPrefixes:            []string{"/clb/clb"},
		PlainSubscriptionPaths:  []string{"/api/v1/client/subscribe"},
		AllowedOrigins:          []string{"*"},
		AllowPlainSubscriptions: true,
		MaxBodyBytes:            1024 * 1024,
		RequestTimeout:          5 * time.Second,
	}
	h := NewHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/client/subscribe?token=demo", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAEADSubscriptionCanBeEncrypted(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/client/subscribe" || r.URL.Query().Get("token") != "demo" {
			t.Errorf("backend received %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	backendURL, _ := url.Parse(backend.URL)
	key := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	cfg := config.Config{
		BackendAPIURL:          backendURL,
		AEADKey:                key,
		EncryptionProtocol:     "aead",
		PathPrefixes:           []string{"/clb/clb"},
		PlainSubscriptionPaths: []string{"/api/v1/client/subscribe"},
		AllowedOrigins:         []string{"*"},
		MaxBodyBytes:           1024 * 1024,
		RequestTimeout:         5 * time.Second,
	}
	h := NewHandler(cfg)
	token, err := crypto.EncodeAEADPath("/api/v1/client/subscribe?token=demo", key)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/clb/clb/"+token, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
