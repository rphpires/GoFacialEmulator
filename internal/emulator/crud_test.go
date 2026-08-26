package emulator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"

	"github.com/jackc/pgx/v4"
)

// managerDeTeste devolve um Manager apoiado em fakes, com a porta do
// serviço em 8080 (o padrão do config.yaml).
func managerDeTeste(tx *txFalsa) (*Manager, *dbFalso) {
	db := &dbFalso{tx: tx}
	return &Manager{
		ServiceDB:   db,
		EmulatorDB:  db,
		Tracer:      trace.NewTracer(),
		ServicePort: 8080,
		emulators:   map[int]Emulator{},
		watchdog:    map[int]*WatchdogInfo{},
		startErrors: map[int]string{},
	}, db
}

// idSequencial simula nextval devolvendo 900001, 900002, ...
func idSequencial(inicio int) func(sql string) pgx.Row {
	proximo := inicio
	return func(sql string) pgx.Row {
		if strings.Contains(sql, "nextval") {
			id := proximo
			proximo++
			return linhaFalsa{valores: []interface{}{id}}
		}
		return linhaFalsa{}
	}
}

func TestCreateDeviceGravaComoManual(t *testing.T) {
	tx := &txFalsa{queryRowFn: idSequencial(900001)}
	m, _ := managerDeTeste(tx)

	dev, err := m.CreateDevice(context.Background(), DeviceSpec{
		Name: "lab-01", Model: ModelDahua, Port: 4000,
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if dev.ID != 900001 {
		t.Errorf("id %d, quero 900001", dev.ID)
	}
	if dev.Source != SourceManual {
		t.Errorf("source %q, quero %q", dev.Source, SourceManual)
	}
	if dev.Status != "stopped" {
		t.Errorf("status %q, quero stopped", dev.Status)
	}
	if dev.IPAddress != IPPadrao {
		t.Errorf("ip %q, quero o padrão %q", dev.IPAddress, IPPadrao)
	}
	if !tx.comitou {
		t.Error("quero commit")
	}

	juntos := strings.Join(tx.execs, "\n")
	if !strings.Contains(juntos, "pg_advisory_xact_lock") {
		t.Error("quero o lock que serializa duas criações concorrentes")
	}
	if !strings.Contains(juntos, "INSERT INTO service.devices") {
		t.Error("quero o INSERT do dispositivo")
	}
}

func TestCreateDeviceRejeitaSpecInvalida(t *testing.T) {
	tx := &txFalsa{queryRowFn: idSequencial(900001)}
	m, _ := managerDeTeste(tx)

	_, err := m.CreateDevice(context.Background(), DeviceSpec{Name: "x", Model: "Nedap", Port: 4000})

	if !errors.Is(err, ErrInvalidSpec) {
		t.Errorf("quero ErrInvalidSpec, tenho %v", err)
	}
	if len(tx.execs) != 0 {
		t.Errorf("nada podia ter sido executado, tenho %v", tx.execs)
	}
}

func TestCreateDeviceRecusaPortaOcupada(t *testing.T) {
	tx := &txFalsa{
		queryRowFn: idSequencial(900001),
		queryRows:  &rowsFalsas{linhas: [][]interface{}{{4000}, {7070}}},
	}
	m, _ := managerDeTeste(tx)

	_, err := m.CreateDevice(context.Background(), DeviceSpec{
		Name: "lab-01", Model: ModelDahua, Port: 4000,
	})

	var conf *ConflictError
	if !errors.As(err, &conf) {
		t.Fatalf("quero ConflictError, tenho %v", err)
	}
	if len(conf.Ports) != 1 || conf.Ports[0] != 4000 {
		t.Errorf("conflitos: %v, quero [4000]", conf.Ports)
	}
	if tx.comitou {
		t.Error("transação não podia ter sido confirmada")
	}
}

func TestCreateDeviceRecusaPortaDoProprioServico(t *testing.T) {
	tx := &txFalsa{queryRowFn: idSequencial(900001)}
	m, _ := managerDeTeste(tx) // ServicePort = 8080

	_, err := m.CreateDevice(context.Background(), DeviceSpec{
		Name: "colide", Model: ModelDahua, Port: 8080,
	})

	var conf *ConflictError
	if !errors.As(err, &conf) {
		t.Fatalf("quero ConflictError, tenho %v", err)
	}
	if len(conf.Reserved) != 1 || conf.Reserved[0] != 8080 {
		t.Errorf("reservadas: %v, quero [8080]", conf.Reserved)
	}
}

func TestCreateDeviceRangeCriaTodasAsPortas(t *testing.T) {
	tx := &txFalsa{queryRowFn: idSequencial(900001)}
	m, _ := managerDeTeste(tx)

	devs, err := m.CreateDeviceRange(context.Background(), RangeSpec{
		NamePrefix: "lab", Model: ModelHikvision, PortStart: 4000, PortEnd: 4002,
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if len(devs) != 3 {
		t.Fatalf("quero 3 dispositivos, tenho %d", len(devs))
	}
	if devs[0].Name != "lab-4000" || devs[2].Name != "lab-4002" {
		t.Errorf("nomes gerados: %q ... %q", devs[0].Name, devs[2].Name)
	}
	if devs[0].ID == devs[1].ID {
		t.Error("IDs repetidos no lote")
	}
	if !tx.comitou {
		t.Error("quero commit")
	}
}

// Colisão em qualquer porta do lote não grava nada — é o comportamento
// atômico que a spec exige.
func TestCreateDeviceRangeComColisaoNaoGravaNada(t *testing.T) {
	tx := &txFalsa{
		queryRowFn: idSequencial(900001),
		queryRows:  &rowsFalsas{linhas: [][]interface{}{{4001}}},
	}
	m, _ := managerDeTeste(tx)

	_, err := m.CreateDeviceRange(context.Background(), RangeSpec{
		NamePrefix: "lab", Model: ModelDahua, PortStart: 4000, PortEnd: 4002,
	})

	var conf *ConflictError
	if !errors.As(err, &conf) {
		t.Fatalf("quero ConflictError, tenho %v", err)
	}
	for _, sql := range tx.execs {
		if strings.Contains(sql, "INSERT INTO service.devices") {
			t.Error("nenhum INSERT podia ter acontecido")
		}
	}
	if tx.comitou {
		t.Error("transação não podia ter sido confirmada")
	}
}

func TestDeleteDevicePurgaTodasAsTabelas(t *testing.T) {
	tx := &txFalsa{}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceManual}}

	if err := m.DeleteDevice(context.Background(), 900001); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	juntos := strings.Join(tx.execs, "\n")
	tabelas := []string{
		"emulator.dahua_cards", "emulator.dahua_faces",
		"emulator.hikvision_users", "emulator.hikvision_cards",
		"emulator.hikvision_faces", "emulator.hikvision_fingers",
		"emulator.device_settings", "service.users_comparison",
		"service.devices",
	}
	for _, tab := range tabelas {
		if !strings.Contains(juntos, tab) {
			t.Errorf("faltou limpar %s", tab)
		}
	}
	if !tx.comitou {
		t.Error("quero commit")
	}
}

// A linha device_id = 0 de emulator.device_settings guarda os padrões
// globais semeados na V001. Purgar por device_id específico nunca pode
// alcançá-la.
func TestDeleteDeviceNaoTocaNosPadroesGlobais(t *testing.T) {
	tx := &txFalsa{}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceManual}}

	if err := m.DeleteDevice(context.Background(), 900001); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	for i, sql := range tx.execs {
		if strings.Contains(sql, "device_settings") {
			args := tx.execArgs[i]
			if len(args) == 0 || args[0].(int) != 900001 {
				t.Errorf("purga de device_settings com args %v", args)
			}
		}
	}
}

