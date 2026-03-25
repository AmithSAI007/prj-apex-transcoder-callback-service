package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/config"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/handler"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/platform"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/repository"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/service"
	"go.uber.org/zap"
)

func main() {

	// Load application configuration from environment variables and .env files.
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize the structured logger (JSON in production, console in development).
	// The service name is added as a base field so logs from this service can be
	// identified when routed to a shared audit sink (e.g., BigQuery).
	logger, err := config.NewLogger(cfg.AppEnv, cfg.OTEL_SERVICE_NAME)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("Application starting",
		zap.String("component", "main"),
		zap.String("action", "startup"),
		zap.String("environment", cfg.AppEnv),
		zap.String("projectId", cfg.GCPProjectID),
		zap.String("serviceName", cfg.OTEL_SERVICE_NAME))

	// Create a root context for the application lifecycle.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up the OpenTelemetry tracing pipeline (OTLP exporter, sampler, propagators).
	otelShutdown, err := platform.InitTracer(cfg, ctx)
	if err != nil {
		logger.Fatal("Failed to initialize tracer", zap.Error(err))
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			logger.Error("Failed to shutdown tracer", zap.Error(err))
		}
	}()

	firestoreClient, err := platform.NewClient(ctx, cfg.GCPProjectID, cfg.FirestoreDatabaseID)
	if err != nil {
		logger.Fatal("Failed to initialize Firestore client", zap.Error(err))
	}
	defer func() { _ = firestoreClient.Close() }()

	subscriber, err := platform.NewSubscriber(ctx, logger, cfg.GCPProjectID, cfg.PubSubSubscriptionID, cfg.MaxOutstandingMessages)
	if err != nil {
		logger.Fatal("Failed to initialize Pub/Sub subscriber", zap.Error(err))
	}
	defer func() { _ = subscriber.Close() }()

	firestoreRepo := repository.NewFirestoreRepo(firestoreClient.Client(), cfg.FirestoreCollectionName, logger, cfg.GCPProjectID)
	callbackService := service.NewCallbackService(cfg, firestoreRepo, nil, logger)
	eventHandler := handler.NewEventHandler(logger, callbackService, cfg.GCPProjectID)

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	healthServer := &http.Server{Addr: cfg.HttpPort, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	wg.Go(func() {
		logger.Info("Starting health check server", zap.String("port", cfg.HttpPort))
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("Health check server failed", zap.Error(err))
		}
	})

	wg.Go(func() {
		if err := subscriber.Start(ctx, eventHandler.Handle); err != nil {
			logger.Fatal("Pub/Sub subscriber failed", zap.Error(err))
		}
	})

	sig := <-sigch
	logger.Info("Received shutdown signal",
		zap.String("component", "main"),
		zap.String("action", "shutdown"),
		zap.String("signal", sig.String()))

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = healthServer.Shutdown(shutdownCtx)

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		logger.Info("Shutdown complete",
			zap.String("component", "main"),
			zap.String("action", "shutdown_complete"))
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timed out, forcing exit",
			zap.String("component", "main"),
			zap.String("action", "shutdown_timeout"))
	}
}
