package emulator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/emulator/dahua"
	"GoFacialEmulator/internal/emulator/hikvision"
	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"
)

// Manager gerencia todos os emuladores - baseado no EmulatorService.py
type Manager struct {
	ServiceDB  *database.SimpleOptimizedPool
	EmulatorDB *database.SimpleOptimizedPool
	WxsDB      *database.WxsDB
	Tracer     *trace.Tracer

	// Mapa de emuladores ativos (equivalente ao devices_watchdog do Python)
	emulators map[int]Emulator
	watchdog  map[int]*WatchdogInfo
	mutex     sync.RWMutex

	// Canais para controle
	shutdownChan   chan struct{}
	watchdogTicker *time.Ticker
}

// WatchdogInfo armazena informações de monitoramento
type WatchdogInfo struct {
	FailureCount int
	LastCheck    time.Time
	LastStatus   string
}

// NewManager cria um novo gerenciador de emuladores
func NewManager(serviceDB *database.SimpleOptimizedPool, emulatorDB *database.SimpleOptimizedPool, wxsDB *database.WxsDB, tracer *trace.Tracer) *Manager {
	return &Manager{
		ServiceDB:      serviceDB,
		EmulatorDB:     emulatorDB,
		WxsDB:          wxsDB,
		Tracer:         tracer,
		emulators:      make(map[int]Emulator),
		watchdog:       make(map[int]*WatchdogInfo),
		shutdownChan:   make(chan struct{}),
		watchdogTicker: time.NewTicker(10 * time.Second), // Equivalente ao schedule.every(10).seconds
	}
}

// Initialize inicializa o sistema - equivalente ao init_devices() do Python
func (m *Manager) Initialize() error {
	m.Tracer.Info("Initializing emulator manager")

	// Marcar todos como parados na inicialização
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := m.ServiceDB.Exec(ctx, "UPDATE service.devices SET status = 'stopped'")
	if err != nil {
		return fmt.Errorf("failed to reset device status: %w", err)
	}

	// Inicializar watchdog para dispositivos existentes
	devices, err := m.ListDevices()
	if err != nil {
		return fmt.Errorf("failed to list devices for initialization: %w", err)
	}

	for _, device := range devices {
		m.watchdog[device.ID] = &WatchdogInfo{
			FailureCount: 0,
			LastCheck:    time.Now(),
			LastStatus:   "stopped",
		}
	}

	// Iniciar watchdog em background
	go m.startWatchdog()

	return nil
}

// RefreshDevices atualiza a lista de dispositivos do WXS - equivalente ao refresh_configured_devices()
func (m *Manager) RefreshDevices() error {
	m.Tracer.Info("Refreshing device list from WXS database")

	// Obter controladores do WXS
	controllers, err := m.WxsDB.GetLocalControllers()
	if err != nil {
		return fmt.Errorf("failed to get controllers from WXS: %w", err)
	}

	m.Tracer.Info("DEBUG: Found %d controllers from WXS", len(controllers))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Processar cada controlador
	for _, controller := range controllers {
		device := m.mapControllerToDevice(controller)

		if err := m.upsertDevice(ctx, device); err != nil {
			m.Tracer.Error("Failed to upsert device %d: %v", device.ID, err)
			continue
		}

		// Inicializar watchdog se não existir
		if _, exists := m.watchdog[device.ID]; !exists {
			m.watchdog[device.ID] = &WatchdogInfo{
				FailureCount: 0,
				LastCheck:    time.Now(),
				LastStatus:   "stopped",
			}
		}
	}

	// Remover dispositivos que não existem mais no WXS
	if err := m.cleanupOrphanedDevices(ctx, controllers); err != nil {
		m.Tracer.Error("Failed to cleanup orphaned devices: %v", err)
	}

	return nil
}

// mapControllerToDevice converte dados do WXS para modelo de dispositivo
func (m *Manager) mapControllerToDevice(controller map[string]interface{}) models.Device {
	id := controller["LocalControllerID"].(int)
	name := controller["Name"].(string)
	ip := controller["IPAddress"].(string)
	port := controller["Port"].(int)
	model := controller["Model"].(string)
	enabled := controller["Enabled"].(int)
	eventInterval := controller["EventInterval"].(int)

	return models.Device{
		ID:            id,
		Name:          name,
		IPAddress:     ip,
		Port:          port,
		Model:         model,
		Enabled:       enabled,
		Type:          m.getDeviceType(model),
		Status:        "stopped",
		EventInterval: eventInterval,
		TotalUsers:    0,
		LogEnabled:    0,
	}
}