func TestDeleteDeviceRecusaDispositivoDoWXS(t *testing.T) {
	tx := &txFalsa{}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceWXS}}

	err := m.DeleteDevice(context.Background(), 17)

	if !errors.Is(err, ErrDeviceIsManaged) {
		t.Errorf("quero ErrDeviceIsManaged, tenho %v", err)
	}
}

func TestUpdateDeviceRecusaDispositivoDoWXS(t *testing.T) {
	tx := &txFalsa{}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceWXS}}

	_, err := m.UpdateDevice(context.Background(), 17, DeviceSpec{
		Name: "novo", Model: ModelDahua, Port: 4000,
	})

	if !errors.Is(err, ErrDeviceIsManaged) {
		t.Errorf("quero ErrDeviceIsManaged, tenho %v", err)
	}
}

// emuladorFalso satisfaz a interface Emulator para simular um em execução.
type emuladorFalso struct{ rodando bool }

func (e *emuladorFalso) Start() error                { return nil }
func (e *emuladorFalso) Stop() error                 { e.rodando = false; return nil }
func (e *emuladorFalso) IsRunning() bool             { return e.rodando }
func (e *emuladorFalso) GetInfo() models.Device      { return models.Device{} }
func (e *emuladorFalso) GenerateEvent() error        { return nil }
func (e *emuladorFalso) GetType() string             { return ModelDahua }
func (e *emuladorFalso) GetTotalUsers() (int, error) { return 0, nil }

