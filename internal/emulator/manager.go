package emulator

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/emulator/dahua"
	"GoFacialEmulator/internal/emulator/hikvision"
	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"
)

// Manager gerencia todos os emuladores - baseado no EmulatorService.py
type Manager struct {
	ServiceDB  database.DBInterface // Pode ser AdaptivePool ou DualPoolManager
	EmulatorDB database.DBInterface // Pode ser AdaptivePool ou DualPoolManager
	WxsDB      *database.WxsDB
	Tracer     *trace.Tracer

	// Mapa de emuladores ativos (equivalente ao devices_watchdog do Python)
	emulators     map[int]Emulator
	emulatorMutex sync.RWMutex // Mutex dedicado para emulators

	// Watchdog com mutex dedicado para evitar race conditions
	watchdog      map[int]*WatchdogInfo
	watchdogMutex sync.RWMutex // Mutex dedicado para watchdog

	// Canais para controle
	shutdownChan   chan struct{}
	watchdogTicker *time.Ticker

	statusListeners []StatusChangeListener
	listenersMutex  sync.RWMutex

	// Controle de refresh em andamento com atomic
	refreshInProgress atomic.Bool

	// Controle de pausa do watchdog
	watchdogPaused atomic.Bool
}

// WatchdogInfo armazena informações de monitoramento
type WatchdogInfo struct {
	FailureCount int
	LastCheck    time.Time
	LastStatus   string
}

type StatusChangeEvent struct {
	DeviceID int    `json:"device_id"`
	Status   string `json:"status"`
	Name     string `json:"name"`
}

type StatusChangeListener chan StatusChangeEvent

// NewManager cria um novo gerenciador de emuladores
func NewManager(serviceDB database.DBInterface, emulatorDB database.DBInterface, wxsDB *database.WxsDB, tracer *trace.Tracer) *Manager {
	return &Manager{
		ServiceDB:      serviceDB,
		EmulatorDB:     emulatorDB,
		WxsDB:          wxsDB,
		Tracer:         tracer,
		emulators:      make(map[int]Emulator),
		watchdog:       make(map[int]*WatchdogInfo),
		shutdownChan:   make(chan struct{}),
		watchdogTicker: time.NewTicker(10 * time.Second), // Equivalente ao schedule.every(10).seconds

		statusListeners: make([]StatusChangeListener, 0),
		listenersMutex:  sync.RWMutex{},
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
	// Usar atomic para refresh em andamento
	if !m.refreshInProgress.CompareAndSwap(false, true) {
		return fmt.Errorf("refresh already in progress")
	}
	defer m.refreshInProgress.Store(false)

	// Verificar se WxsDB está disponível
	if m.WxsDB == nil {
		return fmt.Errorf("WxsDB não está disponível - não é possível atualizar dispositivos")
	}

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

		// Inicializar watchdog se não existir (com proteção de mutex)
		m.watchdogMutex.Lock()
		if _, exists := m.watchdog[device.ID]; !exists {
			m.watchdog[device.ID] = &WatchdogInfo{
				FailureCount: 0,
				LastCheck:    time.Now(),
				LastStatus:   "stopped",
			}
		}
		m.watchdogMutex.Unlock()
	}

	// Remover dispositivos que não existem mais no WXS
	if err := m.cleanupOrphanedDevices(ctx, controllers); err != nil {
		m.Tracer.Error("Failed to cleanup orphaned devices: %v", err)
	}

	return nil
}

