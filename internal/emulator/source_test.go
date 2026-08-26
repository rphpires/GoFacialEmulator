package emulator

import (
	"testing"

	"GoFacialEmulator/internal/trace"
)

// A ordem das colunas no SELECT e no Scan precisa bater. Um teste que
// devolve uma linha completa é o que pega uma coluna acrescentada no SQL
// e esquecida no Scan — o erro mais fácil de cometer aqui.
func TestListDevicesWithFiltersLeSource(t *testing.T) {
	db := &dbFalso{
		queryRows: &rowsFalsas{linhas: [][]interface{}{
			// local_controller_id, name, ip_address, port, model, status,
			// enabled, event_interval, total_users, log_enabled, type, source
			{900001, "lab-4000", "192.168.1.50", 4000, "Dahua", "stopped", 1, 10, 0, 0, 1, "manual"},
			{17, "Portaria", "10.0.0.7", 7070, "Hikvision", "stopped", 1, 10, 3, 0, 2, "wxs"},
		}},
	}

	m := &Manager{ServiceDB: db, Tracer: trace.NewTracer(), emulators: map[int]Emulator{}}

	devices, err := m.ListDevicesWithFilters(map[string]string{})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("quero 2 dispositivos, tenho %d", len(devices))
	}
	if devices[0].Source != SourceManual {
		t.Errorf("dispositivo 900001: source %q, quero %q", devices[0].Source, SourceManual)
	}
	if devices[1].Source != SourceWXS {
		t.Errorf("dispositivo 17: source %q, quero %q", devices[1].Source, SourceWXS)
	}
}
