package auth

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestUUIDFromPgtype(t *testing.T) {
	tests := []struct {
		name string
		id   pgtype.UUID
		want uuid.UUID
	}{
		{
			name: "valid uuid",
			id: pgtype.UUID{
				Bytes: [16]byte{0x12, 0x3e, 0x45, 0x67, 0xe8, 0x9b, 0x12, 0xd3, 0xa4, 0x56, 0x42, 0x66, 0x14, 0x17, 0x40, 0x00},
				Valid: true,
			},
			want: uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		},
		{
			name: "nil uuid",
			id: pgtype.UUID{
				Bytes: [16]byte{},
				Valid: true,
			},
			want: uuid.Nil,
		},
		{
			name: "invalid pgtype uuid",
			id: pgtype.UUID{
				Bytes: [16]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11},
				Valid: false,
			},
			want: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UUIDFromPgtype(tt.id)
			if got != tt.want {
				t.Errorf("UUIDFromPgtype() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUUIDToPgtype(t *testing.T) {
	tests := []struct {
		name string
		id   uuid.UUID
		want pgtype.UUID
	}{
		{
			name: "valid uuid",
			id:   uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
			want: pgtype.UUID{
				Bytes: [16]byte{0x12, 0x3e, 0x45, 0x67, 0xe8, 0x9b, 0x12, 0xd3, 0xa4, 0x56, 0x42, 0x66, 0x14, 0x17, 0x40, 0x00},
				Valid: true,
			},
		},
		{
			name: "nil uuid",
			id:   uuid.Nil,
			want: pgtype.UUID{
				Bytes: [16]byte{},
				Valid: true,
			},
		},
		{
			name: "random uuid",
			id:   uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
			want: pgtype.UUID{
				Bytes: [16]byte{0xaa, 0xaa, 0xaa, 0xaa, 0xbb, 0xbb, 0xcc, 0xcc, 0xdd, 0xdd, 0xee, 0xee, 0xee, 0xee, 0xee, 0xee},
				Valid: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UUIDToPgtype(tt.id)
			if got != tt.want {
				t.Errorf("UUIDToPgtype() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUUIDConversionRoundTrip(t *testing.T) {
	uuids := []uuid.UUID{
		uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		uuid.MustParse("00000000-0000-0000-0000-000000000000"),
		uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		uuid.New(),
		uuid.New(),
	}

	for _, original := range uuids {
		t.Run("roundtrip_"+original.String()[:8], func(t *testing.T) {
			// Convert to pgtype
			pg := UUIDToPgtype(original)

			// Convert back to uuid
			result := UUIDFromPgtype(pg)

			if result != original {
				t.Errorf("Round trip failed: original = %v, result = %v", original, result)
			}
		})
	}
}

func TestNewSessionManager(t *testing.T) {
	tests := []struct {
		name          string
		queries       interface{}
		refreshExpiry int64 // seconds
	}{
		{
			name:          "with nil queries",
			queries:       nil,
			refreshExpiry: 3600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't fully test NewSessionManager without a real Queries instance,
			// but we can verify it doesn't panic with nil
			manager := NewSessionManager(nil, 3600)
			if manager == nil {
				t.Error("NewSessionManager() returned nil")
			}
		})
	}
}