// IsRefreshInProgress verifica se há um refresh em andamento
func (m *Manager) IsRefreshInProgress() bool {
	return m.refreshInProgress.Load()
}

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

	// Fazer um único RLock para ler status de TODOS os emuladores (otimização crítica)
	m.emulatorMutex.RLock()
	statusMap := make(map[int]bool, len(m.emulators))
	for id, emulator := range m.emulators {
		statusMap[id] = emulator.IsRunning()
	}
	m.emulatorMutex.RUnlock()

	// Processar rows sem lock
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

		// Atualizar status baseado no snapshot (sem lock adicional!)
		if running, ok := statusMap[device.ID]; ok && running {
			device.Status = "running"
		} else {
			device.Status = "stopped"
		}

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
	m.emulatorMutex.RLock()
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

	m.emulatorMutex.RUnlock()
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
	m.emulatorMutex.Lock()
	defer m.emulatorMutex.Unlock()

	// Verificar se já está rodando
	if emulator, exists := m.emulators[id]; exists && emulator.IsRunning() {
		return fmt.Errorf("emulator %d already running", id)
	}

	// Obter informações do dispositivo com timeout
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

	// Iniciar emulador com timeout usando canal
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- emulator.Start()
	}()

	// Aguardar inicialização com timeout de 10 segundos
	select {
	case err := <-startErrChan:
		if err != nil {
			return fmt.Errorf("failed to start emulator: %w", err)
		}
	case <-time.After(10 * time.Second):
		// Tentar parar o emulador que pode estar travado
		_ = emulator.Stop()
		return fmt.Errorf("timeout starting emulator %d after 10 seconds", id)
	}

	// Armazenar emulador ANTES de atualizar banco (para garantir consistência)
	m.emulators[id] = emulator

	// Atualizar status no banco SINCRONAMENTE durante startup para evitar race conditions
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = m.ServiceDB.Exec(ctx, "UPDATE service.devices SET status = 'running' WHERE local_controller_id = $1", id)
	if err != nil {
		m.Tracer.Error("Failed to update device status in DB for device %d: %v", id, err)
		// Não falhar, mas logar - emulador já está rodando em memória
	}

	// Resetar contador de falhas do watchdog (com proteção)
	m.watchdogMutex.Lock()
	if info, exists := m.watchdog[id]; exists {
		info.FailureCount = 0
		info.LastStatus = "running"
	}
	m.watchdogMutex.Unlock()

	m.Tracer.Info("Started emulator for device %d (%s)", id, device.Name)

	// Notificar mudança de status
	go m.notifyStatusChange(id, "running", device.Name)

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
	m.emulatorMutex.Lock()
	defer m.emulatorMutex.Unlock()

	emulator, exists := m.emulators[id]
	if !exists || !emulator.IsRunning() {
		return fmt.Errorf("emulator %d not running", id)
	}

	// Obter nome do dispositivo antes de parar
	device, err := m.getDeviceUnsafe(id)
	deviceName := "Unknown"
	if err == nil {
		deviceName = device.Name
	}

	// Parar emulador
	if err := emulator.Stop(); err != nil {
		return fmt.Errorf("failed to stop emulator: %w", err)
	}

	// Atualizar status no banco
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = m.ServiceDB.Exec(ctx, "UPDATE service.devices SET status = 'stopped' WHERE local_controller_id = $1", id)
	if err != nil {
		m.Tracer.Error("Failed to update device status: %v", err)
	}

	// Remover do mapa
	delete(m.emulators, id)

	// Atualizar watchdog (com proteção)
	m.watchdogMutex.Lock()
	if info, exists := m.watchdog[id]; exists {
		info.LastStatus = "stopped"
		info.FailureCount = 0
	}
	m.watchdogMutex.Unlock()

	m.Tracer.Info("Stopped emulator for device %d", id)

	// ADICIONAR ESTA LINHA:
	// Notificar mudança de status
	go m.notifyStatusChange(id, "stopped", deviceName)

	return nil
}

