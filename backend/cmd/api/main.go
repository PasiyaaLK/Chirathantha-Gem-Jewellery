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
	refreshTokenRepo := repository.NewRefreshTokenRepository(pool)

	// LogNotifier writes to structured logs instead of a real provider —
	// buildNotifier below swaps in SendGrid/Twilio automatically once
	// their credentials are configured; nothing else in the wiring
	// changes either way, since every consumer only knows the
	// notifier.NotificationService interface.
	notifierSvc := buildNotifier(cfg)

	productSvc := service.NewProductService(productRepo)
	pricingSvc := service.NewPricingService()
	orderSvc := service.NewOrderService(orderRepo, pricingSvc)
	approvalSvc := service.NewApprovalService(approvalRepo, notifierSvc)
	authSvc := service.NewAuthService(
		userRepo, refreshTokenRepo, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL,
	)

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

// buildNotifier picks a real provider for each channel (email, SMS)
// independently when its credentials are configured, and falls back to
// LogNotifier for whichever channel isn't — so the app degrades
// gracefully (logs instead of sending) rather than failing to start
// just because, say, Twilio isn't set up yet in a given environment.
//
// *LogNotifier satisfies both the email-only and SMS-only interfaces
// CompositeNotifier wants, since it implements both methods — no
// adapter needed to use it for either side.
func buildNotifier(cfg *config.Config) notifier.NotificationService {
	logNotifier := notifier.NewLogNotifier()
	composite := &notifier.CompositeNotifier{Email: logNotifier, SMS: logNotifier}

	if cfg.SendGridAPIKey != "" {
		composite.Email = notifier.NewSendGridEmailNotifier(
			cfg.SendGridAPIKey, cfg.EmailFromAddress, cfg.EmailFromName,
		)
		slog.Info("notifier: sending email via SendGrid", "from", cfg.EmailFromAddress)
	} else {
		slog.Warn("notifier: SENDGRID_API_KEY not set — emails will be logged, not sent")
	}

	if cfg.TwilioAccountSID != "" && cfg.TwilioAuthToken != "" && cfg.TwilioFromNumber != "" {
		composite.SMS = notifier.NewTwilioSMSNotifier(
			cfg.TwilioAccountSID, cfg.TwilioAuthToken, cfg.TwilioFromNumber,
		)
		slog.Info("notifier: sending SMS via Twilio", "from", cfg.TwilioFromNumber)
	} else {
		slog.Warn("notifier: Twilio credentials not fully set — SMS will be logged, not sent")
	}

	return composite
}