// getDeviceType determina o tipo do dispositivo baseado no modelo
func (m *Manager) getDeviceType(model string) int {
	switch model {
	case "Dahua":
		return 1
	case "Hikvision":
		return 2
	default:
		return 0
	}
}

// upsertDevice insere ou atualiza um dispositivo
func (m *Manager) upsertDevice(ctx context.Context, device models.Device) error {
	query := `
		INSERT INTO service.devices (
			local_controller_id, name, ip_address, port, model, enabled, type, 
			status, event_interval, total_users, log_enabled
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (local_controller_id) DO UPDATE SET
			name = EXCLUDED.name,
			ip_address = EXCLUDED.ip_address,
			port = EXCLUDED.port,
			model = EXCLUDED.model,
			enabled = EXCLUDED.enabled,
			type = EXCLUDED.type,
			event_interval = EXCLUDED.event_interval,
			updated_at = NOW()
	`

	_, err := m.ServiceDB.Exec(ctx, query,
		device.ID, device.Name, device.IPAddress, device.Port, device.Model,
		device.Enabled, device.Type, device.Status, device.EventInterval,
		device.TotalUsers, device.LogEnabled)

	return err
}

// cleanupOrphanedDevices remove dispositivos órfãos
func (m *Manager) cleanupOrphanedDevices(ctx context.Context, controllers []map[string]interface{}) error {
	// Criar mapa de IDs válidos
	validIDs := make(map[int]bool)
	for _, controller := range controllers {
		id := controller["LocalControllerID"].(int)
		validIDs[id] = true
	}

	// Obter todos os dispositivos locais
	devices, err := m.ListDevices()
	if err != nil {
		return err
	}

	// Remover dispositivos órfãos
	for _, device := range devices {
		if !validIDs[device.ID] {
			// Parar emulador se estiver rodando
			if emulator, exists := m.emulators[device.ID]; exists && emulator.IsRunning() {
				m.Stop(device.ID)
			}

			// Remover do banco
			_, err := m.ServiceDB.Exec(ctx, "DELETE FROM service.devices WHERE local_controller_id = $1", device.ID)
			if err != nil {
				m.Tracer.Error("Failed to delete orphaned device %d: %v", device.ID, err)
			}

			// Remover do watchdog
			delete(m.watchdog, device.ID)
		}
	}

	return nil
}

// ListDevices retorna todos os dispositivos
func (m *Manager) ListDevices() ([]models.Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT local_controller_id, name, ip_address, port, model, enabled, type, 
		       status, event_interval, total_users, log_enabled
		FROM service.devices
		ORDER BY local_controller_id
	`

	rows, err := m.ServiceDB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query devices: %w", err)
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var device models.Device
		err := rows.Scan(&device.ID, &device.Name, &device.IPAddress, &device.Port,
			&device.Model, &device.Enabled, &device.Type, &device.Status,
			&device.EventInterval, &device.TotalUsers, &device.LogEnabled)
		if err != nil {
			m.Tracer.Error("Failed to scan device: %v", err)
			continue
		}

		// Atualizar status baseado no emulador real
		m.Tracer.Info("Attempting to acquire RLock... ListDevices")
		m.mutex.RLock()
		if emulator, exists := m.emulators[device.ID]; exists && emulator.IsRunning() {
			device.Status = "running"
		} else {
			device.Status = "stopped"
		}
		m.mutex.RUnlock()
		m.Tracer.Info("RLock released: ListDevices")

		devices = append(devices, device)
	}

	return devices, nil
}

// GetDevice retorna um dispositivo específico
func (m *Manager) GetDevice(id int) (models.Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT local_controller_id, name, ip_address, port, model, enabled, type, 
		       status, event_interval, total_users, log_enabled
		FROM service.devices
		WHERE local_controller_id = $1
	`

	m.Tracer.Info("GetDevice in DB with ID=%d", id)
	var device models.Device
	err := m.ServiceDB.QueryRow(ctx, query, id).Scan(
		&device.ID, &device.Name, &device.IPAddress, &device.Port,
		&device.Model, &device.Enabled, &device.Type, &device.Status,
		&device.EventInterval, &device.TotalUsers, &device.LogEnabled)

	if err != nil {
		return device, fmt.Errorf("device not found: %w", err)
	}

	m.Tracer.Info("GetDevice founded in DB ID=%d", device.ID)
	// Atualizar status baseado no emulador real
	m.Tracer.Info("About to acquire RLock for device %d", device.ID)
	m.mutex.RLock()
	m.Tracer.Info("RLock acquired successfully")
	// if emulator, exists := m.emulators[device.ID]; exists && emulator.IsRunning() {
	// 	device.Status = "running"
	// } else {
	// 	device.Status = "stopped"
	// }
	if emulator, exists := m.emulators[device.ID]; exists {
		m.Tracer.Info("Emulator exists for device %d, checking if running...", device.ID)
		isRunning := emulator.IsRunning() // ← PODE TRAVAR AQUI!
		m.Tracer.Info("IsRunning() returned: %v", isRunning)

		if isRunning {
			device.Status = "running"
		} else {
			device.Status = "stopped"
		}
	} else {
		m.Tracer.Info("No emulator found for device %d", device.ID)
		device.Status = "stopped"
	}

	m.mutex.RUnlock()
	m.Tracer.Info("RLock released: GetDevice")

	m.Tracer.Info("Device with ID=%d was founded, Status=%s", id, device.Status)
	return device, nil
}

