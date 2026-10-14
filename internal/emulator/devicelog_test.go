package emulator

import (
	"context"
	"os"
	"strings"
	"testing"

	"GoFacialEmulator/internal/devicelog"
	"GoFacialEmulator/internal/trace"
)

func managerComLogs(t *testing.T, db *dbFalso) (*Manager, devicelog.Options) {
	t.Helper()
	o := devicelog.Options{Dir: t.TempDir(), MaxSize: 1 << 20, MaxFiles: 8}
	return &Manager{
		ServiceDB:   db,
		EmulatorDB:  db,
		Tracer:      trace.NewTracer(),
		emulators:   map[int]Emulator{},
		watchdog:    map[int]*WatchdogInfo{},
		startErrors: map[int]string{},
		Logs:        devicelog.NewRegistry(o, nil),
	}, o
}

func TestUpdateDeviceSettingsLigaComCabecalhoDoBanco(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{valores: []interface{}{
		5, "Portaria B", "10.0.0.1", 4005, "Dahua", 1, 0, "stopped", 0, 0, 1, "manual",
	}}}
	m, o := managerComLogs(t, db)

	if err := m.UpdateDeviceSettings(5, true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(o.FilePath(5, 0))
	if err != nil {
		t.Fatalf("arquivo não criado: %v", err)
	}
	if !strings.HasPrefix(string(b), `# device 5 "Portaria B" Dahua porta 4005`) {
		t.Errorf("cabeçalho: %s", b)
	}

	if err := m.UpdateDeviceSettings(5, false); err != nil {
		t.Fatal(err)
	}
	if fs := o.Files(5); len(fs) != 0 {
		t.Errorf("desligar não apagou: %v", fs)
	}
}

// Mesma montagem de TestDeleteDevicePurgaTodasAsTabelas (crud_test.go):
// deviceSource lê db.linha e precisa ver "manual".
func TestDeleteDeviceApagaLogs(t *testing.T) {
	m, db := managerDeTeste(&txFalsa{})
	db.linha = linhaFalsa{valores: []interface{}{SourceManual}}
	o := devicelog.Options{Dir: t.TempDir(), MaxSize: 1 << 20, MaxFiles: 8}
	m.Logs = devicelog.NewRegistry(o, nil)
	_ = m.Logs.SetEnabled(devicelog.Info{ID: 900001}, true)

	if err := m.DeleteDevice(context.Background(), 900001); err != nil {
		t.Fatal(err)
	}
	if fs := o.Files(900001); len(fs) != 0 {
		t.Errorf("DeleteDevice não apagou os logs: %v", fs)
	}
}

// Dispositivo que sumiu do W-Access também leva os logs: um ID reaproveitado
// não pode continuar o arquivo do antigo.
func TestCleanupOrphanedDevicesApagaLogs(t *testing.T) {
	db := &dbFalso{queryRows: &rowsFalsas{linhas: [][]interface{}{
		{5, "Some", "10.0.0.1", 4005, "Dahua", 1, 0, "stopped", 0, 0, 1, "wxs"},
	}}}
	m, o := managerComLogs(t, db)
	_ = m.Logs.SetEnabled(devicelog.Info{ID: 5}, true)

	if err := m.cleanupOrphanedDevices(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if fs := o.Files(5); len(fs) != 0 {
		t.Errorf("logs do órfão ficaram: %v", fs)
	}
}
