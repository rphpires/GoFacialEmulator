package emulator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"
	"GoFacialEmulator/internal/utils"
)

// Emulator define a interface que todos os emuladores devem implementar
type Emulator interface {
	Start() error
	Stop() error
	IsRunning() bool
	GetInfo() models.Device
	GenerateEvent() error
	GetType() string
}

// EventGenerator define a interface para geradores de eventos
type EventGenerator interface {
	GenerateEvent() error
	SendEventToRemoteServer(event interface{}, contentType string) error
}

// ConfigManager define a interface para gerenciamento de configurações
type ConfigManager interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

// BaseEmulator contém funcionalidade comum para todos os emuladores
type BaseEmulator struct {
	mu         sync.RWMutex
	Device     models.Device
	DB         *database.EmulatorDB
	Tracer     *trace.Tracer
	Server     *http.Server
	running    bool
	stopChan   chan struct{}
	MacAddress string
	RemoteURL  string

	// Canais para controle de lifecycle
	eventTicker *time.Ticker
	eventDone   chan struct{}
}

// NewBaseEmulator cria um novo emulador base
func NewBaseEmulator(db *database.EmulatorDB, device models.Device, tracer *trace.Tracer) *BaseEmulator {
	macAddress := utils.GenerateMacAddress()

	return &BaseEmulator{
		Device:     device,
		DB:         db,
		Tracer:     tracer,
		running:    false,
		MacAddress: macAddress,
		RemoteURL:  fmt.Sprintf("http://%s:%d/notification", device.IPAddress, device.Port),
		eventDone:  make(chan struct{}),
	}
}

// Start inicia o emulador
func (e *BaseEmulator) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return fmt.Errorf("emulator already running")
	}

	e.Tracer.Info("Starting emulator: %s", e.Device.Name)
	e.stopChan = make(chan struct{})
	e.running = true

	return nil
}

// Stop para o emulador
func (e *BaseEmulator) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return fmt.Errorf("emulator not running")
	}

	e.Tracer.Info("Stopping emulator: %s", e.Device.Name)

	// Parar o gerador de eventos
	e.stopEventGenerator()

	// Fechar canal de parada
	close(e.stopChan)
	e.running = false

	// Parar servidor HTTP se estiver rodando
	if e.Server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return e.Server.Shutdown(ctx)
	}

	return nil
}

// IsRunning retorna true se o emulador estiver rodando
func (e *BaseEmulator) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// GetInfo retorna informações sobre o emulador
func (e *BaseEmulator) GetInfo() models.Device {
	e.mu.RLock()
	defer e.mu.RUnlock()

	device := e.Device
	if e.running {
		device.Status = "running"
	} else {
		device.Status = "stopped"
	}

	// Atualizar contagem de usuários
	if count, err := e.GetTotalUsers(); err == nil {
		device.TotalUsers = count
	}

	return device
}

// GetType retorna o tipo do emulador (deve ser sobrescrito pelos emuladores específicos)
func (e *BaseEmulator) GetType() string {
	return "base"
}

// StartEventGenerator inicia o gerador de eventos
func (e *BaseEmulator) StartEventGenerator(generator func() error) {
	if e.Device.EventInterval <= 0 {
		e.Tracer.Info("Event generation disabled (interval <= 0)")
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Parar gerador anterior se existir
	e.stopEventGenerator()

	// Criar novo ticker
	e.eventTicker = time.NewTicker(time.Duration(e.Device.EventInterval) * time.Second)
	e.eventDone = make(chan struct{})

	go func() {
		defer e.eventTicker.Stop()

		for {
			select {
			case <-e.eventTicker.C:
				if e.IsRunning() {
					if err := generator(); err != nil {
						e.Tracer.Error("Failed to generate event: %v", err)
					}
				}
			case <-e.eventDone:
				return
			case <-e.stopChan:
				return
			}
		}
	}()

	e.Tracer.Info("Event generator started with interval: %ds", e.Device.EventInterval)
}

// stopEventGenerator para o gerador de eventos
func (e *BaseEmulator) stopEventGenerator() {
	if e.eventTicker != nil {
		e.eventTicker.Stop()
		e.eventTicker = nil
	}

	if e.eventDone != nil {
		close(e.eventDone)
		e.eventDone = nil
	}
}

// Métodos de conveniência para acesso ao banco de dados

// GetTotalUsers retorna o número total de usuários
func (e *BaseEmulator) GetTotalUsers() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return e.DB.GetTotalUsers(ctx, e.Device.Model, e.Device.ID)
}

