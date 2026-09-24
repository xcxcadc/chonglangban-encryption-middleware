package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/xcxcadc/chonglangban-encryption-middleware/internal/config"
	"github.com/xcxcadc/chonglangban-encryption-middleware/internal/crypto"
)

type Handler struct {
	Config    config.Config
	Transport http.RoundTripper
}

func NewHandler(cfg config.Config) *Handler {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: cfg.RequestTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: cfg.RequestTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &Handler{Config: cfg, Transport: transport}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.applyCORS(w, r)
	if r.Method == http.MethodOptions {
		if r.Header.Get("Origin") != "" && !h.Config.OriginAllowed(r.Header.Get("Origin")) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin not allowed"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if h.Config.AllowPlainSubscriptions && h.Config.PlainSubscriptionAllowed(r.URL.Path) {
		h.forward(w, r, r.URL.Path, r.URL.RawQuery)
		return
	}

	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, h.Config.MaxBodyBytes)
	}
	path, ok := h.Config.RemoveEncryptedPrefix(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "path not found"})
		return
	}

	if h.Config.PaymentNotifyAllowed(path) {
		h.forward(w, r, path, r.URL.RawQuery)
		return
	}

	if h.Config.AllowPlainSubscriptions && (h.isSubscriptionPath(path) || h.Config.PlainSubscriptionAllowed(path)) {
		h.forward(w, r, path, r.URL.RawQuery)
		return
	}

	segment := strings.TrimPrefix(path, "/")
	if segment == "" || strings.Contains(segment, "/") || len(segment) > 32768 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid encrypted path"})
		return
	}
	iv := r.Header.Get("X-IV")
	if !strings.HasPrefix(segment, "v2.") && !validIV(iv) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or missing X-IV"})
		return
	}

	logical, err := h.decryptPath(segment, iv)
	if err != nil {
		if h.Config.DebugMode {
			log.Printf("decrypt failed: %v", err)
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid encrypted path"})
		return
	}
	target, err := h.targetForLogicalPath(logical)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.forwardTarget(w, r, target)
}

func (h *Handler) decryptPath(segment, iv string) (string, error) {
	if strings.HasPrefix(segment, "v2.") {
		if h.Config.EncryptionProtocol == "legacy" || h.Config.AEADKey == "" {
			return "", fmt.Errorf("AEAD protocol is not enabled")
		}
		return crypto.DecryptAEADPath(segment, h.Config.AEADKey)
	}
	if h.Config.EncryptionProtocol == "aead" {
		return "", fmt.Errorf("AEAD protocol is required")
	}
	if !validIV(iv) {
		return "", fmt.Errorf("invalid or missing X-IV")
	}
	return crypto.DecryptPath(segment, h.Config.AESKey, iv)
}

func (h *Handler) targetForLogicalPath(value string) (*url.URL, error) {
	logical, err := url.ParseRequestURI(value)
	if err != nil || logical.IsAbs() || logical.Host != "" || logical.Fragment != "" || !strings.HasPrefix(logical.Path, "/") {
		return nil, fmt.Errorf("invalid logical path")
	}
	if strings.ContainsAny(logical.Path, "\x00\r\n") {
		return nil, fmt.Errorf("invalid logical path")
	}
	path := logical.Path
	if !h.isSubscriptionPath(path) && !h.Config.PlainSubscriptionAllowed(path) {
		path = joinPath(h.Config.APIPrefix, path)
	}
	return h.backendTarget(path, logical.RawQuery), nil
}

func (h *Handler) isSubscriptionPath(path string) bool {
	return h.Config.IsSubscriptionPath(path)
}

func (h *Handler) backendTarget(path, rawQuery string) *url.URL {
	target := *h.Config.BackendAPIURL
	target.Path = joinPath(h.Config.BackendAPIURL.Path, path)
	target.RawPath = ""
	target.RawQuery = rawQuery
	target.Fragment = ""
	return &target
}

func (h *Handler) forward(w http.ResponseWriter, r *http.Request, path, rawQuery string) {
	h.forwardTarget(w, r, h.backendTarget(path, rawQuery))
}

func (h *Handler) forwardTarget(w http.ResponseWriter, r *http.Request, target *url.URL) {
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: target.Scheme, Host: target.Host})
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = target.Path
		req.URL.RawPath = target.RawPath
		req.URL.RawQuery = target.RawQuery
		req.Host = target.Host
		req.Header.Del("X-IV")
	}
	proxy.Transport = h.Transport
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Credentials")
		resp.Header.Del("Access-Control-Allow-Headers")
		resp.Header.Del("Access-Control-Allow-Methods")
		return nil
	}
	proxy.ErrorHandler = func(writer http.ResponseWriter, _ *http.Request, err error) {
		if h.Config.DebugMode {
			log.Printf("backend proxy failed: %v", err)
		}
		writeJSON(writer, http.StatusBadGateway, map[string]string{"error": "backend unavailable"})
	}
	proxy.ServeHTTP(w, r)
}

func (h *Handler) applyCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" || !h.Config.OriginAllowed(origin) {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Length, Content-Type, Accept, Authorization, X-IV")
	w.Header().Set("Access-Control-Max-Age", "600")
	w.Header().Add("Vary", "Origin")
}

func validIV(value string) bool {
	if len(value) != 16 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func joinPath(prefix, path string) string {
	if prefix == "" || prefix == "/" {
		if strings.HasPrefix(path, "/") {
			return path
		}
		return "/" + path
	}
	return strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(path, "/")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
