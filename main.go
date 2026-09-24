package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/xcxcadc/chonglangban-encryption-middleware/internal/config"
	"github.com/xcxcadc/chonglangban-encryption-middleware/internal/proxy"
)

func main() {
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

	log.Printf("chonglangban middleware listening on :%s", cfg.Port)
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
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond))
	})
}
