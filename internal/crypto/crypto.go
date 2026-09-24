package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

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
