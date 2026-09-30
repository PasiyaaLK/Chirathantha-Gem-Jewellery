// Package middleware holds HTTP middleware shared across route groups:
// authentication/RBAC (this file), rate limiting and body-size limits
// (security.go), response security headers (headers.go), and panic
// recovery (recovery.go). Request ID/RealIP/logging/CORS are chi's own
// middleware, wired in handler.Router.
package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"gemstore/internal/httputil"
	"gemstore/internal/models"
)

// Cookie names shared between this package (which reads
// AccessTokenCookieName in Authenticate) and internal/handler/auth_handler.go
// (which sets/clears both). Exported and centralized here rather than
// duplicated as string literals in two files, which is exactly how a
// typo causes "logout doesn't actually log anyone out" bugs.
const (
	AccessTokenCookieName  = "access_token"
	RefreshTokenCookieName = "refresh_token"
)

type contextKey int

const (
	ctxKeyUserID contextKey = iota
	ctxKeyRole
)

// Claims is the JWT payload issued to authenticated users. UserID and
// Role are the only custom fields the rest of the app relies on;
// RegisteredClaims supplies standard exp/iat handling.
type Claims struct {
	UserID uuid.UUID       `json:"user_id"`
	Role   models.UserRole `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken issues a signed access-token JWT for userID/role. Used
// by AuthService on login/signup/refresh, and by cmd/gentoken as a dev
// shortcut for minting a token without going through a real login.
func GenerateToken(secret []byte, userID uuid.UUID, role models.UserRole, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("middleware: sign token: %w", err)
	}
	return signed, nil
}

// Authenticate validates the caller's access token against secret and,
// on success, stores their user ID and role in the request context
// (read back with UserIDFromContext / RoleFromContext).
//
// The token is read from the access_token HttpOnly cookie first (the
// web frontend's path, set by auth_handler.go on login/signup/refresh),
// falling back to an "Authorization: Bearer <token>" header if no
// cookie is present — kept for non-browser API clients (scripts, a
// future mobile app) and for cmd/gentoken-minted test tokens, which
// have no way to arrive as a cookie. Both paths converge on the same
// validation and the same context values; nothing downstream needs to
// know or care which one was used.
func Authenticate(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString, ok := accessToken(r)
			if !ok {
				httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
				return
			}

			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
				// Reject anything but HMAC — without this check, a token
				// signed with "alg": "none" (or crafted for a different
				// algorithm family) could otherwise slip past validation.
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return secret, nil
			})

			switch {
			case errors.Is(err, jwt.ErrTokenExpired):
				httputil.WriteError(w, http.StatusUnauthorized, "session expired")
				return
			case err != nil || !token.Valid:
				httputil.WriteError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ctxKeyRole, claims.Role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole gates a route to callers whose token role claim is one of
// roles. It must sit after Authenticate in the middleware chain — it
// trusts that Authenticate already populated the context, and 500s
// (rather than 401/403) if that context is missing, since that signals a
// routing bug, not a client auth failure.
func RequireRole(roles ...models.UserRole) func(http.Handler) http.Handler {
	allowed := make(map[models.UserRole]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := RoleFromContext(r.Context())
			if !ok {
				httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			if !allowed[role] {
				httputil.WriteError(w, http.StatusForbidden, "insufficient permissions for this action")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserIDFromContext reads the authenticated caller's ID, set by
// Authenticate. The bool is false if called on a request that never
// passed through Authenticate.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxKeyUserID).(uuid.UUID)
	return id, ok
}

// RoleFromContext reads the authenticated caller's role, set by Authenticate.
func RoleFromContext(ctx context.Context) (models.UserRole, bool) {
	role, ok := ctx.Value(ctxKeyRole).(models.UserRole)
	return role, ok
}

// accessToken finds the access token from the cookie or, failing that,
// the Authorization header. See Authenticate's doc comment for why both
// exist.
func accessToken(r *http.Request) (string, bool) {
	if cookie, err := r.Cookie(AccessTokenCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, true
	}

	raw := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(raw, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		return "", false
	}
	return token, true
}
