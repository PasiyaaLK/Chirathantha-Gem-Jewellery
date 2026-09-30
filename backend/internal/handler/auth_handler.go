package handler

import (
	"errors"
	"log/slog"
	"net/http"

	appmiddleware "gemstore/internal/middleware"

	"gemstore/internal/httputil"
	"gemstore/internal/models"
	"gemstore/internal/service"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// SignUp handles POST /api/v1/auth/signup — always creates a 'customer'
// account (see AuthService.SignUp) and sets the access/refresh cookie
// pair so the frontend doesn't need a separate login call right after
// signup.
func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	var req models.SignUpRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	result, err := h.svc.SignUp(r.Context(), req.Email, req.Password, req.FullName, req.Phone)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	setAuthCookies(w, result)
	httputil.WriteJSON(w, http.StatusCreated, models.AuthResponse{User: result.User})
}

// Login handles POST /api/v1/auth/login. Rate-limited to 5/min/IP — see
// handler.NewRouter.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	result, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	setAuthCookies(w, result)
	httputil.WriteJSON(w, http.StatusOK, models.AuthResponse{User: result.User})
}

// Refresh handles POST /api/v1/auth/refresh — exchanges the
// refresh_token cookie for a new access/refresh pair. No request body;
// everything it needs comes from the cookie. See
// AuthService.RefreshAccessToken for the rotation/reuse-detection logic.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(appmiddleware.RefreshTokenCookieName)
	if err != nil || cookie.Value == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	result, err := h.svc.RefreshAccessToken(r.Context(), cookie.Value)
	if err != nil {
		// Whatever went wrong, the presented refresh token is no longer
		// good for anything — clear both cookies so the frontend's next
		// /auth/me call cleanly reports "logged out" instead of retrying
		// a refresh that will only fail the same way again.
		clearAuthCookies(w)
		writeAuthError(w, r, err)
		return
	}

	setAuthCookies(w, result)
	httputil.WriteJSON(w, http.StatusOK, models.AuthResponse{User: result.User})
}

// Logout handles POST /api/v1/auth/logout — revokes the refresh token
// server-side (so it can't be replayed even if captured) and clears
// both cookies. Safe to call with no session at all (no-op).
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(appmiddleware.RefreshTokenCookieName); err == nil {
		if revokeErr := h.svc.Logout(r.Context(), cookie.Value); revokeErr != nil {
			// Still clear cookies below even though the DB revoke
			// failed — the client shouldn't stay stuck "logged in" from
			// its own point of view just because one write hiccuped;
			// worst case the old refresh token remains valid server-side
			// until it naturally expires.
			slog.ErrorContext(r.Context(), "auth: logout revoke failed", "error", revokeErr)
		}
	}

	clearAuthCookies(w)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// Me handles GET /api/v1/auth/me. Mostly useful as a fast way to check
// "am I logged in, and as whom" from the frontend without hitting a
// business endpoint to find out — and, since cookies are invisible to
// JS by design, it's the ONLY way the frontend can know: see
// lib/useAuth.ts's useCurrentUser, which always calls this rather than
// checking for a token client-side the way it used to.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := appmiddleware.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	user, err := h.svc.GetProfile(r.Context(), userID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, user)
}

// setAuthCookies puts both tokens from result into HttpOnly, Secure,
// SameSite=Strict cookies.
//
// Secure is unconditionally true, per spec — this does NOT break local
// http://localhost development: modern browsers (Chrome, Firefox, and
// others) treat "localhost" as a potentially-trustworthy origin and
// still accept Secure cookies over plain HTTP specifically for it, as a
// documented exception. Any other non-HTTPS origin would silently drop
// these cookies, which is the correct, intended behavior in production.
//
// SameSite=Strict means these cookies are sent only on same-SITE
// requests (scheme + registrable domain — port doesn't count), which is
// what makes this a strong CSRF defense on its own: a cross-site
// request from an attacker's page never carries them at all. It also
// means this ONLY works when the frontend and backend share a
// registrable domain (e.g. app.yourstore.com calling api.yourstore.com,
// or this project's localhost:3000 calling localhost:8080 — different
// ports, same site). If you ever deploy the frontend and backend on
// genuinely different domains (a Vercel app calling a Railway API, say),
// SameSite=Strict will silently block these cookies on every
// cross-origin call and you'll need SameSite=None; Secure instead —
// which reopens the CSRF surface Strict closes, so that switch should
// come with an explicit CSRF-token scheme, not by itself.
func setAuthCookies(w http.ResponseWriter, result *service.AuthResult) {
	http.SetCookie(w, &http.Cookie{
		Name:     appmiddleware.AccessTokenCookieName,
		Value:    result.AccessToken,
		Path:     "/",
		MaxAge:   int(result.AccessTokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:  appmiddleware.RefreshTokenCookieName,
		Value: result.RefreshToken,
		// Scoped narrowly: only the refresh/logout endpoints ever need
		// this cookie, so the browser never sends it anywhere else —
		// one less place a bug elsewhere in the API could leak it.
		Path:     "/api/v1/auth",
		MaxAge:   int(result.RefreshTokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearAuthCookies expires both cookies immediately (MaxAge: -1). Used
// by Logout and by Refresh's failure path.
func clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: appmiddleware.AccessTokenCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: appmiddleware.RefreshTokenCookieName, Value: "", Path: "/api/v1/auth", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

func writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid service.ErrInvalidInput

	switch {
	case errors.As(err, &invalid):
		httputil.WriteError(w, http.StatusBadRequest, invalid.Error())
	case errors.Is(err, service.ErrEmailTaken):
		httputil.WriteError(w, http.StatusConflict, "email is already registered")
	case errors.Is(err, service.ErrInvalidCredentials):
		httputil.WriteError(w, http.StatusUnauthorized, "invalid email or password")
	case errors.Is(err, service.ErrInvalidRefreshToken):
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
	default:
		slog.ErrorContext(r.Context(), "unhandled auth error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
	}
}
