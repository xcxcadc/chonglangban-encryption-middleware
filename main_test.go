package main

import (
	"net/http/httptest"
	"testing"

	"github.com/xcxcadc/chonglangban-encryption-middleware/internal/config"
)

func TestRequestProtocolClassification(t *testing.T) {
	cfg := config.Config{
		PathPrefix:             "/clb/clb",
		PathPrefixes:           []string{"/clb/clb"},
		PlainSubscriptionPaths: []string{"/api/v1/client/subscribe"},
	}

	tests := []struct {
		name     string
		path     string
		plain    bool
		expected string
	}{
		{name: "legacy", path: "/clb/clb/encrypted", expected: "legacy-v1"},
		{name: "aead", path: "/clb/clb/v2.payload", expected: "aead-v2"},
		{name: "unmatched", path: "/wrong/path", expected: "rejected"},
		{name: "plain subscription", path: "/clb/clb/api/v1/client/subscribe", plain: true, expected: "plain-subscription"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caseConfig := cfg
			caseConfig.AllowPlainSubscriptions = tt.plain
			req := httptest.NewRequest("GET", tt.path, nil)
			if got := requestProtocol(caseConfig, req); got != tt.expected {
				t.Fatalf("requestProtocol() = %q, want %q", got, tt.expected)
			}
		})
	}
}
