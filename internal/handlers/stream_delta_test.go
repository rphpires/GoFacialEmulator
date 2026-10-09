package handlers

import (
	"reflect"
	"testing"

	"GoFacialEmulator/internal/emulator"
	"GoFacialEmulator/internal/models"
)

func TestPendenciasUltimoAvisoVence(t *testing.T) {
	p := novasPendencias()
	if !p.vazia() {
		t.Fatal("pendências novas não estão vazias")
	}

	p.registrar(emulator.FleetEvent{Kind: emulator.FleetChanged, DeviceID: 1})
	p.registrar(emulator.FleetEvent{Kind: emulator.FleetRemoved, DeviceID: 1})
	p.registrar(emulator.FleetEvent{Kind: emulator.FleetRemoved, DeviceID: 2})
	p.registrar(emulator.FleetEvent{Kind: emulator.FleetChanged, DeviceID: 2})

	if p.alterados[1] || !p.removidos[1] {
		t.Errorf("dispositivo 1 alterado e depois removido deveria constar só como removido: %+v", p)
	}
	if !p.alterados[2] || p.removidos[2] {
		t.Errorf("dispositivo 2 removido e depois recriado deveria constar só como alterado: %+v", p)
	}
	if p.resync {
		t.Error("resync marcado sem aviso de resync")
	}

	p.registrar(emulator.FleetEvent{Kind: emulator.FleetResync})
	if !p.resync {
		t.Error("resync não marcado")
	}
}

func TestParseControlIDs(t *testing.T) {
	all, ids, invalid := parseControlIDs([]string{"3", "x", "7"})
	if all || !reflect.DeepEqual(ids, []int{3, 7}) || !reflect.DeepEqual(invalid, []string{"x"}) {
		t.Errorf("got all=%v ids=%v invalid=%v", all, ids, invalid)
	}

	all, ids, _ = parseControlIDs([]string{"3", "all"})
	if !all || ids != nil {
		t.Errorf("\"all\" deveria vencer: all=%v ids=%v", all, ids)
	}
}

func TestNewDeviceViewLevaOrigemEModoPadrao(t *testing.T) {
	v := newDeviceView(models.Device{ID: 9, Enabled: 0, Status: "running", Source: "manual", IPAddress: "10.0.0.1"})

	if v.Status != "disabled" {
		t.Errorf("status = %q, quero disabled", v.Status)
	}
	if v.Source != "manual" || v.IPAddress != "10.0.0.1" {
		t.Errorf("origem/IP não copiados: %+v", v)
	}
	if v.Mode != modoStandalone {
		t.Errorf("modo padrão = %q, quero %q", v.Mode, modoStandalone)
	}
}
