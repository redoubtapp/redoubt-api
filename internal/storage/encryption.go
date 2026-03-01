package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// Encryptor handles client-side encryption using AES-256-GCM.
type Encryptor struct {
	masterKey []byte
}

// NewEncryptor creates a new encryptor with the given base64-encoded master key.
// The master key must be 32 bytes (256 bits) when decoded.
func NewEncryptor(masterKeyBase64 string) (*Encryptor, error) {
	if masterKeyBase64 == "" {
		return nil, fmt.Errorf("master key is required")
	}

	masterKey, err := base64.StdEncoding.DecodeString(masterKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode master key: %w", err)
	}

	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(masterKey))
	}

	return &Encryptor{masterKey: masterKey}, nil
}

// GenerateFileKey generates a random 32-byte key for encrypting a file.
func (e *Encryptor) GenerateFileKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate file key: %w", err)
	}
	return key, nil
}

// EncryptFileKey encrypts a file key using the master key.
// Returns the encrypted key and IV.
func (e *Encryptor) EncryptFileKey(fileKey []byte) (encryptedKey, iv []byte, err error) {
	block, err := aes.NewCipher(e.masterKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	iv = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	encryptedKey = gcm.Seal(nil, iv, fileKey, nil)
	return encryptedKey, iv, nil
}

// DecryptFileKey decrypts a file key using the master key.
func (e *Encryptor) DecryptFileKey(encryptedKey, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	fileKey, err := gcm.Open(nil, iv, encryptedKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt file key: %w", err)
	}

	return fileKey, nil
}

// Encrypt encrypts data using AES-256-GCM with the given key.
// Returns the encrypted data with the IV prepended.
func (e *Encryptor) Encrypt(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// IV is prepended to the ciphertext
	ciphertext := gcm.Seal(iv, iv, data, nil)
	return ciphertext, nil
}

// Decrypt decrypts data that was encrypted with Encrypt.
// Expects the IV to be prepended to the ciphertext.
func (e *Encryptor) Decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	iv := ciphertext[:gcm.NonceSize()]
	ciphertext = ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}
