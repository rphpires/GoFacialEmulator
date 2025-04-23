package emulator

import (
	"fmt"
	"sync"

	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"
)

// Manager gerencia todos os emuladores
type Manager struct {
	ServiceDB  *database.ServiceDB
	EmulatorDB *database.EmulatorDB
	WxsDB      *database.WxsDB
	Tracer     *trace.Tracer
	emulators  map[int]Emulator
	mutex      sync.RWMutex
}

// NewManager cria um novo gerenciador de emuladores
func NewManager(serviceDB *database.ServiceDB, emulatorDB *database.EmulatorDB, wxsDB *database.WxsDB, tracer *trace.Tracer) *Manager {
	return &Manager{
		ServiceDB:  serviceDB,
		EmulatorDB: emulatorDB,
		WxsDB:      wxsDB,
		Tracer:     tracer,
		emulators:  make(map[int]Emulator),
	}
}

// RefreshDevices atualiza a lista de dispositivos do banco de dados do WXS
func (m *Manager) RefreshDevices() error {
	m.Tracer.Info("Refreshing device list from WXS database")

	// Obter controladores configurados do WXS
	controllers, err := m.WxsDB.GetLocalControllers()
	if err != nil {
		return fmt.Errorf("failed to get controllers from WXS: %w", err)
	}

	// Atualizar banco de dados de serviço
	for _, controller := range controllers {
		id := controller["LocalControllerID"].(int)
		name := controller["Name"].(string)
		ip := controller["IPAddress"].(string)
		port := controller["Port"].(int)
		model := controller["Model"].(string)
		enabled := controller["Enabled"].(int)
		eventInterval := controller["EventInterval"].(int)

		// Verificar se o controlador já existe
		var count int
		err := m.ServiceDB.QueryRow("SELECT COUNT(*) FROM Main WHERE LocalControllerID = ?", id).Scan(&count)
		if err != nil {
			m.Tracer.Error("Failed to check if controller exists: %v", err)
			continue
		}

		if count > 0 {
			// Atualizar controlador existente
			_, err = m.ServiceDB.Exec(
				"UPDATE Main SET Name = ?, IPAddress = ?, Port = ?, Model = ?, Enabled = ?, EventInterval = ? WHERE LocalControllerID = ?",
				name, ip, port, model, enabled, eventInterval, id,
			)
			if err != nil {
				m.Tracer.Error("Failed to update controller: %v", err)
				continue
			}
		} else {
			// Inserir novo controlador
			_, err = m.ServiceDB.Exec(
				"INSERT INTO Main (LocalControllerID, Name, IPAddress, Port, Model, Enabled, Type, Status, EventInterval, TotalUsers, LogEnabled) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0)",
				id, name, ip, port, model, enabled, 0, "stopped", eventInterval,
			)
			if err != nil {
				m.Tracer.Error("Failed to insert controller: %v", err)
				continue
			}
		}
	}

	// Remover controladores que não existem mais
	rows, err := m.ServiceDB.Query("SELECT LocalControllerID FROM Main")
	if err != nil {
		return fmt.Errorf("failed to get controllers from service database: %w", err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			m.Tracer.Error("Failed to scan controller ID: %v", err)
			continue
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating controller IDs: %w", err)
	}

	for _, id := range ids {
		found := false
		for _, controller := range controllers {
			if controller["LocalControllerID"].(int) == id {
				found = true
				break
			}
		}

		if !found {
			// Parar o emulador se estiver em execução
			m.Stop(id)

			// Remover do banco de dados
			_, err := m.ServiceDB.Exec("DELETE FROM Main WHERE LocalControllerID = ?", id)
			if err != nil {
				m.Tracer.Error("Failed to delete controller: %v", err)
				continue
			}
		}
	}

	return nil
}

// ListDevices retorna uma lista de todos os dispositivos
func (m *Manager) ListDevices() ([]models.Device, error) {
	rows, err := m.ServiceDB.Query(`
		SELECT 
			LocalControllerID, Name, IPAddress, Port, Model, 
			Enabled, Type, Status, EventInterval, TotalUsers, LogEnabled 
		FROM Main
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query devices: %w", err)
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var device models.Device
		err := rows.Scan(
			&device.ID, &device.Name, &device.IPAddress, &device.Port, &device.Model,
			&device.Enabled, &device.Type, &device.Status, &device.EventInterval, &device.TotalUsers, &device.LogEnabled,
		)
		if err != nil {
			m.Tracer.Error("Failed to scan device row: %v", err)
			continue
		}

		// Verificar se o emulador está em execução
		m.mutex.RLock()
		emulator, exists := m.emulators[device.ID]
		if exists {
			device.Status = "running"
		}
		m.mutex.RUnlock()

		devices = append(devices, device)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating device rows: %w", err)
	}

	return devices, nil
}

// GetDevice retorna as informações de um dispositivo específico
func (m *Manager) GetDevice(id int) (models.Device, error) {
	var device models.Device

	err := m.ServiceDB.QueryRow(`
		SELECT 
			LocalControllerID, Name, IPAddress, Port, Model, 
			Enabled, Type, Status, EventInterval, TotalUsers, LogEnabled 
		FROM Main
		WHERE LocalControllerID = ?
	`, id).Scan(
		&device.ID, &device.Name, &device.IPAddress, &device.Port, &device.Model,
		&device.Enabled, &device.Type, &device.Status, &device.EventInterval, &device.TotalUsers, &device.LogEnabled,
	)

	if err != nil {
		return device, fmt.Errorf("failed to get device: %w", err)
	}

	// Verificar se o emulador está em execução
	m.mutex.RLock()
	emulator, exists := m.emulators[device.ID]
	if exists && emulator.IsRunning() {
		device.Status = "running"
	} else {
		device.Status = "stopped"
	}
	m.mutex.RUnlock()

	return device, nil
}

// Start inicia um emulador específico
func (m *Manager) Start(id int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Verificar se o emulador já está em execução
	if emulator, exists := m.emulators[id]; exists && emulator.IsRunning() {
		return fmt.Errorf("emulator already running")
	}

	// Obter informações do dispositivo
	device, err := m.GetDevice(id)
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	// Verificar se o dispositivo está habilitado
	if device.Enabled != 1 {
		return fmt.Errorf("device is disabled")
	}

	// Criar emulador apropriado com base no modelo
	var emulator Emulator
	switch device.Model {
	case "Dahua":
		emulator = NewDahuaEmulator(m.EmulatorDB, device, m.Tracer)
	case "Hikvision":
		// TODO: Implementar emulador Hikvision
		return fmt.Errorf("hikvision emulator not implemented yet")
	default:
		return fmt.Errorf("unknown device model: %s", device.Model)
	}

	// Iniciar o emulador
	if err := emulator.Start(); err != nil {
		return fmt.Errorf("failed to start emulator: %w", err)
	}

	// Atualizar o status no banco de dados
	_, err = m.ServiceDB.Exec("UPDATE Main SET Status = 'running' WHERE LocalControllerID = ?", id)
	if err != nil {
		m.Tracer.Error("Failed to update device status: %v", err)
	}

	// Armazenar o emulador
	m.emulators[id] = emulator

	m.Tracer.Info("Started emulator for device %d (%s)", id, device.Name)

	return nil
}

// Stop para um emulador específico
func (m *Manager) Stop(id int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Verificar se o emulador está em execução
	emulator, exists := m.emulators[id]
	if !exists || !emulator.IsRunning() {
		return fmt.Errorf("emulator not running")
	}

	// Parar o emulador
	if err := emulator.Stop(); err != nil {
		return fmt.Errorf("failed to stop emulator: %w", err)
	}

	// Atualizar o status no banco de dados
	_, err := m.ServiceDB.Exec("UPDATE Main SET Status = 'stopped' WHERE LocalControllerID = ?", id)
	if err != nil {
		m.Tracer.Error("Failed to update device status: %v", err)
	}

	// Remover o emulador
	delete(m.emulators, id)

	m.Tracer.Info("Stopped emulator for device %d", id)

	return nil
}

// StartAll inicia todos os emuladores habilitados
func (m *Manager) StartAll() error {
	devices, err := m.ListDevices()
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	for _, device := range devices {
		if device.Enabled == 1 && device.Status == "stopped" {
			if err := m.Start(device.ID); err != nil {
				m.Tracer.Error("Failed to start emulator %d (%s): %v", device.ID, device.Name, err)
			}
		}
	}

	return nil
}

// StopAll para todos os emuladores em execução
func (m *Manager) StopAll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for id, emulator := range m.emulators {
		if emulator.IsRunning() {
			if err := emulator.Stop(); err != nil {
				m.Tracer.Error("Failed to stop emulator %d: %v", id, err)
			}

			// Atualizar o status no banco de dados
			_, err := m.ServiceDB.Exec("UPDATE Main SET Status = 'stopped' WHERE LocalControllerID = ?", id)
			if err != nil {
				m.Tracer.Error("Failed to update device status: %v", err)
			}
		}
	}

	// Limpar o mapa de emuladores
	m.emulators = make(map[int]Emulator)

	m.Tracer.Info("Stopped all emulators")
}

// RefreshUsersComparison atualiza as contagens de usuários para comparação
func (m *Manager) RefreshUsersComparison() error {
	// Obter contagens de CHIDs do WXS
	counts, err := m.WxsDB.CountCHIDsByLocalController()
	if err != nil {
		return fmt.Errorf("failed to count CHIDs: %w", err)
	}

	// Atualizar a tabela UsersCount
	for siteControllerID, lcCounts := range counts {
		for lcID, info := range lcCounts {
			port := info[0].(int)
			count := info[1].(int)

			// Verificar se já existe um registro
			var exists int
			err := m.ServiceDB.QueryRow("SELECT COUNT(*) FROM UsersCount WHERE LocalControllerID = ?", lcID).Scan(&exists)
			if err != nil {
				m.Tracer.Error("Failed to check if user count exists: %v", err)
				continue
			}

			if exists > 0 {
				// Atualizar registro existente
				_, err = m.ServiceDB.Exec(
					"UPDATE UsersCount SET WxsCount = ?, SiteControllerID = ?, BaseCommPort = ? WHERE LocalControllerID = ?",
					count, siteControllerID, port, lcID,
				)
			} else {
				// Inserir novo registro
				_, err = m.ServiceDB.Exec(
					"INSERT INTO UsersCount (SiteControllerID, LocalControllerID, BaseCommPort, WxsCount, SiteControllerCount) VALUES (?, ?, ?, ?, 0)",
					siteControllerID, lcID, port, count,
				)
			}

			if err != nil {
				m.Tracer.Error("Failed to update user count: %v", err)
			}
		}
	}

	// TODO: Atualizar contagens do controlador de site

	return nil
}

// GetUserComparisons retorna as comparações de usuários
func (m *Manager) GetUserComparisons() ([]models.UserComparison, error) {
	rows, err := m.ServiceDB.Query(`
		SELECT 
			uc.SiteControllerID, 
			uc.LocalControllerID,
			m.Name, 
			uc.BaseCommPort,
			uc.WxsCount, 
			uc.SiteControllerCount, 
			m.TotalUsers 
		FROM UsersCount uc
		JOIN Main m ON m.LocalControllerID = uc.LocalControllerID
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query user comparisons: %w", err)
	}
	defer rows.Close()

	var comparisons []models.UserComparison
	for rows.Next() {
		var comp models.UserComparison
		err := rows.Scan(
			&comp.SiteControllerID,
			&comp.LocalControllerID,
			&comp.Name,
			&comp.Port,
			&comp.WxsCount,
			&comp.SiteControllerCount,
			&comp.EmulatorCount,
		)
		if err != nil {
			m.Tracer.Error("Failed to scan user comparison row: %v", err)
			continue
		}

		comparisons = append(comparisons, comp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user comparison rows: %w", err)
	}

	return comparisons, nil
}

// UpdateDeviceSettings atualiza as configurações de um dispositivo
func (m *Manager) UpdateDeviceSettings(id int, logEnabled bool) error {
	logEnabledInt := 0
	if logEnabled {
		logEnabledInt = 1
	}

	_, err := m.ServiceDB.Exec("UPDATE Main SET LogEnabled = ? WHERE LocalControllerID = ?", logEnabledInt, id)
	if err != nil {
		return fmt.Errorf("failed to update device settings: %w", err)
	}

	return nil
}