func TestUpdateDeviceRecusaEmuladorRodando(t *testing.T) {
	tx := &txFalsa{}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceManual}}
	m.emulators[900001] = &emuladorFalso{rodando: true}

	_, err := m.UpdateDevice(context.Background(), 900001, DeviceSpec{
		Name: "novo", Model: ModelDahua, Port: 4000,
	})

	if !errors.Is(err, ErrDeviceRunning) {
		t.Errorf("quero ErrDeviceRunning, tenho %v", err)
	}
}

// leituraDoDispositivo simula, dentro da transação, a query que lê port,
// total_users e log_enabled do próprio dispositivo — separada de
// deviceSource (que roda fora da transação, em dbFalso.linha) e de
// idSequencial (nextval, usada só na criação).
func leituraDoDispositivo(porta, totalUsuarios, logEnabled int) func(sql string) pgx.Row {
	return func(sql string) pgx.Row {
		if strings.Contains(sql, "SELECT port") {
			return linhaFalsa{valores: []interface{}{porta, totalUsuarios, logEnabled}}
		}
		return linhaFalsa{}
	}
}

// Regressão: a porta atual do dispositivo tem que ser excluída do conjunto
// de portas ocupadas antes de checar conflito, senão toda edição que não
// muda a porta esbarra na própria porta.
func TestUpdateDeviceMantemAPortaAtual(t *testing.T) {
	tx := &txFalsa{
		queryRowFn: leituraDoDispositivo(4000, 0, 0),
		queryRows:  &rowsFalsas{linhas: [][]interface{}{{4000}, {7070}}},
	}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceManual}}

	dev, err := m.UpdateDevice(context.Background(), 900001, DeviceSpec{
		Name: "lab-01", Model: ModelDahua, Port: 4000,
	})
	if err != nil {
		t.Fatalf("não podia haver conflito ao manter a própria porta: %v", err)
	}
	if dev.Port != 4000 {
		t.Errorf("porta %d, quero 4000", dev.Port)
	}

	juntos := strings.Join(tx.execs, "\n")
	if !strings.Contains(juntos, "UPDATE service.devices") {
		t.Error("quero o UPDATE do dispositivo")
	}
	if !tx.comitou {
		t.Error("quero commit")
	}
}

// Task 6 renderiza a resposta HTTP direto do models.Device devolvido por
// UpdateDevice: total_users e log_enabled não podem zerar numa edição, já
// que o UPDATE deliberadamente não toca essas duas colunas.
func TestUpdateDevicePreservaTotalUsersELogEnabled(t *testing.T) {
	tx := &txFalsa{
		queryRowFn: leituraDoDispositivo(4000, 300, 1),
		queryRows:  &rowsFalsas{linhas: [][]interface{}{{4000}, {7070}}},
	}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceManual}}

	dev, err := m.UpdateDevice(context.Background(), 900001, DeviceSpec{
		Name: "lab-01", Model: ModelDahua, Port: 4000,
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if dev.TotalUsers != 300 {
		t.Errorf("total_users %d, quero 300", dev.TotalUsers)
	}
	if dev.LogEnabled != 1 {
		t.Errorf("log_enabled %d, quero 1", dev.LogEnabled)
	}
}
