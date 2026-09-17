package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gemstore/internal/config"
	"gemstore/internal/database"
	"gemstore/internal/handler"
	"gemstore/internal/pkg/notifier"
	"gemstore/internal/repository"
	"gemstore/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("fatal startup error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// A 10s upper bound on startup DB connectivity — fail fast in
	// containers/CI rather than hang.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := database.Connect(ctx, cfg)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	// --- Wire the layers: repository -> service -> handler -----------------
	productRepo := repository.NewProductRepository(pool)
	orderRepo := repository.NewOrderRepository(pool)
	approvalRepo := repository.NewApprovalRepository(pool)
	userRepo := repository.NewUserRepository(pool)

	// LogNotifier writes to structured logs instead of a real provider —
	// swap this one line for an SMTP/Twilio-backed implementation once
	// credentials exist; nothing else in the wiring changes, since every
	// consumer only knows the notifier.NotificationService interface.
	notifierSvc := notifier.NewLogNotifier()

	productSvc := service.NewProductService(productRepo)
	pricingSvc := service.NewPricingService()
	orderSvc := service.NewOrderService(orderRepo, pricingSvc)
	approvalSvc := service.NewApprovalService(approvalRepo, notifierSvc)
	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTTokenTTL)

	productHandler := handler.NewProductHandler(productSvc)
	orderHandler := handler.NewOrderHandler(orderSvc)
	adminHandler := handler.NewAdminHandler(approvalSvc)
	authHandler := handler.NewAuthHandler(authSvc)
	pricingHandler := handler.NewPricingHandler(pricingSvc)

	router := handler.NewRouter(
		pool, productHandler, orderHandler, adminHandler, authHandler, pricingHandler,
		cfg.JWTSecret, cfg.AllowedOrigins,
	)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Run the server in a goroutine so the main goroutine is free to wait
	// on OS signals for graceful shutdown.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case sig := <-quit:
		slog.Info("shutdown signal received", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	slog.Info("server shut down cleanly")
	return nil
}
