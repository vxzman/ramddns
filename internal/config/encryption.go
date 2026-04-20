package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ensureKeyFile ensures the encryption key file exists
func ensureKeyFile(keyPath string) ([]byte, error) {
	if _, err := os.Stat(keyPath); err == nil {
		return os.ReadFile(keyPath)
	}

	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, k, 0600); err != nil {
		return nil, err
	}
	return k, nil
}

func encryptValue(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return "enc:" + base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

func decryptValue(enc string, key []byte) (string, error) {
	if !strings.HasPrefix(enc, "enc:") {
		return enc, nil
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, "enc:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	pt, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func isProtected(s string) bool {
	return strings.HasPrefix(s, "enc:") || (strings.HasPrefix(s, "$") && !strings.HasPrefix(s, "${"))
}

// EncryptConfigSecrets encrypts plaintext secrets in config and writes back
func EncryptConfigSecrets(cfg *Config, configPath string, baseDir string) (bool, error) {
	if baseDir == "" {
		baseDir = filepath.Dir(configPath)
	}
	keyPath := filepath.Join(baseDir, ".aiolos.key")
	key, err := ensureKeyFile(keyPath)
	if err != nil {
		return false, fmt.Errorf("failed to ensure key file: %w", err)
	}

	changed := false

	// Encrypt environment
	if cfg.Environment == nil {
		cfg.Environment = make(map[string]string)
	}
	for k, v := range cfg.Environment {
		if v == "" || isProtected(v) {
			continue
		}
		enc, err := encryptValue(v, key)
		if err != nil {
			return false, err
		}
		cfg.Environment[k] = enc
		changed = true
	}

	// Encrypt record secrets
	for i := range cfg.Records {
		changed = encryptRecord(&cfg.Records[i], key) || changed
	}

	if changed {
		if err := WriteConfig(configPath, cfg); err != nil {
			return false, fmt.Errorf("failed to write config after encrypting secrets: %w", err)
		}
	}
	return changed, nil
}

func encryptRecord(r *RecordConfig, key []byte) bool {
	changed := false
	if r.Cloudflare != nil {
		changed = encryptField(&r.Cloudflare.APIToken, key) || changed
		changed = encryptField(&r.Cloudflare.ZoneID, key) || changed
	}
	if r.Aliyun != nil {
		changed = encryptField(&r.Aliyun.AccessKeyID, key) || changed
		changed = encryptField(&r.Aliyun.AccessKeySecret, key) || changed
	}
	return changed
}

func encryptField(field *string, key []byte) bool {
	if *field == "" || isProtected(*field) {
		return false
	}
	if enc, err := encryptValue(*field, key); err == nil {
		*field = enc
		return true
	}
	return false
}