// StartAll inicia todos os emuladores habilitados com controle de concorrência e retry
func (m *Manager) StartAll() error {
	// Pausar watchdog durante inicialização massiva
	m.Tracer.Info("Pausing watchdog during mass startup")
	m.watchdogPaused.Store(true)
	defer func() {
		// Aguardar 15 segundos após startup para estabilização antes de retomar watchdog
		go func() {
			time.Sleep(15 * time.Second)
			m.watchdogPaused.Store(false)
			m.Tracer.Info("Watchdog resumed after startup stabilization")
		}()
	}()

	devices, err := m.ListDevices()
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	// Filtrar apenas dispositivos habilitados e parados
	var devicesToStart []models.Device
	for _, device := range devices {
		if device.Enabled == 1 && device.Status == "stopped" {
			devicesToStart = append(devicesToStart, device)
		}
	}

	if len(devicesToStart) == 0 {
		m.Tracer.Info("No devices to start")
		return nil
	}

	m.Tracer.Info("Starting %d emulators with controlled concurrency (watchdog paused)", len(devicesToStart))

	// Configurações otimizadas para inicialização massiva
	const (
		maxConcurrent = 20                        // Aumentado para 20 emuladores simultâneos
		delayBetween  = 200 * time.Millisecond    // Delay reduzido entre batches
		maxRetries    = 3                         // Aumentado para 3 tentativas
		retryDelay    = 1 * time.Second           // Retry mais rápido
	)

	// Semáforo para controlar concorrência
	semaphore := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	errorsChan := make(chan error, len(devicesToStart))

	// Iniciar dispositivos em batches controlados
	for i, device := range devicesToStart {
		wg.Add(1)

		// Pequeno delay entre lançamento de goroutines (apenas entre batches)
		if i > 0 && i%maxConcurrent == 0 {
			time.Sleep(delayBetween)
		}

		go func(dev models.Device, index int) {
			defer wg.Done()

			// Adquirir slot no semáforo
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			m.Tracer.Info("[%d/%d] Starting device %d (%s)...", index+1, len(devicesToStart), dev.ID, dev.Name)

			// Tentar iniciar com retry limitado
			var lastErr error
			for attempt := 1; attempt <= maxRetries; attempt++ {
				err := m.Start(dev.ID)
				if err == nil {
					m.Tracer.Info("[%d/%d] ✓ Device %d started successfully", index+1, len(devicesToStart), dev.ID)
					return
				}

				lastErr = err

				// Se não for a última tentativa, retry rápido
				if attempt < maxRetries {
					m.Tracer.Warning("[%d/%d] Attempt %d failed for device %d, retrying in %v...",
						index+1, len(devicesToStart), attempt, dev.ID, retryDelay)
					time.Sleep(retryDelay)
				}
			}

			// Todas as tentativas falharam
			finalErr := fmt.Errorf("device %d (%s) failed after %d attempts: %w",
				dev.ID, dev.Name, maxRetries, lastErr)
			m.Tracer.Error("[%d/%d] ✗ %v", index+1, len(devicesToStart), finalErr)
			errorsChan <- finalErr

		}(device, i)
	}

	// Aguardar todas as goroutines terminarem
	wg.Wait()
	close(errorsChan)

	// Coletar erros
	var errors []error
	for err := range errorsChan {
		errors = append(errors, err)
	}

	// Reportar resultado final
	successCount := len(devicesToStart) - len(errors)
	m.Tracer.Info("StartAll completed: %d/%d devices started successfully", successCount, len(devicesToStart))

	if len(errors) > 0 {
		return fmt.Errorf("failed to start %d/%d devices", len(errors), len(devicesToStart))
	}

	return nil
}

// StopAll para todos os emuladores
func (m *Manager) StopAll() {
	m.emulatorMutex.Lock()
	defer m.emulatorMutex.Unlock()

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
	// Pular health checks se watchdog estiver pausado
	if m.watchdogPaused.Load() {
		m.Tracer.Info("Watchdog paused, skipping health checks")
		return
	}

	devices, err := m.ListDevices()
	if err != nil {
		m.Tracer.Error("Failed to list devices for health check: %v", err)
		return
	}

	for _, device := range devices {
		m.checkDeviceHealth(device)
	}
}

