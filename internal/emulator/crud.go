package emulator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"GoFacialEmulator/internal/models"

	"github.com/jackc/pgx/v4"
)

// chaveLockCriacao serializa a criação de emuladores. A validação de porta
// e a gravação precisam ser atômicas entre si: sem o lock, duas criações
// simultâneas passam as duas pela validação e colidem na gravação.
//
// Um UNIQUE em service.devices(port) seria mais direto, mas quebraria a
// migração de qualquer instalação cujo W-Access já tenha portas repetidas,
// e o esquema atual não impede isso.
const chaveLockCriacao int64 = 8080_4000

// CreateDevice cadastra um emulador manual.
func (m *Manager) CreateDevice(ctx context.Context, spec DeviceSpec) (models.Device, error) {
	if err := spec.Normalize(); err != nil {
		return models.Device{}, err
	}

	devices, err := m.criarVarios(ctx, []DeviceSpec{spec})
	if err != nil {
		return models.Device{}, err
	}
	return devices[0], nil
}

// CreateDeviceRange cadastra um emulador por porta do intervalo. Ou todos
// entram, ou nenhum entra.
func (m *Manager) CreateDeviceRange(ctx context.Context, spec RangeSpec) ([]models.Device, error) {
	if err := spec.Normalize(); err != nil {
		return nil, err
	}
	return m.criarVarios(ctx, spec.Expand())
}

