package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"GoFacialEmulator/internal/config"
	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/emulator"
	"GoFacialEmulator/internal/handlers"
	"GoFacialEmulator/internal/trace"
)

func main() {
	// Parse command line flags
	configFile := flag.String("config", "config.yml", "Path to configuration file")
	flag.Parse()

	// Initialize tracer
	tracer := trace.NewTracer()
	defer tracer.Close()

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		tracer.Error("Failed to load configuration: %v", err)
		os.Exit(1)
	}

	// Initialize database connections
	serviceDB, err := database.NewServiceDB(cfg.ServiceDB)
	if err != nil {
		tracer.Error("Failed to connect to service database: %v", err)
		os.Exit(1)
	}
	defer serviceDB.Close()

	emulatorDB, err := database.NewEmulatorDB(cfg.EmulatorDB)
	if err != nil {
		tracer.Error("Failed to connect to emulator database: %v", err)
		os.Exit(1)
	}
	defer emulatorDB.Close()

	wxsDB, err := database.NewWxsDB(cfg.WxsDB)
	if err != nil {
		tracer.Error("Failed to connect to WXS database: %v", err)
		os.Exit(1)
	}
	defer wxsDB.Close()

	// Create emulator manager
	manager := emulator.NewManager(serviceDB, emulatorDB, wxsDB, tracer)

	// Create HTTP handlers
	handler := handlers.NewHandler(manager, serviceDB, wxsDB, tracer)

	// Set up HTTP server
	server := &http.Server{
		Addr:    cfg.Server.Address,
		Handler: handler.Router(),
	}

	// Start the HTTP server in a separate goroutine
	go func() {
		tracer.Info("Starting server on %s", cfg.Server.Address)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			tracer.Error("Failed to start server: %v", err)
			os.Exit(1)
		}
	}()

	func gracefulShutdown(server *http.Server, manager *emulator.Manager, tracer *trace.Tracer) {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

		<-stop
		tracer.Info("Shutdown signal received")

		// Create shutdown context with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Shutdown HTTP server
		if err := server.Shutdown(shutdownCtx); err != nil {
			tracer.Error("HTTP server shutdown failed: %v", err)
		}

		// Stop all emulators
		manager.StopAll()
		
		tracer.Info("Application stopped gracefully")
	}

	// Set up signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Wait for interrupt signal
	<-stop

	tracer.Info("Shutting down server...")

	// Create a context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shut down the HTTP server
	if err := server.Shutdown(ctx); err != nil {
		tracer.Error("Server shutdown failed: %v", err)
	}

	// Stop all running emulators
	manager.StopAll()

	tracer.Info("Server stopped")
}
