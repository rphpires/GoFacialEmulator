package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"GoFacialEmulator/assets"
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

	tracer.Info("Applying schema migrations...")
	migFS, err := assets.MigrationFiles()
	if err != nil {
		log.Fatalf("Failed to open embedded migrations: %v", err)
	}
	migCtx, migCancel := context.WithTimeout(context.Background(), 60*time.Second)
	err = database.ApplyMigrations(migCtx, serviceDB, migFS)
	migCancel()
	if err != nil {
		// Banco meio migrado é pior que serviço que não sobe.
		log.Fatalf("Failed to apply migrations: %v", err)
	}
	tracer.Info("Schema migrations up to date")

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

		if cfg.WxsDB.Host == "" {
			// WXS não configurado: isso não é uma falha, é o estado esperado
			// de uma instalação nova antes de o cliente configurar em /settings.
			// wxsDB permanece nil, o que faz o handler pular o registro do
			// health checker de wxs_db (ver handlers.NewHandler).
			tracer.Info("WXS not configured (no host in config.yaml) — service will run without WxsDB connection")
		} else {
			// Usar configurações do config.yaml como fallback
			wxsDB, err = database.GetWxsDB(cfg.WxsDB)
			if err != nil {
				tracer.Warning("Failed to connect to WxsDB using config.yaml: %v", err)
				tracer.Info("Service will continue without WxsDB connection")
				wxsDB = nil
			} else {
				tracer.Info("WxsDB connected successfully using config.yaml")
			}
		}
	} else if wxsSettings.Host == "" {
		// Mesma ideia, mas a fonte da configuração foi o banco (tela /settings
		// gravou um registro, porém sem host preenchido).
		tracer.Info("WXS not configured (no host in database settings) — service will run without WxsDB connection")
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

	// Testar WxsDB em background: ele é opcional e pode estar inacessível.
	// Não deve bloquear a subida do servidor HTTP.
	if wxsDB != nil {
		go func() {
			pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer pingCancel()
			if err := wxsDB.Ping(pingCtx); err != nil {
				tracer.Warning("WxsDB ping failed (WXS may be temporarily unavailable): %v", err)
			} else {
				tracer.Info("WxsDB connection successful")
			}
		}()
	}

	// Inicializar manager de emuladores
	manager := emulator.NewManager(serviceDB, emulatorDB, wxsDB, tracer)
	manager.ServicePort = cfg.Server.Port
	if err := manager.Initialize(); err != nil {
		log.Fatalf("Failed to initialize emulator manager: %v", err)
	}

	// Atualizar dispositivos do WXS em background — não pode bloquear a
	// subida do HTTP server caso o WXS esteja inacessível.
	go func() {
		if err := manager.RefreshDevices(); err != nil {
			if errors.Is(err, emulator.ErrSyncDisabled) {
				// Sync desligado é o estado normal de uma instalação
				// manual-only: dispara em todo boot que não sincroniza, e
				// não é falha nenhuma — logar como Error faria o operador
				// caçar um problema que não existe.
				tracer.Info("Sincronização com o W-Access desligada — nenhum dispositivo será importado")
			} else {
				tracer.Error("Failed to refresh devices: %v", err)
			}
		}
	}()

	// Inicializar handlers HTTP
	handler := handlers.NewHandler(manager, serviceDB, wxsDB, cfg.AppVersion, tracer)

	// Configurar servidor HTTP
	server := &http.Server{
		Addr:        fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:     handler.Router(),
		ReadTimeout: 30 * time.Second,
		// 5 minutos, não 30 segundos: precisa acomodar o orçamento de
		// contexto do handler mais lento do serviço,
		// apiCreateEmulatorRange (internal/handlers/emulators.go), que abre
		// um context.WithTimeout de 5 minutos para um lote grande com
		// auto_start — centenas de emuladores, cada Start com até 10s
		// serializado. Os dois valores se movem juntos: mudar um sem o
		// outro corta a conexão antes do handler terminar, ou deixa o
		// handler abortar antes do servidor.
		WriteTimeout: 5 * time.Minute,
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
