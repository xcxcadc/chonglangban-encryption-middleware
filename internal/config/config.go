package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                      string
	ServiceLabel              string
	BackendAPIURL             *url.URL
	AESKey                    string
	PathPrefix                string
	PathPrefixes              []string
	APIPrefix                 string
	SubscriptionPrefix        string
	SubscriptionPrefixes      []string
	PlainSubscriptionPaths    []string
	AllowedOrigins            []string
	AllowedPaymentNotifyPaths []string
	AllowPlainSubscriptions   bool
	RequestTimeout            time.Duration
	MaxBodyBytes              int64
	EnableLogging             bool
	DebugMode                 bool
}

func Load() (Config, error) {
	loadDotEnv(".env")

	backendRaw := env("BACKEND_API_URL", "")
	if backendRaw == "" {
		return Config{}, errors.New("BACKEND_API_URL is required")
	}
	backend, err := url.Parse(backendRaw)
	if err != nil || backend.Scheme == "" || backend.Host == "" || (backend.Scheme != "http" && backend.Scheme != "https") {
		return Config{}, fmt.Errorf("BACKEND_API_URL must be an http(s) URL")
	}
	if backend.RawQuery != "" || backend.Fragment != "" {
		return Config{}, errors.New("BACKEND_API_URL must not contain a query or fragment")
	}

	key := env("AES_KEY", "")
	if len(key) != 16 || !isHex(key) {
		return Config{}, errors.New("AES_KEY must be exactly 16 hexadecimal characters")
	}

	timeout, err := time.ParseDuration(env("REQUEST_TIMEOUT", "20000") + "ms")
	if err != nil || timeout < time.Second {
		return Config{}, errors.New("REQUEST_TIMEOUT must be milliseconds and at least 1000")
	}
	maxBody, err := strconv.ParseInt(env("MAX_BODY_BYTES", "8388608"), 10, 64)
	if err != nil || maxBody < 1024 {
		return Config{}, errors.New("MAX_BODY_BYTES must be at least 1024")
	}

	pathPrefix := normalizePrefix(env("PATH_PREFIX", "/clb/clb"))
	pathPrefixes := normalizePrefixes(env("ENCRYPTED_PATH_PREFIXES", pathPrefix))
	subscriptionPrefix := normalizePrefix(env("SUBSCRIPTION_PREFIX", "/sub"))
	subscriptionPrefixes := normalizePrefixes(env("SUBSCRIPTION_PREFIXES", subscriptionPrefix))
	paymentPaths := env("PAYMENT_NOTIFY_PATHS", env("ALLOWED_PAYMENT_NOTIFY_PATHS", ""))

	return Config{
		Port:                      env("PORT", "3000"),
		ServiceLabel:              env("SERVICE_LABEL", "chonglangban-encryption-middleware"),
		BackendAPIURL:             backend,
		AESKey:                    key,
		PathPrefix:                firstOrEmpty(pathPrefixes),
		PathPrefixes:              pathPrefixes,
		APIPrefix:                 normalizePrefix(env("API_PREFIX", "/api/v1")),
		SubscriptionPrefix:        firstOrEmpty(subscriptionPrefixes),
		SubscriptionPrefixes:      subscriptionPrefixes,
		PlainSubscriptionPaths:    csv(env("PLAIN_SUBSCRIPTION_PATHS", "/api/v1/client/subscribe")),
		AllowedOrigins:            csv(env("ALLOWED_ORIGINS", "*")),
		AllowedPaymentNotifyPaths: csv(paymentPaths),
		AllowPlainSubscriptions:   boolEnv("ALLOW_PLAIN_SUBSCRIPTIONS", true),
		RequestTimeout:            timeout,
		MaxBodyBytes:              maxBody,
		EnableLogging:             boolEnv("ENABLE_LOGGING", false),
		DebugMode:                 boolEnv("DEBUG_MODE", false),
	}, nil
}

func (c Config) OriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	for _, allowed := range c.AllowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}

func (c Config) PaymentNotifyAllowed(path string) bool {
	return matchesPathPattern(c.AllowedPaymentNotifyPaths, path)
}

func (c Config) PlainSubscriptionAllowed(path string) bool {
	return matchesPathPattern(c.PlainSubscriptionPaths, path)
}

func (c Config) RemoveEncryptedPrefix(path string) (string, bool) {
	prefixes := c.PathPrefixes
	if len(prefixes) == 0 && c.PathPrefix != "" {
		prefixes = []string{c.PathPrefix}
	}
	longest := ""
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			if len(prefix) > len(longest) {
				longest = prefix
			}
		}
	}
	if longest == "" {
		return "", false
	}
	if path == longest {
		return "/", true
	}
	return strings.TrimPrefix(path, longest), true
}

func (c Config) IsSubscriptionPath(path string) bool {
	prefixes := c.SubscriptionPrefixes
	if len(prefixes) == 0 && c.SubscriptionPrefix != "" {
		prefixes = []string{c.SubscriptionPrefix}
	}
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func csv(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			values = append(values, item)
		}
	}
	return values
}

func boolEnv(key string, fallback bool) bool {
	value := strings.ToLower(env(key, strconv.FormatBool(fallback)))
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func normalizePrefix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	return "/" + strings.Trim(value, "/")
}

func normalizePrefixes(value string) []string {
	var result []string
	for _, item := range csv(value) {
		prefix := normalizePrefix(item)
		if prefix == "" {
			continue
		}
		result = append(result, prefix)
	}
	return result
}

func firstOrEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func matchesPathPattern(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if strings.HasSuffix(pattern, "*") && strings.HasPrefix(path, strings.TrimSuffix(pattern, "*")) {
			return true
		}
		if pattern == path {
			return true
		}
	}
	return false
}

func isHex(value string) bool {
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		if key != "" {
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, value)
			}
		}
	}
}
