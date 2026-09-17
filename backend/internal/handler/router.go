package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "gemstore/internal/middleware"

	"gemstore/internal/database"
	"gemstore/internal/httputil"
	"gemstore/internal/models"
)

// NewRouter builds the full route tree. Handlers are passed in already
// constructed (see cmd/api/main.go) so this function has no knowledge of
// how they're wired to the database — it only knows HTTP and authorization.
//
// jwtSecret is threaded through to appmiddleware.Authenticate here rather
// than each handler reaching for it independently, keeping "what secret
// signs tokens" a router-level concern.
func NewRouter(
	pool *pgxpool.Pool,
	productHandler *ProductHandler,
	orderHandler *OrderHandler,
	adminHandler *AdminHandler,
	authHandler *AuthHandler,
	pricingHandler *PricingHandler,
	jwtSecret []byte,
	allowedOrigins []string,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", healthHandler(pool))

	authenticate := appmiddleware.Authenticate(jwtSecret)
	requireAdmin := appmiddleware.RequireRole(models.RoleAdmin)

	r.Route("/api/v1", func(r chi.Router) {
		// --- Public: no login required --------------------------------------
		r.Route("/products", func(r chi.Router) {
			r.Get("/", productHandler.List)
			r.Get("/{id}", productHandler.Get)
		})

		r.Get("/pricing/catalog", pricingHandler.Catalog)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/signup", authHandler.SignUp)
			r.Post("/login", authHandler.Login)

			r.Group(func(r chi.Router) {
				r.Use(authenticate)
				r.Get("/me", authHandler.Me)
			})
		})

		// --- Authenticated customer routes ---------------------------------
		r.Route("/orders", func(r chi.Router) {
			r.Use(authenticate)
			r.Post("/custom", orderHandler.SubmitCustom)
			r.Get("/{id}", orderHandler.Get)
		})

		// --- Authenticated + admin-only routes ------------------------------
		r.Route("/admin", func(r chi.Router) {
			r.Use(authenticate)
			r.Use(requireAdmin)

			r.Route("/orders", func(r chi.Router) {
				r.Get("/pending", adminHandler.ListPending)
				r.Post("/{id}/approve", adminHandler.Approve)
				r.Post("/{id}/decline", adminHandler.Decline)
			})
		})
	})

	return r
}

func healthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := database.HealthCheck(r.Context(), pool); err != nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "database unreachable")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