func (m *Manager) checkDeviceHealth(device models.Device) {
	// Ler status do emulador
	m.emulatorMutex.RLock()
	emulator, exists := m.emulators[device.ID]
	isRunning := exists && emulator.IsRunning()
	m.emulatorMutex.RUnlock()

	// Atualizar total_users se emulador estiver rodando (ANTES de acessar watchdog)
	if isRunning && emulator != nil {
		m.updateDeviceTotalUsers(device.ID, emulator)
	}

	// Acessar watchdog com proteção de mutex
	m.watchdogMutex.Lock()
	watchdogInfo, exists := m.watchdog[device.ID]
	if !exists {
		watchdogInfo = &WatchdogInfo{FailureCount: 0, LastCheck: time.Now(), LastStatus: "unknown"}
		m.watchdog[device.ID] = watchdogInfo
	}

	// Verificar se está rodando quando deveria
	if device.Status == "running" && !isRunning {
		watchdogInfo.FailureCount++
		failureCount := watchdogInfo.FailureCount
		m.watchdogMutex.Unlock() // Liberar lock ANTES de operações bloqueantes

		m.Tracer.Warning("Device %d (%s) should be running but isn't (failures: %d)",
			device.ID, device.Name, failureCount)

		// Tentar reiniciar após 3 falhas
		if failureCount >= 3 {
			m.Tracer.Info("Attempting to restart device %d after %d failures", device.ID, failureCount)
			if err := m.Start(device.ID); err != nil {
				m.Tracer.Error("Failed to restart device %d: %v", device.ID, err)
				go m.notifyStatusChange(device.ID, "error", device.Name)
			} else {
				// Resetar contador após sucesso
				m.watchdogMutex.Lock()
				if info, ok := m.watchdog[device.ID]; ok {
					info.FailureCount = 0
				}
				m.watchdogMutex.Unlock()
			}
		}
	} else if device.Status == "stopped" && isRunning {
		m.watchdogMutex.Unlock() // Liberar lock ANTES de Stop()
		m.Tracer.Info("Device %d is running but should be stopped", device.ID)
		m.Stop(device.ID)
	} else {
		watchdogInfo.FailureCount = 0
		watchdogInfo.LastCheck = time.Now()
		m.watchdogMutex.Unlock()
	}
}

func (m *Manager) updateDeviceTotalUsers(deviceID int, emulator Emulator) {
	// Obter total de usuários usando a interface comum
	totalUsers, err := emulator.GetTotalUsers()
	if err != nil {
		// Log do erro mas não interromper o processo
		m.Tracer.Error("[WATCHDOG] Failed to get total users for device %d: %v", deviceID, err)
		return
	}

	// Log para rastreamento
	m.Tracer.Info("[WATCHDOG] Device %d: Updating total_users to %d", deviceID, totalUsers)

	// Atualizar no banco (com timeout curto para não travar)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := m.ServiceDB.Exec(ctx,
		"UPDATE service.devices SET total_users = $1, updated_at = NOW() WHERE local_controller_id = $2",
		totalUsers, deviceID)

	if err != nil {
		m.Tracer.Error("[WATCHDOG] Failed to update total_users for device %d: %v", deviceID, err)
		return
	}

	// Verificar se a linha foi atualizada
	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		m.Tracer.Warning("[WATCHDOG] Device %d: No rows updated (device may not exist in database)", deviceID)
	} else {
		m.Tracer.Info("[WATCHDOG] Device %d: Successfully updated total_users=%d", deviceID, totalUsers)
	}
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

func (m *Manager) AddStatusListener() StatusChangeListener {
	m.listenersMutex.Lock()
	defer m.listenersMutex.Unlock()

	listener := make(StatusChangeListener, 10) // Buffer de 10 eventos
	m.statusListeners = append(m.statusListeners, listener)
	return listener
}

// RemoveStatusListener remove um listener
func (m *Manager) RemoveStatusListener(listener StatusChangeListener) {
	m.listenersMutex.Lock()
	defer m.listenersMutex.Unlock()

	for i, l := range m.statusListeners {
		if l == listener {
			close(l)
			m.statusListeners = append(m.statusListeners[:i], m.statusListeners[i+1:]...)
			break
		}
	}
}

