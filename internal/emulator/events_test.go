package emulator

import "testing"

func TestFleetHubEntregaEmOrdem(t *testing.T) {
	m := &Manager{}
	l := m.AddFleetListener()
	defer m.RemoveFleetListener(l)

	m.NotifyChanged(1)
	m.notifyRemoved(2)
	m.notifyResync()

	quero := []FleetEvent{
		{Kind: FleetChanged, DeviceID: 1},
		{Kind: FleetRemoved, DeviceID: 2},
		{Kind: FleetResync},
	}
	for i, q := range quero {
		if got := <-l.C; got != q {
			t.Errorf("evento %d = %+v, quero %+v", i, got, q)
		}
	}
	if l.TakeOverflow() {
		t.Error("overflow marcado sem transbordar")
	}
}

// Canal cheio não pode bloquear quem notifica, e o descarte precisa ficar
// registrado: é o que faz o stream mandar um snapshot em vez de deixar a
// tela errada.
func TestFleetHubMarcaOverflow(t *testing.T) {
	m := &Manager{}
	l := m.AddFleetListener()
	defer m.RemoveFleetListener(l)

	for i := 0; i < fleetListenerBuffer+5; i++ {
		m.NotifyChanged(i)
	}

	if !l.TakeOverflow() {
		t.Fatal("overflow não marcado")
	}
	if l.TakeOverflow() {
		t.Error("TakeOverflow não zerou a marca")
	}
}

func TestFleetHubRemoveFechaCanal(t *testing.T) {
	m := &Manager{}
	l := m.AddFleetListener()
	m.RemoveFleetListener(l)

	if _, aberto := <-l.C; aberto {
		t.Error("canal continua aberto depois de RemoveFleetListener")
	}
	m.NotifyChanged(1) // não pode entrar em pânico com listener removido
}
