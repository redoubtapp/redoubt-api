package auth

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "valid password",
			password: "securePassword123!",
			wantErr:  false,
		},
		{
			name:     "empty password",
			password: "",
			wantErr:  false,
		},
		{
			name:     "long password",
			password: strings.Repeat("a", 1000),
			wantErr:  false,
		},
		{
			name:     "unicode password",
			password: "пароль密码🔐",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Verify hash format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
				if !strings.HasPrefix(hash, "$argon2id$v=19$") {
					t.Errorf("HashPassword() hash format invalid, got %v", hash)
				}
				parts := strings.Split(hash, "$")
				if len(parts) != 6 {
					t.Errorf("HashPassword() expected 6 parts, got %d", len(parts))
				}
			}
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	// Pre-generate hashes for test passwords
	correctHash, _ := HashPassword("correctPassword123!")

	tests := []struct {
		name     string
		password string
		hash     string
		want     bool
		wantErr  bool
	}{
		{
			name:     "correct password",
			password: "correctPassword123!",
			hash:     correctHash,
			want:     true,
			wantErr:  false,
		},
		{
			name:     "incorrect password",
			password: "wrongPassword",
			hash:     correctHash,
			want:     false,
			wantErr:  false,
		},
		{
			name:     "invalid hash format - too few parts",
			password: "anyPassword",
			hash:     "$argon2id$v=19$invalid",
			want:     false,
			wantErr:  true,
		},
		{
			name:     "invalid hash format - wrong algorithm",
			password: "anyPassword",
			hash:     "$argon2i$v=19$m=65536,t=1,p=4$c2FsdA$aGFzaA",
			want:     false,
			wantErr:  true,
		},
		{
			name:     "invalid hash format - wrong version",
			password: "anyPassword",
			hash:     "$argon2id$v=18$m=65536,t=1,p=4$c2FsdA$aGFzaA",
			want:     false,
			wantErr:  true,
		},
		{
			name:     "invalid hash format - bad params",
			password: "anyPassword",
			hash:     "$argon2id$v=19$invalid$c2FsdA$aGFzaA",
			want:     false,
			wantErr:  true,
		},
		{
			name:     "invalid hash format - bad salt encoding",
			password: "anyPassword",
			hash:     "$argon2id$v=19$m=65536,t=1,p=4$!!!invalid!!!$aGFzaA",
			want:     false,
			wantErr:  true,
		},
		{
			name:     "invalid hash format - bad hash encoding",
			password: "anyPassword",
			hash:     "$argon2id$v=19$m=65536,t=1,p=4$c2FsdA$!!!invalid!!!",
			want:     false,
			wantErr:  true,
		},
		{
			name:     "empty hash",
			password: "anyPassword",
			hash:     "",
			want:     false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := VerifyPassword(tt.password, tt.hash)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyPassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("VerifyPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeHash(t *testing.T) {
	tests := []struct {
		name        string
		encodedHash string
		wantErr     error
	}{
		{
			name:        "valid hash",
			encodedHash: "$argon2id$v=19$m=65536,t=1,p=4$c2FsdHNhbHRzYWx0$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNo",
			wantErr:     nil,
		},
		{
			name:        "too few parts",
			encodedHash: "$argon2id$v=19",
			wantErr:     ErrInvalidHash,
		},
		{
			name:        "wrong algorithm",
			encodedHash: "$bcrypt$v=19$m=65536,t=1,p=4$c2FsdA$aGFzaA",
			wantErr:     ErrInvalidHash,
		},
		{
			name:        "incompatible version",
			encodedHash: "$argon2id$v=18$m=65536,t=1,p=4$c2FsdA$aGFzaA",
			wantErr:     ErrIncompatibleVersion,
		},
		{
			name:        "malformed version",
			encodedHash: "$argon2id$vX$m=65536,t=1,p=4$c2FsdA$aGFzaA",
			wantErr:     ErrInvalidHash,
		},
		{
			name:        "malformed params",
			encodedHash: "$argon2id$v=19$invalid$c2FsdA$aGFzaA",
			wantErr:     ErrInvalidHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := decodeHash(tt.encodedHash)
			if err != tt.wantErr {
				t.Errorf("decodeHash() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHashAndVerifyRoundTrip(t *testing.T) {
	passwords := []string{
		"simplePassword",
		"Complex!P@ssw0rd#123",
		"unicode密码пароль",
		"",
		strings.Repeat("x", 100),
	}

	for _, password := range passwords {
		t.Run("roundtrip_"+password[:min(10, len(password))], func(t *testing.T) {
			hash, err := HashPassword(password)
			if err != nil {
				t.Fatalf("HashPassword() error = %v", err)
			}

			valid, err := VerifyPassword(password, hash)
			if err != nil {
				t.Fatalf("VerifyPassword() error = %v", err)
			}
			if !valid {
				t.Error("VerifyPassword() = false, want true for correct password")
			}

			// Also verify wrong password fails
			valid, err = VerifyPassword(password+"wrong", hash)
			if err != nil {
				t.Fatalf("VerifyPassword() error = %v", err)
			}
			if valid {
				t.Error("VerifyPassword() = true, want false for incorrect password")
			}
		})
	}
}

func TestHashUniqueness(t *testing.T) {
	password := "samePassword"
	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	if hash1 == hash2 {
		t.Error("HashPassword() produced identical hashes for same password (salt should differ)")
	}

	// Both should still verify correctly
	valid1, _ := VerifyPassword(password, hash1)
	valid2, _ := VerifyPassword(password, hash2)
	if !valid1 || !valid2 {
		t.Error("Both hashes should verify the same password")
	}
}
