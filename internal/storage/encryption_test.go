package storage

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// generateTestMasterKey generates a valid 32-byte base64-encoded key for testing.
func generateTestMasterKey() string {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestNewEncryptor(t *testing.T) {
	tests := []struct {
		name      string
		masterKey string
		wantErr   bool
	}{
		{
			name:      "valid 32-byte key",
			masterKey: generateTestMasterKey(),
			wantErr:   false,
		},
		{
			name:      "empty key",
			masterKey: "",
			wantErr:   true,
		},
		{
			name:      "invalid base64",
			masterKey: "not-valid-base64!!!",
			wantErr:   true,
		},
		{
			name:      "key too short",
			masterKey: base64.StdEncoding.EncodeToString([]byte("short")),
			wantErr:   true,
		},
		{
			name:      "key too long",
			masterKey: base64.StdEncoding.EncodeToString(make([]byte, 64)),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := NewEncryptor(tt.masterKey)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if enc == nil {
				t.Error("expected encryptor, got nil")
			}
		})
	}
}

func TestGenerateFileKey(t *testing.T) {
	enc, err := NewEncryptor(generateTestMasterKey())
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	key1, err := enc.GenerateFileKey()
	if err != nil {
		t.Fatalf("failed to generate file key: %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("expected 32-byte key, got %d bytes", len(key1))
	}

	// Generate another key and ensure they're different
	key2, err := enc.GenerateFileKey()
	if err != nil {
		t.Fatalf("failed to generate second file key: %v", err)
	}

	if bytes.Equal(key1, key2) {
		t.Error("generated keys should be different")
	}
}

func TestEncryptDecryptFileKey(t *testing.T) {
	enc, err := NewEncryptor(generateTestMasterKey())
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	// Generate a file key
	originalKey, err := enc.GenerateFileKey()
	if err != nil {
		t.Fatalf("failed to generate file key: %v", err)
	}

	// Encrypt the file key
	encryptedKey, iv, err := enc.EncryptFileKey(originalKey)
	if err != nil {
		t.Fatalf("failed to encrypt file key: %v", err)
	}

	if len(encryptedKey) == 0 {
		t.Error("encrypted key should not be empty")
	}
	if len(iv) == 0 {
		t.Error("IV should not be empty")
	}

	// Decrypt the file key
	decryptedKey, err := enc.DecryptFileKey(encryptedKey, iv)
	if err != nil {
		t.Fatalf("failed to decrypt file key: %v", err)
	}

	if !bytes.Equal(originalKey, decryptedKey) {
		t.Error("decrypted key does not match original")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	enc, err := NewEncryptor(generateTestMasterKey())
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "short data",
			data: []byte("hello"),
		},
		{
			name: "medium data",
			data: []byte("this is a longer piece of test data that spans multiple blocks"),
		},
		{
			name: "binary data",
			data: []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD},
		},
		{
			name: "empty data",
			data: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate a file key for this test
			fileKey, err := enc.GenerateFileKey()
			if err != nil {
				t.Fatalf("failed to generate file key: %v", err)
			}

			// Encrypt
			ciphertext, err := enc.Encrypt(tt.data, fileKey)
			if err != nil {
				t.Fatalf("failed to encrypt: %v", err)
			}

			// Ciphertext should be different from plaintext (unless empty)
			if len(tt.data) > 0 && bytes.Equal(ciphertext, tt.data) {
				t.Error("ciphertext should differ from plaintext")
			}

			// Decrypt
			plaintext, err := enc.Decrypt(ciphertext, fileKey)
			if err != nil {
				t.Fatalf("failed to decrypt: %v", err)
			}

			if !bytes.Equal(plaintext, tt.data) {
				t.Errorf("decrypted data does not match original: got %v, want %v", plaintext, tt.data)
			}
		})
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	enc, err := NewEncryptor(generateTestMasterKey())
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	data := []byte("secret message")

	// Encrypt with one key
	key1, _ := enc.GenerateFileKey()
	ciphertext, err := enc.Encrypt(data, key1)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	// Try to decrypt with a different key
	key2, _ := enc.GenerateFileKey()
	_, err = enc.Decrypt(ciphertext, key2)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDecryptTooShortCiphertext(t *testing.T) {
	enc, err := NewEncryptor(generateTestMasterKey())
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	key, _ := enc.GenerateFileKey()

	// Try to decrypt data that's too short to contain an IV
	_, err = enc.Decrypt([]byte{0x01, 0x02}, key)
	if err == nil {
		t.Error("expected error when decrypting too-short ciphertext")
	}
}

func TestEncryptionDeterminism(t *testing.T) {
	enc, err := NewEncryptor(generateTestMasterKey())
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	data := []byte("same data")
	key, _ := enc.GenerateFileKey()

	// Encrypt the same data twice
	ciphertext1, _ := enc.Encrypt(data, key)
	ciphertext2, _ := enc.Encrypt(data, key)

	// Due to random IVs, ciphertexts should be different
	if bytes.Equal(ciphertext1, ciphertext2) {
		t.Error("encrypting same data should produce different ciphertexts due to random IVs")
	}
}
