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

	// ServicePort é a porta HTTP do próprio serviço. Cadastrar um emulador
	// nela produziria um dispositivo que nunca consegue subir, e o erro
	// apareceria só no start, longe da causa.
	ServicePort int

	// Mapa de emuladores ativos (equivalente ao devices_watchdog do Python)
	emulators     map[int]Emulator
	emulatorMutex sync.RWMutex // Mutex dedicado para emulators

	// Watchdog com mutex dedicado para evitar race conditions
	watchdog      map[int]*WatchdogInfo
	watchdogMutex sync.RWMutex // Mutex dedicado para watchdog

	// Canais para controle
	shutdownChan   chan struct{}
	watchdogTicker *time.Ticker

	hub fleetHub

	// starting reserva os IDs com start em andamento. O start de um
	// emulador pode levar até 10 s; segurar emulatorMutex durante esse tempo
	// serializava o StartAll inteiro e travava ListDevices (e, com ele, a
	// tela) enquanto a frota subia.
	starting map[int]bool

	// Controle de refresh em andamento com atomic
	refreshInProgress atomic.Bool

	// Controle de pausa do watchdog
	watchdogPaused atomic.Bool

	// Último erro de abertura de porta por dispositivo. É o sinal de
	// alcançabilidade em ambiente nativo, onde o bind é direto.
	startErrors     map[int]string
	startErrorMutex sync.RWMutex
}

// WatchdogInfo armazena informações de monitoramento
type WatchdogInfo struct {
	FailureCount int
	LastCheck    time.Time
	LastStatus   string
}

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
		starting:       make(map[int]bool),
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

	// O vínculo com o Invenzi é opcional. Erro tipado, e não fmt.Errorf,
	// porque o handler precisa distinguir "sync desligado" (estado
	// esperado, 409) de "W-Access fora do ar" (falha, 502).
	if m.WxsDB == nil {
		return fmt.Errorf("%w: W-Access não configurado", ErrSyncDisabled)
	}

	gateCtx, gateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	ligado, err := database.GetSyncEnabled(gateCtx, m.ServiceDB, m.WxsDB != nil)
	gateCancel()
	if err != nil {
		return fmt.Errorf("failed to read sync setting: %w", err)
	}
	if !ligado {
		return fmt.Errorf("%w: desligada nas configurações", ErrSyncDisabled)
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

	// Inclusões e remoções em massa: a tela relê a frota inteira.
	m.notifyResync()
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
	// O WHERE no DO UPDATE é obrigatório, não decorativo: um LocalControllerID
	// do W-Access que colida com a faixa manual (>= 900000 — ver
	// service.manual_device_id_seq) não pode silenciosamente tomar conta de
	// um dispositivo cadastrado à mão. Sem o filtro, a linha continuaria com
	// source='manual', mas nome, IP, porta, modelo, habilitado e intervalo
	// teriam sido sobrescritos pelo controlador do W-Access — a linha
	// sobrevive, o conteúdo manual não. A qualificação pela tabela
	// (service.devices.source, não só "source") é exigida pelo Postgres em
	// WHERE de ON CONFLICT DO UPDATE.
	query := `
		INSERT INTO service.devices (
			local_controller_id, name, ip_address, port, model, enabled, type,
			status, event_interval, total_users, log_enabled, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'wxs')
		ON CONFLICT (local_controller_id) DO UPDATE SET
			name = EXCLUDED.name,
			ip_address = EXCLUDED.ip_address,
			port = EXCLUDED.port,
			model = EXCLUDED.model,
			enabled = EXCLUDED.enabled,
			type = EXCLUDED.type,
			event_interval = EXCLUDED.event_interval,
			updated_at = NOW()
		WHERE service.devices.source = 'wxs'
	`

	_, err := m.ServiceDB.Exec(ctx, query,
		device.ID, device.Name, device.IPAddress, device.Port, device.Model,
		device.Enabled, device.Type, device.Status, device.EventInterval,
		device.TotalUsers, device.LogEnabled)

	return err
}

// orphanIDs devolve os dispositivos que o sync deve apagar: os de origem
// W-Access que não vieram na última resposta. Emulador manual nunca é
// órfão — ninguém além do próprio operador manda nele.
func orphanIDs(devices []models.Device, validos map[int]bool) []int {
	var orfaos []int
	for _, d := range devices {
		if d.Source == SourceManual {
			continue
		}
		if !validos[d.ID] {
			orfaos = append(orfaos, d.ID)
		}
	}
	return orfaos
}

