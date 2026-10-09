package emulator

import (
	"sync"
	"sync/atomic"
)

// FleetEventKind diz ao consumidor o que reler.
type FleetEventKind string

const (
	// FleetChanged: algum campo do dispositivo mudou (estado, contagem de
	// usuários, cadastro, log, modo) ou ele acabou de ser criado.
	FleetChanged FleetEventKind = "changed"
	// FleetRemoved: o dispositivo deixou de existir.
	FleetRemoved FleetEventKind = "removed"
	// FleetResync: mudança em massa (sync com o W-Access); o consumidor
	// deve reler a frota inteira em vez de dispositivo a dispositivo.
	FleetResync FleetEventKind = "resync"
)

// FleetEvent é o aviso de que algo na frota mudou. Não carrega o
// dispositivo: quem consome relê do banco, que é a única fonte de verdade,
// e assim vários avisos seguidos custam uma leitura só.
type FleetEvent struct {
	Kind     FleetEventKind `json:"kind"`
	DeviceID int            `json:"device_id"`
}

// fleetListenerBuffer comporta um StartAll inteiro de uma frota grande
// sem descartar. Se ainda assim encher, o listener fica marcado e o
// consumidor faz uma releitura completa — perder um aviso em silêncio era
// o motivo de a tela precisar de F5.
const fleetListenerBuffer = 1024

// FleetListener é uma assinatura dos eventos da frota.
type FleetListener struct {
	C        chan FleetEvent
	overflow atomic.Bool
}

// TakeOverflow informa se algum evento foi descartado desde a última
// chamada, e zera a marca.
func (l *FleetListener) TakeOverflow() bool {
	return l.overflow.Swap(false)
}

// fleetHub distribui eventos para os listeners. O envio é síncrono e
// não-bloqueante: a ordem de chegada é a ordem em que as mudanças
// aconteceram (a versão anterior disparava uma goroutine por aviso, e um
// "stopped" podia chegar antes do "running" que o precedeu).
type fleetHub struct {
	mu        sync.RWMutex
	listeners []*FleetListener
}

func (h *fleetHub) add() *FleetListener {
	l := &FleetListener{C: make(chan FleetEvent, fleetListenerBuffer)}
	h.mu.Lock()
	h.listeners = append(h.listeners, l)
	h.mu.Unlock()
	return l
}

func (h *fleetHub) remove(l *FleetListener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, atual := range h.listeners {
		if atual == l {
			close(l.C)
			h.listeners = append(h.listeners[:i], h.listeners[i+1:]...)
			return
		}
	}
}

func (h *fleetHub) publish(ev FleetEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, l := range h.listeners {
		select {
		case l.C <- ev:
		default:
			l.overflow.Store(true)
		}
	}
}

// AddFleetListener assina os eventos da frota. Quem assina precisa chamar
// RemoveFleetListener ao terminar.
func (m *Manager) AddFleetListener() *FleetListener {
	return m.hub.add()
}

// RemoveFleetListener encerra a assinatura e fecha o canal.
func (m *Manager) RemoveFleetListener(l *FleetListener) {
	m.hub.remove(l)
}

// NotifyChanged avisa que o dispositivo mudou. Exportado para mudanças
// gravadas fora do Manager (modo do dispositivo, no handler).
func (m *Manager) NotifyChanged(deviceID int) {
	m.hub.publish(FleetEvent{Kind: FleetChanged, DeviceID: deviceID})
}

func (m *Manager) notifyRemoved(deviceID int) {
	m.hub.publish(FleetEvent{Kind: FleetRemoved, DeviceID: deviceID})
}

func (m *Manager) notifyResync() {
	m.hub.publish(FleetEvent{Kind: FleetResync})
}
