package storage

import (
	"testing"
)

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		filename string
		want     string
	}{
		{
			name:     "PNG magic bytes",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00},
			filename: "image.png",
			want:     "image/png",
		},
		{
			name:     "JPEG magic bytes",
			data:     []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x00, 0x00, 0x00},
			filename: "photo.jpg",
			want:     "image/jpeg",
		},
		{
			name:     "GIF magic bytes",
			data:     []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x00, 0x00},
			filename: "animation.gif",
			want:     "image/gif",
		},
		{
			name:     "WebP magic bytes",
			data:     []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50},
			filename: "image.webp",
			want:     "image/webp",
		},
		{
			name:     "fallback to extension - PNG",
			data:     []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			filename: "image.png",
			want:     "image/png",
		},
		{
			name:     "fallback to extension - JPEG",
			data:     []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			filename: "photo.jpeg",
			want:     "image/jpeg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectContentType(tt.data, tt.filename)
			if got != tt.want {
				t.Errorf("detectContentType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectFromMagicBytes(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "PNG",
			data: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			want: "image/png",
		},
		{
			name: "JPEG",
			data: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46},
			want: "image/jpeg",
		},
		{
			name: "GIF87a",
			data: []byte{0x47, 0x49, 0x46, 0x38, 0x37, 0x61, 0x00, 0x00},
			want: "image/gif",
		},
		{
			name: "GIF89a",
			data: []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x00, 0x00},
			want: "image/gif",
		},
		{
			name: "WebP",
			data: []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50},
			want: "image/webp",
		},
		{
			name: "unknown",
			data: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want: "",
		},
		{
			name: "too short",
			data: []byte{0x89, 0x50},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectFromMagicBytes(tt.data)
			if got != tt.want {
				t.Errorf("detectFromMagicBytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtensionFromMIME(t *testing.T) {
	tests := []struct {
		mimeType string
		want     string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"application/octet-stream", ""},
		{"text/plain", ""},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := extensionFromMIME(tt.mimeType)
			if got != tt.want {
				t.Errorf("extensionFromMIME(%q) = %v, want %v", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestAllowedMIMETypes(t *testing.T) {
	// Verify expected types are allowed
	allowed := []string{
		"image/png",
		"image/jpeg",
		"image/webp",
		"image/gif",
	}

	for _, mime := range allowed {
		if !AllowedMIMETypes[mime] {
			t.Errorf("expected %q to be allowed", mime)
		}
	}

	// Verify some types are not allowed
	notAllowed := []string{
		"image/bmp",
		"image/tiff",
		"application/pdf",
		"text/html",
	}

	for _, mime := range notAllowed {
		if AllowedMIMETypes[mime] {
			t.Errorf("expected %q to NOT be allowed", mime)
		}
	}
}

func TestMaxFileSize(t *testing.T) {
	// Verify max file size is 5 MB
	expected := int64(5 * 1024 * 1024)
	if MaxFileSize != expected {
		t.Errorf("MaxFileSize = %d, want %d", MaxFileSize, expected)
	}
}

func TestAvatarPrefix(t *testing.T) {
	if AvatarPrefix != "avatars" {
		t.Errorf("AvatarPrefix = %q, want %q", AvatarPrefix, "avatars")
	}
}
