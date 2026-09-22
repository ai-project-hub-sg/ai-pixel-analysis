package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// Encrypt AES-256-GCM 加密，key 经 SHA256 派生；输出 base64(nonce||ct)
func Encrypt(plaintext, key string) (string, error) {
	k := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(k[:])
	if err != nil { return "", err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return "", err }
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return "", err }
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt 解密 Encrypt 的输出
func Decrypt(encoded, key string) (string, error) {
	k := sha256.Sum256([]byte(key))
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil { return "", fmt.Errorf("base64: %w", err) }
	block, err := aes.NewCipher(k[:])
	if err != nil { return "", err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return "", err }
	if len(raw) < gcm.NonceSize() { return "", fmt.Errorf("ciphertext too short") }
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil { return "", fmt.Errorf("decrypt: %w", err) }
	return string(pt), nil
}