// GetSetting obtém uma configuração do dispositivo
func (e *BaseEmulator) GetSetting(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return e.DB.GetDeviceSettings(ctx, e.Device.ID, key)
}

// SetSetting define uma configuração do dispositivo
func (e *BaseEmulator) SetSetting(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return e.DB.SetDeviceSettings(ctx, e.Device.ID, key, value)
}

// HandleStatus é um handler comum para requisições de status
func (e *BaseEmulator) HandleStatus(w http.ResponseWriter, r *http.Request) {
	info := e.GetInfo()

	response := map[string]interface{}{
		"CurrentDatetime": time.Now().Format("2006-01-02 15:04:05"),
		"TotalUsers":      info.TotalUsers,
		"Status":          info.Status,
		"Model":           info.Model,
		"MacAddress":      e.MacAddress,
		"Version":         "1.0.0",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// SendEventToRemoteServer envia um evento para o servidor remoto
func (e *BaseEmulator) SendEventToRemoteServer(event interface{}, contentType string) error {
	// Verificar se autenticação local está ativada
	localAuth, err := e.GetSetting("LocalAuthentication")
	if err != nil {
		return fmt.Errorf("failed to get LocalAuthentication setting: %w", err)
	}

	if localAuth == "1" {
		e.Tracer.Debug("Local authentication enabled, not sending to remote server")
		return nil
	}

	// Obter configurações do servidor remoto
	remoteServer, err := e.GetSetting("RemoteServer")
	if err != nil {
		return fmt.Errorf("failed to get RemoteServer setting: %w", err)
	}

	remotePort, err := e.GetSetting("RemotePort")
	if err != nil {
		return fmt.Errorf("failed to get RemotePort setting: %w", err)
	}

	if remoteServer == "" || remotePort == "" {
		return fmt.Errorf("remote server configuration incomplete")
	}

	remoteURL := fmt.Sprintf("http://%s:%s/notification", remoteServer, remotePort)

	// // Serializar evento
	// payload, err := json.Marshal(event)
	// if err != nil {
	// 	return fmt.Errorf("failed to marshal event: %w", err)
	// }

	// Criar requisição HTTP
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", remoteURL,
		http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	req.Body = http.NoBody // Será definido pelos emuladores específicos

	// Executar requisição
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("remote server returned error: %s", resp.Status)
	}

	e.Tracer.Info("Event sent successfully to %s", remoteURL)

	// Simular eventos de porta se necessário
	if utils.RandomAccessNotDone() {
		e.simulateDoorEvents()
	}

	return nil
}

// simulateDoorEvents simula eventos de abertura/fechamento de porta
func (e *BaseEmulator) simulateDoorEvents() {
	go func() {
		time.Sleep(2 * time.Second)
		if err := e.sendDoorEvent("Open"); err != nil {
			e.Tracer.Error("Failed to send door open event: %v", err)
		}

		time.Sleep(3 * time.Second)
		if err := e.sendDoorEvent("Close"); err != nil {
			e.Tracer.Error("Failed to send door close event: %v", err)
		}
	}()
}

// sendDoorEvent envia evento de porta (deve ser implementado pelos emuladores específicos)
func (e *BaseEmulator) sendDoorEvent(status string) error {
	e.Tracer.Info("Door event: %s (base implementation)", status)
	return nil
}

// Cleanup realiza limpeza de recursos
func (e *BaseEmulator) Cleanup() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.stopEventGenerator()

	if e.stopChan != nil {
		close(e.stopChan)
		e.stopChan = nil
	}
}