// cleanupOrphanedDevices remove dispositivos que sumiram do W-Access.
func (m *Manager) cleanupOrphanedDevices(ctx context.Context, controllers []map[string]interface{}) error {
	validIDs := make(map[int]bool)
	for _, controller := range controllers {
		id := controller["LocalControllerID"].(int)
		validIDs[id] = true
	}

	devices, err := m.ListDevices()
	if err != nil {
		return err
	}

	for _, id := range orphanIDs(devices, validIDs) {
		if emulator, exists := m.emulators[id]; exists && emulator.IsRunning() {
			m.Stop(id)
		}

		// O AND source garante que uma corrida entre o refresh e um
		// cadastro manual não apague o cadastro: se a linha virou manual
		// entre o SELECT e o DELETE, o DELETE não pega nada.
		_, err := m.ServiceDB.Exec(ctx,
			"DELETE FROM service.devices WHERE local_controller_id = $1 AND source = 'wxs'", id)
		if err != nil {
			m.Tracer.Error("Failed to delete orphaned device %d: %v", id, err)
		}

		m.watchdogMutex.Lock()
		delete(m.watchdog, id)
		m.watchdogMutex.Unlock()
	}

	return nil
}

// ListDevices retorna todos os dispositivos
func (m *Manager) ListDevices() ([]models.Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT local_controller_id, name, ip_address, port, model, enabled, type,
		       status, event_interval, total_users, log_enabled, source
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
	iniciando := make(map[int]bool, len(m.starting))
	for id := range m.starting {
		iniciando[id] = true
	}
	m.emulatorMutex.RUnlock()

	// Processar rows sem lock
	var devices []models.Device
	for rows.Next() {
		var device models.Device
		err := rows.Scan(&device.ID, &device.Name, &device.IPAddress, &device.Port,
			&device.Model, &device.Enabled, &device.Type, &device.Status,
			&device.EventInterval, &device.TotalUsers, &device.LogEnabled, &device.Source)
		if err != nil {
			m.Tracer.Error("Failed to scan device: %v", err)
			continue
		}

		// Atualizar status baseado no snapshot (sem lock adicional!)
		switch {
		case statusMap[device.ID]:
			device.Status = "running"
		case iniciando[device.ID]:
			device.Status = "starting"
		default:
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
		       status, event_interval, total_users, log_enabled, source
		FROM service.devices
		WHERE local_controller_id = $1
	`

	m.Tracer.Info("GetDevice in DB with ID=%d", id)
	var device models.Device
	err := m.ServiceDB.QueryRow(ctx, query, id).Scan(
		&device.ID, &device.Name, &device.IPAddress, &device.Port,
		&device.Model, &device.Enabled, &device.Type, &device.Status,
		&device.EventInterval, &device.TotalUsers, &device.LogEnabled, &device.Source)

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
               status, event_interval, total_users, log_enabled, source
        FROM service.devices
        WHERE local_controller_id = $1
    `

	var device models.Device
	err := m.ServiceDB.QueryRow(ctx, query, id).Scan(
		&device.ID, &device.Name, &device.IPAddress, &device.Port,
		&device.Model, &device.Enabled, &device.Type, &device.Status,
		&device.EventInterval, &device.TotalUsers, &device.LogEnabled, &device.Source)

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
	// Reserva o ID e solta o mutex antes do start propriamente dito: o
	// emulador abre porta e conexões, o que pode levar segundos, e o mutex
	// é o mesmo que ListDevices usa para ler o estado da frota.
	m.emulatorMutex.Lock()
	if emulator, exists := m.emulators[id]; exists && emulator.IsRunning() {
		m.emulatorMutex.Unlock()
		return fmt.Errorf("emulator %d already running", id)
	}
	if m.starting[id] {
		m.emulatorMutex.Unlock()
		return fmt.Errorf("emulator %d is already starting", id)
	}
	if m.starting == nil {
		m.starting = make(map[int]bool)
	}
	m.starting[id] = true
	m.emulatorMutex.Unlock()

	err := m.startReserved(id)

	m.emulatorMutex.Lock()
	delete(m.starting, id)
	m.emulatorMutex.Unlock()

	// Falha também notifica: o cliente que pediu o start está esperando o
	// estado final, e o último erro vai junto na releitura.
	m.NotifyChanged(id)
	return err
}

// startReserved faz o start de um ID já reservado em m.starting.
func (m *Manager) startReserved(id int) error {
	m.emulatorMutex.RLock()
	device, err := m.getDeviceUnsafe(id)
	m.emulatorMutex.RUnlock()
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	if device.Enabled != 1 {
		return fmt.Errorf("device %d is disabled", id)
	}

	emulator, err := m.createEmulator(device)
	if err != nil {
		return fmt.Errorf("failed to create emulator: %w", err)
	}

	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- emulator.Start()
	}()

	select {
	case err := <-startErrChan:
		m.recordStartResult(id, err)
		if err != nil {
			return fmt.Errorf("failed to start emulator: %w", err)
		}
	case <-time.After(10 * time.Second):
		// Tentar parar o emulador que pode estar travado
		_ = emulator.Stop()
		erroTimeout := fmt.Errorf("timeout starting emulator %d after 10 seconds", id)
		m.recordStartResult(id, erroTimeout)
		return erroTimeout
	}

	// Armazenar emulador ANTES de atualizar banco (para garantir consistência)
	m.emulatorMutex.Lock()
	m.emulators[id] = emulator
	m.emulatorMutex.Unlock()

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
	emulator, exists := m.emulators[id]
	if !exists || !emulator.IsRunning() {
		m.emulatorMutex.Unlock()
		return fmt.Errorf("emulator %d not running", id)
	}

	if err := emulator.Stop(); err != nil {
		m.emulatorMutex.Unlock()
		return fmt.Errorf("failed to stop emulator: %w", err)
	}
	delete(m.emulators, id)
	m.emulatorMutex.Unlock()

	m.markStopped(id)
	m.Tracer.Info("Stopped emulator for device %d", id)
	m.NotifyChanged(id)
	return nil
}

// markStopped grava o estado parado e zera o watchdog. Fica fora do
// emulatorMutex: é I/O de banco, e segurar o mutex aqui travava a leitura
// da frota.
func (m *Manager) markStopped(id int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := m.ServiceDB.Exec(ctx, "UPDATE service.devices SET status = 'stopped' WHERE local_controller_id = $1", id); err != nil {
		m.Tracer.Error("Failed to update device status: %v", err)
	}

	m.watchdogMutex.Lock()
	if info, exists := m.watchdog[id]; exists {
		info.LastStatus = "stopped"
		info.FailureCount = 0
	}
	m.watchdogMutex.Unlock()
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
	// Tira o mapa inteiro de uma vez e para fora do mutex: parar e gravar
	// no banco um por um com o mutex preso travava ListDevices até o fim.
	m.emulatorMutex.Lock()
	ativos := m.emulators
	m.emulators = make(map[int]Emulator)
	m.emulatorMutex.Unlock()

	for id, emulator := range ativos {
		if !emulator.IsRunning() {
			continue
		}
		if err := emulator.Stop(); err != nil {
			m.Tracer.Error("Failed to stop emulator %d: %v", id, err)
		}
		m.markStopped(id)
		// A versão anterior não notificava aqui: "Parar todos" só
		// aparecia na tela depois de um F5.
		m.NotifyChanged(id)
	}

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
		m.updateDeviceTotalUsers(device.ID, device.TotalUsers, emulator)
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
				m.NotifyChanged(device.ID)
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

// updateDeviceTotalUsers grava a contagem de usuários quando ela muda e
// avisa a tela. Antes gravava todo dispositivo rodando a cada ciclo do
// watchdog, mesmo sem mudança (1000 UPDATEs e 2000 linhas de trace a cada
// 10 s numa frota grande), e não avisava ninguém — a coluna Usuários só
// mudava com F5.
func (m *Manager) updateDeviceTotalUsers(deviceID, atual int, emulator Emulator) {
	totalUsers, err := emulator.GetTotalUsers()
	if err != nil {
		m.Tracer.Error("[WATCHDOG] Failed to get total users for device %d: %v", deviceID, err)
		return
	}
	if totalUsers == atual {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := m.ServiceDB.Exec(ctx,
		"UPDATE service.devices SET total_users = $1, updated_at = NOW() WHERE local_controller_id = $2",
		totalUsers, deviceID); err != nil {
		m.Tracer.Error("[WATCHDOG] Failed to update total_users for device %d: %v", deviceID, err)
		return
	}

	m.Tracer.Info("[WATCHDOG] Device %d: total_users %d -> %d", deviceID, atual, totalUsers)
	m.NotifyChanged(deviceID)
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

	m.NotifyChanged(id)
	return nil
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
		       event_interval, total_users, log_enabled, type, source
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
			&device.TotalUsers, &logEnabled, &device.Type, &device.Source,
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

// recordStartResult guarda ou limpa o erro do último start do dispositivo.
// Um start bem-sucedido apaga o erro anterior: o veredito precisa refletir
// a última tentativa, não a pior.
func (m *Manager) recordStartResult(deviceID int, err error) {
	m.startErrorMutex.Lock()
	defer m.startErrorMutex.Unlock()

	if m.startErrors == nil {
		m.startErrors = make(map[int]string)
	}

	if err == nil {
		delete(m.startErrors, deviceID)
		return
	}
	m.startErrors[deviceID] = err.Error()
}

// LastStartError devolve o erro do último start do dispositivo, ou string
// vazia se o último start funcionou ou se nunca houve start.
func (m *Manager) LastStartError(deviceID int) string {
	m.startErrorMutex.RLock()
	defer m.startErrorMutex.RUnlock()
	return m.startErrors[deviceID]
}
