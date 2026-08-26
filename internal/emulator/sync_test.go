package emulator

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"
)

// A regra que sustenta a coexistência: sync só apaga o que o sync criou.
// Sem isso, o primeiro refresh depois de um cadastro manual apagaria o
// cadastro.
func TestOrphanIDsPreservaManuais(t *testing.T) {
	devices := []models.Device{
		{ID: 17, Source: SourceWXS},        // ainda existe no W-Access
		{ID: 18, Source: SourceWXS},        // sumiu do W-Access — órfão
		{ID: 900001, Source: SourceManual}, // cadastrado à mão
		{ID: 900002, Source: SourceManual},
	}
	validos := map[int]bool{17: true}

	got := orphanIDs(devices, validos)

	if quero := []int{18}; !reflect.DeepEqual(got, quero) {
		t.Errorf("órfãos: %v, quero %v", got, quero)
	}
}

func TestOrphanIDsSemOrfaos(t *testing.T) {
	devices := []models.Device{{ID: 17, Source: SourceWXS}}
	if got := orphanIDs(devices, map[int]bool{17: true}); len(got) != 0 {
		t.Errorf("quero nenhum órfão, tenho %v", got)
	}
}

// Dispositivo sem origem gravada é anterior à V002 — tratar como wxs, que
// é o que o DEFAULT da migração diz.
func TestOrphanIDsTrataSourceVazioComoWXS(t *testing.T) {
	devices := []models.Device{{ID: 18, Source: ""}}
	if got := orphanIDs(devices, map[int]bool{}); !reflect.DeepEqual(got, []int{18}) {
		t.Errorf("órfãos: %v, quero [18]", got)
	}
}

// Sem WxsDB não há o que sincronizar, e o erro precisa ser distinguível
// de "WXS fora do ar" — o handler devolve 409 num caso e 502 no outro.
func TestRefreshDevicesSemWxsDBDevolveSyncDisabled(t *testing.T) {
	m := &Manager{
		ServiceDB: &dbFalso{},
		Tracer:    trace.NewTracer(),
		emulators: map[int]Emulator{},
		watchdog:  map[int]*WatchdogInfo{},
	}

	err := m.RefreshDevices()
	if !errors.Is(err, ErrSyncDisabled) {
		t.Errorf("quero ErrSyncDisabled, tenho %v", err)
	}
}

// M-1: um LocalControllerID do W-Access que colida com a faixa manual
// (>= 900000) não pode fazer o upsert do sync tomar conta de um dispositivo
// cadastrado à mão. A defesa é o WHERE no DO UPDATE — este teste garante que
// a query que sai do código carrega esse filtro.
func TestUpsertDeviceFiltraPorSourceWxsNoConflito(t *testing.T) {
	db := &dbFalso{}
	m := &Manager{ServiceDB: db, Tracer: trace.NewTracer()}

	dev := models.Device{
		ID: 900001, Name: "colide", IPAddress: "10.0.0.5", Port: 4000,
		Model: ModelDahua, Enabled: 1, EventInterval: 10,
	}
	if err := m.upsertDevice(context.Background(), dev); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if len(db.execs) != 1 {
		t.Fatalf("quero 1 Exec, tenho %d: %v", len(db.execs), db.execs)
	}
	if !strings.Contains(db.execs[0], "WHERE service.devices.source = 'wxs'") {
		t.Error("DO UPDATE sem o filtro por source deixaria um id manual ser tomado por um upsert do W-Access")
	}
}

// Um refresh que falhou no gate não pode deixar a flag presa em true, ou
// o próximo refresh legítimo seria recusado como "já em andamento".
func TestRefreshDevicesLiberaFlagAposGate(t *testing.T) {
	m := &Manager{
		ServiceDB: &dbFalso{},
		Tracer:    trace.NewTracer(),
		emulators: map[int]Emulator{},
		watchdog:  map[int]*WatchdogInfo{},
	}

	_ = m.RefreshDevices()

	if m.IsRefreshInProgress() {
		t.Error("flag de refresh ficou presa em true")
	}
}