func (m *Manager) notifyStatusChange(deviceID int, status string, deviceName string) {
	m.listenersMutex.RLock()
	defer m.listenersMutex.RUnlock()

	event := StatusChangeEvent{
		DeviceID: deviceID,
		Status:   status,
		Name:     deviceName,
	}

	// ADICIONAR ESTE LOG:
	m.Tracer.Info("Notifying status change: Device %d (%s) -> %s. Active listeners: %d",
		deviceID, deviceName, status, len(m.statusListeners))

	for i, listener := range m.statusListeners {
		select {
		case listener <- event:
			// ADICIONAR ESTE LOG:
			m.Tracer.Info("Successfully sent event to listener %d", i)
		default:
			// Se o canal estiver cheio, pula (evita bloqueio)
			m.Tracer.Warning("Status listener %d channel full, skipping event for device %d", i, deviceID)
		}
	}
}

func (m *Manager) GetPoolStats() map[string]interface{} {
	stats := map[string]interface{}{
		"timestamp": time.Now(),
	}

	// Tentar obter stats de DualPoolManager primeiro, senão AdaptivePool
	if dpm, ok := m.ServiceDB.(*database.DualPoolManager); ok {
		stats["service_db"] = dpm.GetStats()
	} else if ap, ok := m.ServiceDB.(*database.AdaptivePool); ok {
		stats["service_db"] = ap.GetStats()
	}

	if dpm, ok := m.EmulatorDB.(*database.DualPoolManager); ok {
		stats["emulator_db"] = dpm.GetStats()
	} else if ap, ok := m.EmulatorDB.(*database.AdaptivePool); ok {
		stats["emulator_db"] = ap.GetStats()
	}

	return stats
}

func (m *Manager) ListDevicesWithFilters(filters map[string]string) ([]*models.Device, error) {
	m.emulatorMutex.RLock()
	defer m.emulatorMutex.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Construir query com filtros
	query := `
		SELECT local_controller_id, name, ip_address, port, model, status, enabled, 
		       event_interval, total_users, log_enabled, type
		FROM service.devices
		WHERE 1=1
	`
	args := []interface{}{}
	argIndex := 1

	// Adicionar filtros condicionalmente
	if filters["id"] != "" {
		query += fmt.Sprintf(" AND local_controller_id = $%d", argIndex)
		if id, err := strconv.Atoi(filters["id"]); err == nil {
			args = append(args, id)
			argIndex++
		}
	}

	if filters["name"] != "" {
		query += fmt.Sprintf(" AND LOWER(name) LIKE LOWER($%d)", argIndex)
		args = append(args, "%"+filters["name"]+"%")
		argIndex++
	}

	if filters["port"] != "" {
		query += fmt.Sprintf(" AND port = $%d", argIndex)
		if port, err := strconv.Atoi(filters["port"]); err == nil {
			args = append(args, port)
			argIndex++
		}
	}

	query += " ORDER BY local_controller_id"

	rows, err := m.ServiceDB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query devices with filters: %w", err)
	}
	defer rows.Close()

	var devices []*models.Device
	for rows.Next() {
		device := &models.Device{}
		var enabled, logEnabled int

		err := rows.Scan(
			&device.ID, &device.Name, &device.IPAddress, &device.Port,
			&device.Model, &device.Status, &enabled, &device.EventInterval,
			&device.TotalUsers, &logEnabled, &device.Type,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan device: %w", err)
		}

		device.Enabled = enabled
		device.LogEnabled = logEnabled

		// Verificar se o emulador está realmente rodando
		if instance, exists := m.emulators[device.ID]; exists && instance.IsRunning() {
			device.Status = "running"
		} else {
			device.Status = "stopped"
		}

		devices = append(devices, device)
	}

	return devices, nil
}
