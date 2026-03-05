package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/altlimit/dsorm"
	jwt "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"passedbox.com/model"
)

// AuthConfig holds auth state initialized at startup.
type AuthConfig struct {
	AdminHash []byte // bcrypt hash of admin password
	JWTKey    []byte // HMAC key for JWT signing
}

// InitAdmin bootstraps the admin password and JWT key on first run.
// If AdminPassword is empty, generates a random one and prints it.
// Accepts ADMIN_PASSWORD env var to override the stored password.
func InitAdmin(ctx context.Context, db *dsorm.Client) (*AuthConfig, error) {
	config, err := model.Config(ctx)
	isNew := false
	if err == datastore.ErrNoSuchEntity {
		isNew = true
	} else if err != nil {
		return nil, err
	}

	changed := false

	// Handle admin password
	if envPw := os.Getenv("ADMIN_PASSWORD"); envPw != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(envPw), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		config.AdminPassword = string(hash)
		changed = true
		slog.Info("Admin password set from ADMIN_PASSWORD env var")
	} else if config.AdminPassword == "" || isNew {
		pw := generateRandomString(24)
		hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		config.AdminPassword = string(hash)
		changed = true
		slog.Warn("Generated admin password", "password", pw)
		slog.Warn("Save this password! It will not be shown again.")
	}

	// Handle JWT signing key
	if config.JWTKey == "" || isNew {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("failed to generate JWT key: %w", err)
		}
		config.JWTKey = hex.EncodeToString(key)
		changed = true
	}

	if changed {
		if err := db.Put(ctx, config); err != nil {
			return nil, fmt.Errorf("failed to save server config: %w", err)
		}
	}

	jwtKey, err := hex.DecodeString(config.JWTKey)
	if err != nil {
		return nil, fmt.Errorf("invalid JWT key: %w", err)
	}

	return &AuthConfig{
		AdminHash: []byte(config.AdminPassword),
		JWTKey:    jwtKey,
	}, nil
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// CreateJWT creates a signed JWT with 24h expiry.
func (a *AuthConfig) CreateJWT() (string, error) {
	claims := jwt.MapClaims{
		"sub": "admin",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.JWTKey)
}

// ValidateJWT validates a JWT token string.
func (a *AuthConfig) ValidateJWT(tokenStr string) bool {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return a.JWTKey, nil
	})
	return err == nil && token.Valid
}

// CheckPassword verifies a plaintext password against the stored bcrypt hash.
func (a *AuthConfig) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword(a.AdminHash, []byte(password)) == nil
}

// AuthLevel determines how a route should be authenticated.
type AuthLevel int

const (
	// AuthAdmin requires admin JWT cookie or Bearer password (default).
	AuthAdmin AuthLevel = iota
	// AuthToken requires a vault token (Bearer header) or admin JWT.
	AuthToken
	// AuthNone skips authentication entirely (public endpoint).
	AuthNone
)

// pathPattern holds a parsed route pattern for matching request paths.
type pathPattern struct {
	// parts are the split segments, e.g. ["vaults", "{id}", "checkin"]
	parts []string
}

// AuthRoute maps a path pattern + methods to an auth level.
type AuthRoute struct {
	Path    string
	Methods []string
	Level   AuthLevel
}

// NewAuthMiddleware returns a middleware that enforces auth based on the
// request path. Routes are matched against authRoutes; unmatched routes
// default to AuthAdmin (full JWT / Bearer password).
func NewAuthMiddleware(auth *AuthConfig, db *dsorm.Client, authRoutes []AuthRoute) func(http.Handler) http.Handler {
	// Pre-parse patterns for fast matching.
	rules := make([]authRule, 0, len(authRoutes))
	for _, ar := range authRoutes {
		parts := splitPath(ar.Path)
		var methods map[string]bool
		if len(ar.Methods) > 0 {
			methods = make(map[string]bool, len(ar.Methods))
			for _, m := range ar.Methods {
				methods[m] = true
			}
		}
		rules = append(rules, authRule{
			pattern: pathPattern{parts: parts},
			methods: methods,
			level:   ar.Level,
		})
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			level := resolveAuthLevel(r, rules)

			switch level {
			case AuthNone:
				next.ServeHTTP(w, r)
				return

			case AuthToken:
				if checkAdminAuth(r, auth) {
					next.ServeHTTP(w, r)
					return
				}
				if db != nil && checkTokenAuth(r, db) {
					next.ServeHTTP(w, r)
					return
				}
				unauthorized(w)
				return

			default: // AuthAdmin
				if checkAdminAuth(r, auth) {
					next.ServeHTTP(w, r)
					return
				}
				unauthorized(w)
				return
			}
		})
	}
}

// authRule is a pre-compiled route rule used by resolveAuthLevel.
type authRule struct {
	pattern pathPattern
	methods map[string]bool
	level   AuthLevel
}

// resolveAuthLevel finds the auth level for a request by matching against rules.
func resolveAuthLevel(r *http.Request, rules []authRule) AuthLevel {
	// Strip the API prefix to get the route-relative path.
	// e.g. "/api/v1/vaults/abc123/checkin" -> "vaults/abc123/checkin"
	relPath := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	relPath = strings.TrimPrefix(relPath, "/")
	reqParts := splitPath(relPath)

	for _, rule := range rules {
		if rule.methods != nil && !rule.methods[r.Method] {
			continue
		}
		if matchParts(rule.pattern.parts, reqParts) {
			return rule.level
		}
	}
	return AuthAdmin
}

// matchParts checks if request path parts match a pattern.
// Pattern segments like "{id}" or "{id*}" act as wildcards.
func matchParts(pattern, path []string) bool {
	if len(pattern) != len(path) {
		// Check for wildcard suffix
		if len(pattern) > 0 && strings.HasSuffix(pattern[len(pattern)-1], "*}") {
			return len(path) >= len(pattern)-1
		}
		return false
	}
	for i, p := range pattern {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			continue // wildcard segment, matches anything
		}
		if p != path[i] {
			return false
		}
	}
	return true
}

// checkAdminAuth validates admin credentials via JWT cookie or Bearer password.
func checkAdminAuth(r *http.Request, auth *AuthConfig) bool {
	if auth == nil {
		return false
	}
	// 1. JWT session cookie
	if cookie, err := r.Cookie("session"); err == nil {
		if auth.ValidateJWT(cookie.Value) {
			return true
		}
	}
	// 2. Authorization: Bearer <password>
	if bearer := r.Header.Get("Authorization"); len(bearer) > 7 && bearer[:7] == "Bearer " {
		if auth.CheckPassword(bearer[7:]) {
			return true
		}
	}
	return false
}

// checkTokenAuth validates a vault token from the Authorization header.
func checkTokenAuth(r *http.Request, db *dsorm.Client) bool {
	bearer := r.Header.Get("Authorization")
	if len(bearer) <= 7 || bearer[:7] != "Bearer " {
		return false
	}
	token := bearer[7:]

	vaultID := extractVaultID(r.URL.Path)
	if vaultID == "" {
		return false
	}

	vault := &model.Vault{ID: vaultID}
	if err := db.Get(r.Context(), vault); err != nil || vault.Token != token {
		return false
	}
	return true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"Unauthorized"}`))
}

// extractVaultID extracts the vault ID from URL paths like /api/v1/vaults/{id} or /api/v1/vaults/{id}/...
func extractVaultID(path string) string {
	parts := splitPath(path)
	for i, p := range parts {
		if p == "vaults" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func splitPath(path string) []string {
	var parts []string
	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}