func (m *Manager) getDeviceUnsafe(id int) (models.Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
        SELECT local_controller_id, name, ip_address, port, model, enabled, type, 
               status, event_interval, total_users, log_enabled
        FROM service.devices
        WHERE local_controller_id = $1
    `

	var device models.Device
	err := m.ServiceDB.QueryRow(ctx, query, id).Scan(
		&device.ID, &device.Name, &device.IPAddress, &device.Port,
		&device.Model, &device.Enabled, &device.Type, &device.Status,
		&device.EventInterval, &device.TotalUsers, &device.LogEnabled)

	if err != nil {
		return device, fmt.Errorf("device not found: %w", err)
	}

	// Atualizar status baseado no emulador real (SEM LOCK - já está dentro de lock)
	if emulator, exists := m.emulators[device.ID]; exists && emulator.IsRunning() {
		device.Status = "running"
	} else {
		device.Status = "stopped"
	}

	return device, nil
}

// Start inicia um emulador específico - equivalente ao start_emulators() do Python
func (m *Manager) Start(id int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Verificar se já está rodando
	if emulator, exists := m.emulators[id]; exists && emulator.IsRunning() {
		return fmt.Errorf("emulator %d already running", id)
	}

	// Obter informações do dispositivo
	device, err := m.getDeviceUnsafe(id)
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	if device.Enabled != 1 {
		return fmt.Errorf("device %d is disabled", id)
	}

	// Criar emulador baseado no modelo
	emulator, err := m.createEmulator(device)
	if err != nil {
		return fmt.Errorf("failed to create emulator: %w", err)
	}

	// Iniciar emulador
	if err := emulator.Start(); err != nil {
		return fmt.Errorf("failed to start emulator: %w", err)
	}

	// Atualizar status no banco
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = m.ServiceDB.Exec(ctx, "UPDATE service.devices SET status = 'running' WHERE local_controller_id = $1", id)
	if err != nil {
		m.Tracer.Error("Failed to update device status: %v", err)
	}

	// Armazenar emulador
	m.emulators[id] = emulator

	// Resetar contador de falhas do watchdog
	if info, exists := m.watchdog[id]; exists {
		info.FailureCount = 0
		info.LastStatus = "running"
	}

	m.Tracer.Info("Started emulator for device %d (%s)", id, device.Name)
	return nil
}

// createEmulator cria um emulador baseado no tipo de dispositivo
func (m *Manager) createEmulator(device models.Device) (Emulator, error) {
	switch device.Model {
	case "Dahua":
		return dahua.NewEmulator(m.EmulatorDB, device, m.Tracer), nil
	case "Hikvision":
		return hikvision.NewEmulator(m.EmulatorDB, device, m.Tracer), nil
	default:
		return nil, fmt.Errorf("unsupported device model: %s", device.Model)
	}
}

// Stop para um emulador específico
func (m *Manager) Stop(id int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	emulator, exists := m.emulators[id]
	if !exists || !emulator.IsRunning() {
		return fmt.Errorf("emulator %d not running", id)
	}

	// Parar emulador
	if err := emulator.Stop(); err != nil {
		return fmt.Errorf("failed to stop emulator: %w", err)
	}

	// Atualizar status no banco
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := m.ServiceDB.Exec(ctx, "UPDATE service.devices SET status = 'stopped' WHERE local_controller_id = $1", id)
	if err != nil {
		m.Tracer.Error("Failed to update device status: %v", err)
	}

	// Remover do mapa
	delete(m.emulators, id)

	// Atualizar watchdog
	if info, exists := m.watchdog[id]; exists {
		info.LastStatus = "stopped"
		info.FailureCount = 0
	}

	m.Tracer.Info("Stopped emulator for device %d", id)
	return nil
}

// StartAll inicia todos os emuladores habilitados
func (m *Manager) StartAll() error {
	devices, err := m.ListDevices()
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	var errors []string
	for _, device := range devices {
		if device.Enabled == 1 && device.Status == "stopped" {
			if err := m.Start(device.ID); err != nil {
				errorMsg := fmt.Sprintf("device %d: %v", device.ID, err)
				errors = append(errors, errorMsg)
				m.Tracer.Error("Failed to start emulator %d: %v", device.ID, err)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to start some emulators: %s", errors)
	}

	return nil
}

// StopAll para todos os emuladores
func (m *Manager) StopAll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for id, emulator := range m.emulators {
		if emulator.IsRunning() {
			if err := emulator.Stop(); err != nil {
				m.Tracer.Error("Failed to stop emulator %d: %v", id, err)
			}

			// Atualizar status no banco
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := m.ServiceDB.Exec(ctx, "UPDATE service.devices SET status = 'stopped' WHERE local_controller_id = $1", id)
			cancel()

			if err != nil {
				m.Tracer.Error("Failed to update device status: %v", err)
			}
		}
	}

	// Limpar mapa
	m.emulators = make(map[int]Emulator)
	m.Tracer.Info("Stopped all emulators")
}

// startWatchdog inicia o sistema de monitoramento - equivalente ao scheduler() do Python
func (m *Manager) startWatchdog() {
	m.Tracer.Info("Starting watchdog system")

	for {
		select {
		case <-m.watchdogTicker.C:
			m.performHealthChecks()
		case <-m.shutdownChan:
			m.Tracer.Info("Watchdog system shutting down")
			return
		}
	}
}

// performHealthChecks realiza verificações de saúde - equivalente ao refresh_device_status()
func (m *Manager) performHealthChecks() {
	devices, err := m.ListDevices()
	if err != nil {
		m.Tracer.Error("Failed to list devices for health check: %v", err)
		return
	}

	for _, device := range devices {
		m.checkDeviceHealth(device)
	}
}

// checkDeviceHealth verifica a saúde de um dispositivo - equivalente ao check_connection() e emulator_watchdog()
func (m *Manager) checkDeviceHealth(device models.Device) {
	m.Tracer.Info("Attempting to acquire RLock...checkDeviceHealth")
	m.mutex.RLock()
	emulator, exists := m.emulators[device.ID]
	isRunning := exists && emulator.IsRunning()
	m.mutex.RUnlock()
	m.Tracer.Info("RLock released: checkDeviceHealth")

	m.Tracer.Info("DeviceID=%d is running=%t", device.ID, isRunning)

	watchdogInfo, exists := m.watchdog[device.ID]
	if !exists {
		watchdogInfo = &WatchdogInfo{FailureCount: 0, LastCheck: time.Now(), LastStatus: "unknown"}
		m.watchdog[device.ID] = watchdogInfo
	}

	// Verificar se está rodando quando deveria
	if device.Status == "running" && !isRunning {
		watchdogInfo.FailureCount++
		m.Tracer.Warning("Device %d (%s) should be running but isn't (failures: %d)",
			device.ID, device.Name, watchdogInfo.FailureCount)

		// Tentar reiniciar após 3 falhas
		if watchdogInfo.FailureCount >= 3 {
			m.Tracer.Info("Attempting to restart device %d after %d failures", device.ID, watchdogInfo.FailureCount)
			if err := m.Start(device.ID); err != nil {
				m.Tracer.Error("Failed to restart device %d: %v", device.ID, err)
			} else {
				watchdogInfo.FailureCount = 0
			}
		}
	} else if device.Status == "stopped" && isRunning {
		// Parar se não deveria estar rodando
		m.Tracer.Info("Device %d is running but should be stopped", device.ID)
		m.Stop(device.ID)
	} else {
		// Tudo OK, resetar contador
		watchdogInfo.FailureCount = 0
	}

	watchdogInfo.LastCheck = time.Now()
}

// Shutdown fecha o manager gracefully
func (m *Manager) Shutdown() {
	m.Tracer.Info("Shutting down emulator manager")

	// Sinalizar parada do watchdog
	close(m.shutdownChan)

	// Parar ticker
	m.watchdogTicker.Stop()

	// Parar todos os emuladores
	m.StopAll()
}

// UpdateDeviceSettings atualiza configurações de um dispositivo
func (m *Manager) UpdateDeviceSettings(id int, logEnabled bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logEnabledInt := 0
	if logEnabled {
		logEnabledInt = 1
	}

	_, err := m.ServiceDB.Exec(ctx,
		"UPDATE service.devices SET log_enabled = $1, updated_at = NOW() WHERE local_controller_id = $2",
		logEnabledInt, id)

	if err != nil {
		return fmt.Errorf("failed to update device settings: %w", err)
	}

	return nil
}
