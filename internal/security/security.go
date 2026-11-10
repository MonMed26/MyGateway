// Package security provides token generation, hashing, webhook signing and
// session encryption helpers.
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MonMed26/MyGateway/utils"
)

// NewToken returns a random hex token of n bytes (2*n hex chars).
func NewToken(n int) string {
	if n <= 0 {
		n = 32
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is unrecoverable; fall back to a panic so callers
		// never silently persist a predictable secret.
		panic(fmt.Sprintf("security: rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}

// HashToken returns the SHA-256 hex digest of a token. Session tokens are
// stored hashed so a database leak does not hand over live sessions.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SignWebhook returns the HMAC-SHA256 hex signature of payload using secret.
func SignWebhook(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// EncryptSession encrypts a provider session (JSON) at rest with the master
// key, returning a base64 string suitable for a TEXT column.
func EncryptSession(plaintext []byte, masterKey string) (string, error) {
	if masterKey == "" {
		return "", fmt.Errorf("master key is required")
	}
	enc, err := utils.Encrypt(plaintext, masterKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(enc), nil
}

// DecryptSession reverses EncryptSession.
func DecryptSession(encoded, masterKey string) ([]byte, error) {
	if masterKey == "" {
		return nil, fmt.Errorf("master key is required")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, err
	}
	return utils.Decrypt(raw, masterKey)
}

// LoadOrCreateMasterKey returns the master key from the environment, or reads
// (creating if needed) a random key file at path. The file is written 0600.
func LoadOrCreateMasterKey(envValue, path string) (string, error) {
	if strings.TrimSpace(envValue) != "" {
		return envValue, nil
	}
	data, err := os.ReadFile(path)
	if err == nil {
		key := strings.TrimSpace(string(data))
		if key != "" {
			return key, nil
		}
	}
	if !os.IsNotExist(err) && err != nil {
		return "", err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	key := NewToken(32)
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		return "", err
	}
	return key, nil
}