// criarVarios é o caminho único de gravação dos dois verbos de criação.
func (m *Manager) criarVarios(ctx context.Context, specs []DeviceSpec) ([]models.Device, error) {
	tx, err := m.ServiceDB.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chaveLockCriacao); err != nil {
		return nil, fmt.Errorf("erro ao obter lock de criação: %w", err)
	}

	ocupadas, err := portasOcupadas(ctx, tx)
	if err != nil {
		return nil, err
	}

	desejadas := make([]int, 0, len(specs))
	for _, s := range specs {
		desejadas = append(desejadas, s.Port)
	}

	conflito := &ConflictError{Ports: Conflicts(desejadas, ocupadas)}
	if m.ServicePort > 0 {
		conflito.Reserved = Conflicts(desejadas, map[int]bool{m.ServicePort: true})
	}
	if len(conflito.Ports) > 0 || len(conflito.Reserved) > 0 {
		return nil, conflito
	}

	devices := make([]models.Device, 0, len(specs))
	for _, s := range specs {
		var id int
		if err := tx.QueryRow(ctx,
			"SELECT nextval('service.manual_device_id_seq')").Scan(&id); err != nil {
			return nil, fmt.Errorf("erro ao alocar id: %w", err)
		}

		dev := models.Device{
			ID:            id,
			Name:          s.Name,
			IPAddress:     s.IPAddress,
			Port:          s.Port,
			Model:         s.Model,
			Enabled:       boolParaInt(*s.Enabled),
			Type:          m.getDeviceType(s.Model),
			Status:        "stopped",
			EventInterval: s.EventInterval,
			TotalUsers:    0,
			LogEnabled:    0,
			Source:        SourceManual,
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO service.devices (
				local_controller_id, name, ip_address, port, model, enabled, type,
				status, event_interval, total_users, log_enabled, source
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'manual')`,
			dev.ID, dev.Name, dev.IPAddress, dev.Port, dev.Model, dev.Enabled,
			dev.Type, dev.Status, dev.EventInterval, dev.TotalUsers, dev.LogEnabled)
		if err != nil {
			return nil, fmt.Errorf("erro ao inserir dispositivo na porta %d: %w", s.Port, err)
		}

		devices = append(devices, dev)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("erro ao confirmar criação: %w", err)
	}

	// Só depois do commit: watchdog apontando para dispositivo que não
	// existe geraria health check em cima de nada.
	m.watchdogMutex.Lock()
	for _, dev := range devices {
		m.watchdog[dev.ID] = &WatchdogInfo{LastCheck: time.Now(), LastStatus: "stopped"}
	}
	m.watchdogMutex.Unlock()

	return devices, nil
}

// portasOcupadas lê todas as portas já cadastradas, de qualquer origem —
// um emulador manual não pode colidir nem com outro manual nem com um
// dispositivo vindo do W-Access.
func portasOcupadas(ctx context.Context, tx pgx.Tx) (map[int]bool, error) {
	rows, err := tx.Query(ctx, "SELECT port FROM service.devices")
	if err != nil {
		return nil, fmt.Errorf("erro ao ler portas cadastradas: %w", err)
	}
	defer rows.Close()

	ocupadas := map[int]bool{}
	for rows.Next() {
		var porta int
		if err := rows.Scan(&porta); err != nil {
			return nil, fmt.Errorf("erro ao ler porta cadastrada: %w", err)
		}
		ocupadas[porta] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar portas cadastradas: %w", err)
	}
	return ocupadas, nil
}

// UpdateDevice substitui os campos editáveis de um emulador manual parado.
func (m *Manager) UpdateDevice(ctx context.Context, id int, spec DeviceSpec) (models.Device, error) {
	if err := spec.Normalize(); err != nil {
		return models.Device{}, err
	}

	if err := m.exigeManualEParado(ctx, id); err != nil {
		return models.Device{}, err
	}

	tx, err := m.ServiceDB.Begin(ctx)
	if err != nil {
		return models.Device{}, fmt.Errorf("erro ao abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chaveLockCriacao); err != nil {
		return models.Device{}, fmt.Errorf("erro ao obter lock: %w", err)
	}

	ocupadas, err := portasOcupadas(ctx, tx)
	if err != nil {
		return models.Device{}, err
	}
	// A porta atual do próprio dispositivo não conta como conflito: manter
	// a porta é o caso mais comum de edição. A leitura vai pela própria
	// transação — buscar uma segunda conexão do pool enquanto se segura o
	// advisory lock é contenção sem motivo, e o valor lido tem que ser do
	// mesmo snapshot que portasOcupadas.
	var portaAtual, totalUsuariosAtual, logAtual int
	err = tx.QueryRow(ctx, `
		SELECT port, total_users, log_enabled
		  FROM service.devices
		 WHERE local_controller_id = $1`, id).Scan(&portaAtual, &totalUsuariosAtual, &logAtual)
	if err != nil {
		return models.Device{}, fmt.Errorf("erro ao ler dispositivo %d: %w", id, err)
	}
	delete(ocupadas, portaAtual)

	conflito := &ConflictError{Ports: Conflicts([]int{spec.Port}, ocupadas)}
	if m.ServicePort > 0 {
		conflito.Reserved = Conflicts([]int{spec.Port}, map[int]bool{m.ServicePort: true})
	}
	if len(conflito.Ports) > 0 || len(conflito.Reserved) > 0 {
		return models.Device{}, conflito
	}

	_, err = tx.Exec(ctx, `
		UPDATE service.devices
		   SET name = $2, ip_address = $3, port = $4, model = $5,
		       enabled = $6, type = $7, event_interval = $8, updated_at = NOW()
		 WHERE local_controller_id = $1 AND source = 'manual'`,
		id, spec.Name, spec.IPAddress, spec.Port, spec.Model,
		boolParaInt(*spec.Enabled), m.getDeviceType(spec.Model), spec.EventInterval)
	if err != nil {
		return models.Device{}, fmt.Errorf("erro ao atualizar dispositivo %d: %w", id, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return models.Device{}, fmt.Errorf("erro ao confirmar atualização: %w", err)
	}

	return models.Device{
		ID: id, Name: spec.Name, IPAddress: spec.IPAddress, Port: spec.Port,
		Model: spec.Model, Enabled: boolParaInt(*spec.Enabled),
		Type: m.getDeviceType(spec.Model), Status: "stopped",
		EventInterval: spec.EventInterval, Source: SourceManual,
		TotalUsers:    totalUsuariosAtual,
		LogEnabled:    logAtual,
	}, nil
}

// tabelasDeDados são todas as tabelas com device_id solto. Não há FK no
// esquema, então a limpeza é explícita.
var tabelasDeDados = []string{
	"emulator.dahua_cards",
	"emulator.dahua_faces",
	"emulator.hikvision_users",
	"emulator.hikvision_cards",
	"emulator.hikvision_faces",
	"emulator.hikvision_fingers",
	"emulator.device_settings",
}

// DeleteDevice remove um emulador manual e tudo que ele acumulou. Para o
// emulador antes, sem exigir que o operador pare na mão: remover é uma
// intenção inequívoca, editar não.
func (m *Manager) DeleteDevice(ctx context.Context, id int) error {
	origem, err := m.deviceSource(ctx, id)
	if err != nil {
		return err
	}
	if origem != SourceManual {
		return fmt.Errorf("%w: dispositivo %d", ErrDeviceIsManaged, id)
	}

	m.emulatorMutex.RLock()
	inst, rodando := m.emulators[id]
	m.emulatorMutex.RUnlock()
	if rodando && inst.IsRunning() {
		if err := m.Stop(id); err != nil {
			return fmt.Errorf("erro ao parar emulador %d antes de remover: %w", id, err)
		}
	}

	tx, err := m.ServiceDB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("erro ao abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, tabela := range tabelasDeDados {
		// Filtro por device_id específico: a linha device_id = 0 de
		// device_settings guarda os padrões globais e não pertence a
		// dispositivo nenhum.
		if _, err := tx.Exec(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE device_id = $1", tabela), id); err != nil {
			return fmt.Errorf("erro ao limpar %s do dispositivo %d: %w", tabela, id, err)
		}
	}

	if _, err := tx.Exec(ctx,
		"DELETE FROM service.users_comparison WHERE local_controller_id = $1", id); err != nil {
		return fmt.Errorf("erro ao limpar comparação do dispositivo %d: %w", id, err)
	}

	if _, err := tx.Exec(ctx,
		"DELETE FROM service.devices WHERE local_controller_id = $1 AND source = 'manual'", id); err != nil {
		return fmt.Errorf("erro ao remover dispositivo %d: %w", id, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("erro ao confirmar remoção: %w", err)
	}

	m.watchdogMutex.Lock()
	delete(m.watchdog, id)
	m.watchdogMutex.Unlock()

	m.emulatorMutex.Lock()
	delete(m.emulators, id)
	m.emulatorMutex.Unlock()

	m.Tracer.Info("Removed manual device %d", id)
	return nil
}

// deviceSource lê a origem de um dispositivo.
func (m *Manager) deviceSource(ctx context.Context, id int) (string, error) {
	var origem string
	err := m.ServiceDB.QueryRow(ctx,
		"SELECT source FROM service.devices WHERE local_controller_id = $1", id).Scan(&origem)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: %d", ErrDeviceNotFound, id)
	}
	if err != nil {
		return "", fmt.Errorf("erro ao ler origem do dispositivo %d: %w", id, err)
	}
	if origem == "" {
		// Linha anterior à V002: o DEFAULT da migração diz wxs.
		origem = SourceWXS
	}
	return origem, nil
}

// exigeManualEParado é a guarda comum da edição.
func (m *Manager) exigeManualEParado(ctx context.Context, id int) error {
	origem, err := m.deviceSource(ctx, id)
	if err != nil {
		return err
	}
	if origem != SourceManual {
		return fmt.Errorf("%w: dispositivo %d", ErrDeviceIsManaged, id)
	}

	m.emulatorMutex.RLock()
	inst, existe := m.emulators[id]
	m.emulatorMutex.RUnlock()
	if existe && inst.IsRunning() {
		return fmt.Errorf("%w: pare o emulador %d antes de editar", ErrDeviceRunning, id)
	}
	return nil
}

func boolParaInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
