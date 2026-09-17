package handler

import (
	"encoding/json"
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
// account (see AuthService.SignUp) and returns a ready-to-use token so
// the frontend doesn't need a separate login call right after signup.
func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	var req models.SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}

	result, err := h.svc.SignUp(r.Context(), req.Email, req.Password, req.FullName, req.Phone)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, models.AuthResponse{
		Token: result.Token,
		User:  result.User,
	})
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}

	result, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, models.AuthResponse{
		Token: result.Token,
		User:  result.User,
	})
}

// Me handles GET /api/v1/auth/me. Mostly useful as a fast way to check
// "does my token actually work" from the frontend (or curl) without
// hitting a business endpoint to find out.
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

func writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid service.ErrInvalidInput

	switch {
	case errors.As(err, &invalid):
		httputil.WriteError(w, http.StatusBadRequest, invalid.Error())
	case errors.Is(err, service.ErrEmailTaken):
		httputil.WriteError(w, http.StatusConflict, "email is already registered")
	case errors.Is(err, service.ErrInvalidCredentials):
		httputil.WriteError(w, http.StatusUnauthorized, "invalid email or password")
	default:
		slog.ErrorContext(r.Context(), "unhandled auth error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
	}
}
