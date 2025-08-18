package main

import (
	"context"
	"fmt"
	"log"
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
	tracer := trace.NewTracer()
	tracer.Info("Starting Facial Emulator Service")

	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	tracer.Info("Validating database structure...")
	if err := database.ValidateDatabaseOnStartup(cfg.ServiceDB); err != nil {
		log.Fatalf("Failed to validate/recreate database: %v", err)
	}
	tracer.Info("Database validation completed successfully")

	tracer.Info("Initializing database connections...")
	serviceDB, err := database.GetServiceDB(cfg.ServiceDB)
	if err != nil {
		log.Fatalf("Failed to get ServiceDB: %v", err)
	}

	emulatorDB, err := database.GetEmulatorDB(cfg.EmulatorDB)
	if err != nil {
		log.Fatalf("Failed to get EmulatorDB: %v", err)
	}

	wxsDB, err := database.GetWxsDB(cfg.WxsDB)
	if err != nil {
		log.Fatalf("Failed to get WxsDB: %v", err)
	}

	// Testar conexões
	tracer.Info("Testing database connections...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := serviceDB.Ping(ctx); err != nil {
		log.Fatalf("ServiceDB connection test failed: %v", err)
	}

	if err := emulatorDB.Ping(ctx); err != nil {
		log.Fatalf("EmulatorDB connection test failed: %v", err)
	}

	if err := wxsDB.Ping(ctx); err != nil {
		tracer.Warning("WxsDB connection test failed (WXS may be unavailable): %v", err)
		// Não falhar aqui pois WXS pode estar indisponível temporariamente
	} else {
		tracer.Info("All database connections successful")
	}

	// Inicializar manager de emuladores
	manager := emulator.NewManager(serviceDB, emulatorDB, wxsDB, tracer)
	if err := manager.Initialize(); err != nil {
		log.Fatalf("Failed to initialize emulator manager: %v", err)
	}

	// Atualizar dispositivos do WXS (se disponível)
	if err := manager.RefreshDevices(); err != nil {
		tracer.Error("Failed to refresh devices: %v", err)
		// Continuar mesmo se WXS não estiver disponível
	}

	// Inicializar handlers HTTP
	handler := handlers.NewHandler(manager, serviceDB, wxsDB, tracer)

	// Configurar servidor HTTP
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      handler.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Iniciar servidor em goroutine
	go func() {
		tracer.Info("Starting HTTP server on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Configurar graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	tracer.Info("Shutting down server...")

	// Shutdown graceful
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parar servidor HTTP
	if err := server.Shutdown(ctx); err != nil {
		tracer.Error("Server forced to shutdown: %v", err)
	}

	// Parar manager de emuladores
	manager.Shutdown()

	// Fechar conexões de banco
	database.CloseAll()

	tracer.Info("Server exited")
}
