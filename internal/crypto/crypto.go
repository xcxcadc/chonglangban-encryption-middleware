package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const aeadAssociatedData = "chonglangban:v2:path"

// DecryptPath decodes the same envelope emitted by the Chonglangban frontend:
// encodeURIComponent(btoa(AES-CBC-PKCS7(plainPath))).
func DecryptPath(encoded string, key string, iv string) (string, error) {
	encoded, err := url.PathUnescape(encoded)
	if err != nil {
		return "", fmt.Errorf("invalid escaped path: %w", err)
	}
	outer, err := decodeBase64(encoded)
	if err != nil {
		return "", fmt.Errorf("invalid outer base64: %w", err)
	}
	inner, err := decodeBase64(string(outer))
	if err != nil {
		return "", fmt.Errorf("invalid encrypted payload: %w", err)
	}
	return decryptCBC(inner, []byte(key), []byte(iv))
}

// EncodePath is used by tests and integration tools to produce a compatible URL segment.
func EncodePath(plainPath string, key string, iv string) (string, error) {
	inner, err := encryptCBC([]byte(plainPath), []byte(key), []byte(iv))
	if err != nil {
		return "", err
	}
	ciphertext := base64.StdEncoding.EncodeToString(inner)
	outer := base64.StdEncoding.EncodeToString([]byte(ciphertext))
	return url.PathEscape(outer), nil
}

// DecryptAEADPath 解密 v2 协议路径。
// 格式为 v2.<base64url(nonce || ciphertext || tag)>，使用 AES-256-GCM。
// nonce 随密文一起传输，不能复用；GCM tag 会同时验证路径内容是否被篡改。
func DecryptAEADPath(encoded string, keyHex string) (string, error) {
	if !strings.HasPrefix(encoded, "v2.") {
		return "", errors.New("unsupported AEAD version")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return "", errors.New("AEAD_KEY must be 64 hexadecimal characters")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "v2."))
	if err != nil {
		return "", errors.New("invalid AEAD payload")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) <= aead.NonceSize()+aead.Overhead() {
		return "", errors.New("AEAD payload is too short")
	}
	nonce := payload[:aead.NonceSize()]
	plaintext, err := aead.Open(nil, nonce, payload[aead.NonceSize():], []byte(aeadAssociatedData))
	if err != nil {
		return "", errors.New("AEAD authentication failed")
	}
	return string(plaintext), nil
}

// EncodeAEADPath 生成 v2 协议路径，供集成测试和服务端工具使用。
func EncodeAEADPath(plainPath string, keyHex string) (string, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return "", errors.New("AEAD_KEY must be 64 hexadecimal characters")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate AEAD nonce: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, []byte(plainPath), []byte(aeadAssociatedData))
	payload := append(nonce, ciphertext...)
	return "v2." + base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	decoders := []*base64.Encoding{
		base64.RawURLEncoding, base64.URLEncoding,
		base64.RawStdEncoding, base64.StdEncoding,
	}
	for _, decoder := range decoders {
		if decoded, err := decoder.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("unsupported base64 variant")
}

func decryptCBC(ciphertext, key, iv []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	if len(iv) != aes.BlockSize {
		return "", errors.New("IV must be 16 bytes")
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", errors.New("ciphertext has invalid length")
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	plaintext, err = unpad(plaintext, aes.BlockSize)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func encryptCBC(plaintext, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != aes.BlockSize {
		return nil, errors.New("IV must be 16 bytes")
	}
	plaintext = pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, plaintext)
	return ciphertext, nil
}

func pad(value []byte, size int) []byte {
	padding := size - len(value)%size
	result := make([]byte, len(value)+padding)
	copy(result, value)
	for i := len(value); i < len(result); i++ {
		result[i] = byte(padding)
	}
	return result
}

func unpad(value []byte, size int) ([]byte, error) {
	if len(value) == 0 || len(value)%size != 0 {
		return nil, errors.New("invalid PKCS7 padding length")
	}
	padding := int(value[len(value)-1])
	if padding < 1 || padding > size || padding > len(value) {
		return nil, errors.New("invalid PKCS7 padding")
	}
	for _, char := range value[len(value)-padding:] {
		if int(char) != padding {
			return nil, errors.New("invalid PKCS7 padding")
		}
	}
	return value[:len(value)-padding], nil
}
