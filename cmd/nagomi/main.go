package main

import (
	"context"
	"io"
	"net/http"
	api "nagomi-core/internal/api"
	"nagomi-core/internal/api/middleware"
	"nagomi-core/internal/config"
	"nagomi-core/internal/db"
	"nagomi-core/internal/service"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func main() {
	cfg := config.Load()

	// ----- logger -----------------
	var logger *log.Logger

	// create a log file only when using json
	// cause why would anyone point monitoring tools to a non json log file
	logWriter := io.Writer(os.Stdout)
	logFormatter := log.TextFormatter

	if cfg.LogFormat != "text" {
		logFormatter = log.JSONFormatter
	}

	logger = log.NewWithOptions(
		logWriter,
		log.Options{
			ReportTimestamp: true,
			Level:           cfg.LogLevel,
			Formatter:       logFormatter,
		},
	)

	// ----- migrations -------------
	logger.Info("running database migrations")
	if err := db.RunMigrations(cfg.DatabaseURL); err != nil {
		logger.Fatal("failed to run database migrations", "err", err)
	}
	logger.Info("database migrations completed successfully")

	// ----- database ---------------
	store, err := db.New(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("database connection failed", "err", err)
	}
	defer func() { _ = store.Close() }()
	logger.Info("database connection established")

	// ----- services ---------------
	services, err := service.New(store, logger, &cfg)
	if err != nil {
		logger.Fatal("failed to create services", "error", err)
	}
	logger.Info("services initialized")

	// ----- receipt OCR worker ----
	go services.Receipts.StartWorker(context.Background())
	go services.Statements.StartCleanup(context.Background())

	// ----- api layer --------
	srv := api.NewServer(services, logger.WithPrefix("api"))
	authConfig := &middleware.AuthConfig{
		InternalAPIKey: cfg.APIKey,
		WebURL:         cfg.NagomiGatewayURL,
	}

	handler := srv.GetHandler(authConfig)

	serverErrors := make(chan error, 1)

	go func() {
		server := &http.Server{
			Addr:    cfg.ListenAddress,
			Handler: h2c.NewHandler(handler, &http2.Server{}),
		}

		logger.Info("server is listening", "addr", cfg.ListenAddress)
		serverErrors <- server.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Fatal("server error", "err", err)

	case <-quit:
		logger.Info("shutdown signal received")
		logger.Info("server stopping...")
	}

	logger.Info("server shutdown complete")
}
