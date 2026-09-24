package crypto

import (
	"encoding/base64"
	"net/url"
	"testing"
)

func TestChonglangbanEnvelopeRoundTrip(t *testing.T) {
	key := "0123456789abcdef"
	iv := "0123456789abcdef"
	plain := "/user/order/check?trade_no=sample-123"

	encoded, err := EncodePath(plain, key, iv)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptPath(encoded, key, iv)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("got %q, want %q", got, plain)
	}
}

func TestDecryptRejectsInvalidPadding(t *testing.T) {
	if _, err := DecryptPath("not-a-valid-payload", "0123456789abcdef", "0123456789abcdef"); err == nil {
		t.Fatal("expected invalid payload error")
	}
}

func TestDecryptsKnownEzCompatibleVector(t *testing.T) {
	// 该向量由 CryptoJS AES-CBC/PKCS7 生成，验证线上 EZ 前端的字节级兼容性。
	cryptoJSBase64 := "Jb1W01Tvp8mJ9r6bl2TZNK6zSsEYjMl0ihcN1VNA+EL2YnBO09NHvFQu7RimLlVd"
	encoded := url.PathEscape(base64.StdEncoding.EncodeToString([]byte(cryptoJSBase64)))
	got, err := DecryptPath(encoded, "0123456789abcdef", "abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	want := "/user/order/check?trade_no=sample-123"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAEADPathRoundTrip(t *testing.T) {
	key := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	plain := "/api/v1/client/subscribe?token=sample-123"

	encoded, err := EncodeAEADPath(plain, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) < 4 || encoded[:3] != "v2." {
		t.Fatalf("unexpected AEAD prefix: %q", encoded[:3])
	}
	got, err := DecryptAEADPath(encoded, key)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("got %q, want %q", got, plain)
	}

	last := encoded[len(encoded)-1]
	changed := encoded[:len(encoded)-1] + "A"
	if changed[len(changed)-1] == last {
		changed = encoded[:len(encoded)-1] + "B"
	}
	if _, err := DecryptAEADPath(changed, key); err == nil {
		t.Fatal("expected tampered AEAD payload to be rejected")
	}
}
