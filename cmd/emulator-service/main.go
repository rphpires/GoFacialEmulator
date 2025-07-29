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
	// Inicializar tracer
	tracer := trace.NewTracer()
	tracer.Info("Starting Facial Emulator Service")

	// Carregar configuração
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Inicializar bancos de dados
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Obter instâncias dos bancos
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

	// Inicializar manager de emuladores
	manager := emulator.NewManager(serviceDB, emulatorDB, wxsDB, tracer)
	if err := manager.Initialize(); err != nil {
		log.Fatalf("Failed to initialize emulator manager: %v", err)
	}

	// Atualizar dispositivos do WXS
	if err := manager.RefreshDevices(); err != nil {
		tracer.Error("Failed to refresh devices: %v", err)
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
