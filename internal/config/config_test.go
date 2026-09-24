package config

import "testing"

func TestAEADOnlyDoesNotRequireLegacyKey(t *testing.T) {
	t.Setenv("BACKEND_API_URL", "https://panel.example.com")
	t.Setenv("ENCRYPTION_PROTOCOL", "aead")
	t.Setenv("AES_KEY", "")
	t.Setenv("AEAD_KEY", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AESKey != "" || cfg.EncryptionProtocol != "aead" {
		t.Fatalf("unexpected config: protocol=%q legacy_key_present=%v", cfg.EncryptionProtocol, cfg.AESKey != "")
	}
	if cfg.AllowPlainSubscriptions {
		t.Fatal("plain subscriptions should be disabled by default")
	}
}

func TestAutoModeRequiresLegacyKey(t *testing.T) {
	t.Setenv("BACKEND_API_URL", "https://panel.example.com")
	t.Setenv("ENCRYPTION_PROTOCOL", "auto")
	t.Setenv("AES_KEY", "")
	t.Setenv("AEAD_KEY", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")

	if _, err := Load(); err == nil {
		t.Fatal("auto mode must require AES_KEY for v1 compatibility")
	}
}
