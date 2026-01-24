package main

import (
	"context"
	"flag"
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
	// Flag para arquivo de configuração
	configPath := flag.String("config", "configs/config.yaml", "Path to config file")
	flag.Parse()

	tracer := trace.NewTracer()
	tracer.Info("Starting Facial Emulator Service")
	tracer.Info("Using config file: %s", *configPath)

	cfg, err := config.Load(*configPath)
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

	// Carregar configurações WXS do banco de dados
	tracer.Info("Loading WXS configuration from database...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wxsSettings, err := database.GetWxsSettingsFromDB(ctx, serviceDB)
	var wxsDB *database.WxsDB

	if err != nil {
		tracer.Warning("Failed to load WXS settings from database: %v", err)
		tracer.Info("Falling back to config.yaml for WXS settings")

		// Usar configurações do config.yaml como fallback
		wxsDB, err = database.GetWxsDB(cfg.WxsDB)
		if err != nil {
			tracer.Warning("Failed to connect to WxsDB using config.yaml: %v", err)
			tracer.Info("Service will continue without WxsDB connection")
			wxsDB = nil
		} else {
			tracer.Info("WxsDB connected successfully using config.yaml")
		}
	} else {
		// Usar configurações do banco de dados
		tracer.Info("Using WXS settings from database: %s:%d/%s", wxsSettings.Host, wxsSettings.Port, wxsSettings.Database)

		wxsCfg := config.DatabaseConfig{
			Host:     wxsSettings.Host,
			Port:     wxsSettings.Port,
			Database: wxsSettings.Database,
			Username: wxsSettings.Username,
			Password: wxsSettings.Password,
			Driver:   "mssql",
		}

		wxsDB, err = database.NewWxsDB(wxsCfg)
		if err != nil {
			tracer.Warning("Failed to connect to WxsDB using database settings: %v", err)
			tracer.Info("Service will continue without WxsDB connection")
			wxsDB = nil
		} else {
			tracer.Info("WxsDB connected successfully using database settings")
		}
	}

	// Testar conexões
	tracer.Info("Testing database connections...")

	if err := serviceDB.Ping(ctx); err != nil {
		log.Fatalf("ServiceDB connection test failed: %v", err)
	}

	if err := emulatorDB.Ping(ctx); err != nil {
		log.Fatalf("EmulatorDB connection test failed: %v", err)
	}

	// Testar WxsDB apenas se conectou
	if wxsDB != nil {
		if err := wxsDB.Ping(ctx); err != nil {
			tracer.Warning("WxsDB ping failed (WXS may be temporarily unavailable): %v", err)
		} else {
			tracer.Info("All database connections successful")
		}
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
