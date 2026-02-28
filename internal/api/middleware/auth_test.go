package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/redoubtapp/redoubt-api/internal/auth"
)

func TestAuth(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret", 15*time.Minute)
	userID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")

	validToken, _ := jwtManager.GenerateToken(userID, false, uuid.New())
	adminToken, _ := jwtManager.GenerateToken(userID, true, uuid.New())
	wrongSecretManager := auth.NewJWTManager("wrong-secret", 15*time.Minute)
	invalidToken, _ := wrongSecretManager.GenerateToken(userID, false, uuid.New())

	tests := []struct {
		name           string
		authHeader     string
		wantStatus     int
		wantUserInCtx  bool
		wantAdminInCtx bool
	}{
		{
			name:           "valid token",
			authHeader:     "Bearer " + validToken,
			wantStatus:     http.StatusOK,
			wantUserInCtx:  true,
			wantAdminInCtx: false,
		},
		{
			name:           "valid admin token",
			authHeader:     "Bearer " + adminToken,
			wantStatus:     http.StatusOK,
			wantUserInCtx:  true,
			wantAdminInCtx: true,
		},
		{
			name:          "missing auth header",
			authHeader:    "",
			wantStatus:    http.StatusUnauthorized,
			wantUserInCtx: false,
		},
		{
			name:          "invalid format - no bearer",
			authHeader:    validToken,
			wantStatus:    http.StatusUnauthorized,
			wantUserInCtx: false,
		},
		{
			name:          "invalid format - wrong prefix",
			authHeader:    "Basic " + validToken,
			wantStatus:    http.StatusUnauthorized,
			wantUserInCtx: false,
		},
		{
			name:          "invalid token",
			authHeader:    "Bearer invalid.token.here",
			wantStatus:    http.StatusUnauthorized,
			wantUserInCtx: false,
		},
		{
			name:          "wrong secret token",
			authHeader:    "Bearer " + invalidToken,
			wantStatus:    http.StatusUnauthorized,
			wantUserInCtx: false,
		},
		{
			name:          "bearer lowercase",
			authHeader:    "bearer " + validToken,
			wantStatus:    http.StatusOK,
			wantUserInCtx: true,
		},
		{
			name:          "bearer mixed case",
			authHeader:    "BEARER " + validToken,
			wantStatus:    http.StatusOK,
			wantUserInCtx: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotUserID uuid.UUID
			var gotIsAdmin bool
			var hadUser bool

			handler := Auth(jwtManager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotUserID, hadUser = GetUserID(r.Context())
				gotIsAdmin = GetIsAdmin(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %v, want %v", rec.Code, tt.wantStatus)
			}

			if tt.wantUserInCtx && !hadUser {
				t.Error("expected user ID in context")
			}

			if tt.wantUserInCtx && gotUserID != userID {
				t.Errorf("userID = %v, want %v", gotUserID, userID)
			}

			if tt.wantAdminInCtx && !gotIsAdmin {
				t.Error("expected admin flag in context")
			}
		})
	}
}

func TestRequireAuth(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret", 15*time.Minute)
	userID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	validToken, _ := jwtManager.GenerateToken(userID, false, uuid.New())

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "valid token",
			authHeader: "Bearer " + validToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing token",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := RequireAuth(jwtManager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %v, want %v", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequireInstanceAdmin(t *testing.T) {
	tests := []struct {
		name       string
		setupCtx   func(r *http.Request) *http.Request
		wantStatus int
	}{
		{
			name: "admin user",
			setupCtx: func(r *http.Request) *http.Request {
				ctx := context.WithValue(r.Context(), isAdminKey, true)
				return r.WithContext(ctx)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "non-admin user",
			setupCtx: func(r *http.Request) *http.Request {
				ctx := context.WithValue(r.Context(), isAdminKey, false)
				return r.WithContext(ctx)
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "no admin flag in context",
			setupCtx: func(r *http.Request) *http.Request {
				return r
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := RequireInstanceAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodDelete, "/admin/users/1", nil)
			req = tt.setupCtx(req)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %v, want %v", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestGetUserID(t *testing.T) {
	userID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")

	tests := []struct {
		name   string
		ctx    context.Context
		wantID uuid.UUID
		wantOK bool
	}{
		{
			name:   "user ID present",
			ctx:    context.WithValue(context.Background(), userIDKey, userID),
			wantID: userID,
			wantOK: true,
		},
		{
			name:   "user ID missing",
			ctx:    context.Background(),
			wantID: uuid.Nil,
			wantOK: false,
		},
		{
			name:   "wrong type in context",
			ctx:    context.WithValue(context.Background(), userIDKey, "not-a-uuid"),
			wantID: uuid.Nil,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := GetUserID(tt.ctx)
			if gotOK != tt.wantOK {
				t.Errorf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotID != tt.wantID {
				t.Errorf("userID = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}

func TestGetIsAdmin(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{
			name: "is admin",
			ctx:  context.WithValue(context.Background(), isAdminKey, true),
			want: true,
		},
		{
			name: "not admin",
			ctx:  context.WithValue(context.Background(), isAdminKey, false),
			want: false,
		},
		{
			name: "admin flag missing",
			ctx:  context.Background(),
			want: false,
		},
		{
			name: "wrong type in context",
			ctx:  context.WithValue(context.Background(), isAdminKey, "true"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetIsAdmin(tt.ctx)
			if got != tt.want {
				t.Errorf("GetIsAdmin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetClaims(t *testing.T) {
	claims := &auth.Claims{Admin: true}

	tests := []struct {
		name   string
		ctx    context.Context
		wantOK bool
	}{
		{
			name:   "claims present",
			ctx:    context.WithValue(context.Background(), claimsKey, claims),
			wantOK: true,
		},
		{
			name:   "claims missing",
			ctx:    context.Background(),
			wantOK: false,
		},
		{
			name:   "wrong type in context",
			ctx:    context.WithValue(context.Background(), claimsKey, "not-claims"),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOK := GetClaims(tt.ctx)
			if gotOK != tt.wantOK {
				t.Errorf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if tt.wantOK && got != claims {
				t.Error("claims should match")
			}
		})
	}
}

func TestOptionalAuth(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret", 15*time.Minute)
	userID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	validToken, _ := jwtManager.GenerateToken(userID, false, uuid.New())

	tests := []struct {
		name          string
		authHeader    string
		wantStatus    int
		wantUserInCtx bool
	}{
		{
			name:          "valid token",
			authHeader:    "Bearer " + validToken,
			wantStatus:    http.StatusOK,
			wantUserInCtx: true,
		},
		{
			name:          "no auth header - still succeeds",
			authHeader:    "",
			wantStatus:    http.StatusOK,
			wantUserInCtx: false,
		},
		{
			name:          "invalid format - still succeeds",
			authHeader:    "Invalid",
			wantStatus:    http.StatusOK,
			wantUserInCtx: false,
		},
		{
			name:          "invalid token - still succeeds",
			authHeader:    "Bearer invalid.token",
			wantStatus:    http.StatusOK,
			wantUserInCtx: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hadUser bool

			handler := OptionalAuth(jwtManager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, hadUser = GetUserID(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/public", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %v, want %v", rec.Code, tt.wantStatus)
			}

			if hadUser != tt.wantUserInCtx {
				t.Errorf("had user = %v, want %v", hadUser, tt.wantUserInCtx)
			}
		})
	}
}
