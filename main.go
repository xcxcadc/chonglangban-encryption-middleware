package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/xcxcadc/chonglangban-encryption-middleware/internal/config"
	"github.com/xcxcadc/chonglangban-encryption-middleware/internal/proxy"
)

func main() {
	// Supervisor/aaPanel 将标准输出作为“运行日志”。Go 的默认 logger 写入
	// stderr，会导致正常请求全部出现在“错误日志”或被面板隐藏，因此显式写到 stdout。
	log.SetOutput(os.Stdout)
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	handler := proxy.NewHandler(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": cfg.ServiceLabel})
	})
	mux.Handle("/", handler)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           loggingMiddleware(cfg, mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.RequestTimeout + 5*time.Second,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       90 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("middleware listening port=%s protocol=%s path_prefix=%s plain_subscriptions=%t", cfg.Port, cfg.EncryptionProtocol, cfg.PathPrefix, cfg.AllowPlainSubscriptions)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func loggingMiddleware(cfg config.Config, next http.Handler) http.Handler {
	if !cfg.EnableLogging {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		log.Printf("request method=%s route=%s protocol=%s status=%d duration=%s", r.Method, requestRoute(cfg, r), requestProtocol(cfg, r), recorder.statusCode(), time.Since(started).Round(time.Millisecond))
	})
}

// statusRecorder 保留响应状态码，同时保留反向代理所需的 Flush 能力。
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.written {
		return
	}
	w.status = status
	w.written = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusRecorder) Flush() {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusRecorder) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusRecorder) statusCode() int {
	if w.written {
		return w.status
	}
	return http.StatusOK
}

func requestRoute(cfg config.Config, r *http.Request) string {
	if r.URL.Path == "/healthz" {
		return "healthz"
	}
	if r.Method == http.MethodOptions {
		return "preflight"
	}
	if _, ok := cfg.RemoveEncryptedPrefix(r.URL.Path); !ok {
		return "unmatched"
	}
	return cfg.PathPrefix
}

// requestProtocol 只返回协议类型，不记录加密段、Token 或解密后的真实路径。
func requestProtocol(cfg config.Config, r *http.Request) string {
	if r.URL.Path == "/healthz" {
		return "health"
	}
	if r.Method == http.MethodOptions {
		return "cors"
	}
	path, ok := cfg.RemoveEncryptedPrefix(r.URL.Path)
	if !ok {
		return "rejected"
	}
	if cfg.PaymentNotifyAllowed(path) {
		return "payment-allowlist"
	}
	if cfg.AllowPlainSubscriptions && (cfg.IsSubscriptionPath(path) || cfg.PlainSubscriptionAllowed(path)) {
		return "plain-subscription"
	}
	segment := strings.TrimPrefix(path, "/")
	if strings.HasPrefix(segment, "v2.") {
		return "aead-v2"
	}
	if segment == "" || strings.Contains(segment, "/") {
		return "rejected"
	}
	return "legacy-v1"
}
