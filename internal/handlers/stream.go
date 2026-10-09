package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"GoFacialEmulator/internal/emulator"
	"GoFacialEmulator/internal/models"

	"github.com/gin-gonic/gin"
)

// streamKeepalive é o intervalo do comentário que mantém a conexão viva.
// Proxies costumam derrubar conexões ociosas em 60s; 20s dá margem de
// três batidas antes disso.
const streamKeepalive = 20 * time.Second

// streamRetry é o que o browser espera antes de reconectar sozinho, em
// milissegundos. Enviado uma vez, na abertura do stream.
const streamRetry = 3000

// streamCoalesce é a janela em que avisos seguidos viram um frame só. Um
// "Iniciar todos" gera um aviso por emulador; sem a janela, cada aviso
// custava uma leitura da frota inteira por aba aberta.
const streamCoalesce = 150 * time.Millisecond

// deviceView é a forma de um dispositivo no wire do SSE. Carrega tudo o
// que a tabela mostra, para o cliente desenhar uma linha inteira — inclusive
// uma linha nova — sem uma segunda requisição.
type deviceView struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Model      string `json:"model"`
	IPAddress  string `json:"ip_address"`
	Port       int    `json:"port"`
	Status     string `json:"status"`
	Enabled    int    `json:"enabled"`
	LogEnabled int    `json:"log_enabled"`
	Interval   int    `json:"interval"`
	TotalUsers int    `json:"total_users"`
	Source     string `json:"source"`
	Mode       string `json:"mode"`
	LastError  string `json:"last_error,omitempty"`
}

// newDeviceView aplica a mesma regra da tabela: Enabled == 0 vence o
// Status gravado. Manter essa decisão num lugar só evita o header e a
// tabela discordarem, que era exatamente o sintoma antigo.
func newDeviceView(d models.Device) deviceView {
	status := d.Status
	if d.Enabled == 0 {
		status = "disabled"
	}

	return deviceView{
		ID:         d.ID,
		Name:       d.Name,
		Model:      d.Model,
		IPAddress:  d.IPAddress,
		Port:       d.Port,
		Status:     status,
		Enabled:    d.Enabled,
		LogEnabled: d.LogEnabled,
		Interval:   d.EventInterval,
		TotalUsers: d.TotalUsers,
		Source:     d.Source,
		Mode:       modoStandalone,
	}
}

// writeSSE serializa um frame SSE. O JSON vai em linha única porque uma
// quebra dentro de "data:" encerraria o frame no meio.
func writeSSE(w io.Writer, event string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal SSE payload: %w", err)
	}

	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}

// pendencias acumula os avisos de uma janela de agregação.
type pendencias struct {
	alterados map[int]bool
	removidos map[int]bool
	resync    bool
}

func novasPendencias() *pendencias {
	return &pendencias{alterados: map[int]bool{}, removidos: map[int]bool{}}
}

func (p *pendencias) vazia() bool {
	return !p.resync && len(p.alterados) == 0 && len(p.removidos) == 0
}

// registrar incorpora um aviso. Remoção depois de alteração vence, e
// vice-versa: vale o último que aconteceu.
func (p *pendencias) registrar(ev emulator.FleetEvent) {
	switch ev.Kind {
	case emulator.FleetResync:
		p.resync = true
	case emulator.FleetRemoved:
		delete(p.alterados, ev.DeviceID)
		p.removidos[ev.DeviceID] = true
	default:
		delete(p.removidos, ev.DeviceID)
		p.alterados[ev.DeviceID] = true
	}
}

// handleStream serve /events.
//
// Protocolo:
//
//	snapshot -> { devices: [...], counts }            estado completo
//	delta    -> { devices: [...], removed: [ids], counts }
//
// O snapshot vai na abertura, depois de um sync com o W-Access e sempre
// que o canal deste cliente transbordou — nesse caso algum aviso se
// perdeu, e só a releitura completa garante que a tela não fica errada.
func (h *Handler) handleStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	// Desliga o buffer do nginx: com ele ligado, os frames ficam presos no
	// proxy e o stream chega em rajadas ou não chega.
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		h.tracer.Error("SSE: ResponseWriter não suporta Flush")
		c.Status(http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintf(c.Writer, "retry: %d\n\n", streamRetry); err != nil {
		return
	}

	// Assina antes do snapshot: um aviso que chegue entre os dois cai na
	// janela seguinte em vez de se perder.
	listener := h.manager.AddFleetListener()
	defer h.manager.RemoveFleetListener(listener)

	if err := h.writeSnapshot(c.Request.Context(), c.Writer); err != nil {
		h.tracer.Error("SSE: falha ao enviar snapshot: %v", err)
		return
	}
	flusher.Flush()

	keepalive := time.NewTicker(streamKeepalive)
	defer keepalive.Stop()

	// Timer parado até o primeiro aviso de uma janela.
	janela := time.NewTimer(time.Hour)
	janela.Stop()
	pend := novasPendencias()

	clientGone := c.Request.Context().Done()

	for {
		select {
		case ev, ok := <-listener.C:
			if !ok {
				return
			}
			if pend.vazia() {
				janela.Reset(streamCoalesce)
			}
			pend.registrar(ev)

		case <-janela.C:
			var err error
			if pend.resync || listener.TakeOverflow() {
				err = h.writeSnapshot(c.Request.Context(), c.Writer)
			} else {
				err = h.writeDelta(c.Request.Context(), c.Writer, pend)
			}
			pend = novasPendencias()
			if err != nil {
				h.tracer.Error("SSE: falha ao enviar atualização: %v", err)
				return
			}
			flusher.Flush()

		case <-keepalive.C:
			// Comentário SSE: mantém a conexão quente e é ignorado pelo
			// EventSource. Aproveita a batida para recuperar um overflow
			// que não tenha vindo acompanhado de nenhum aviso posterior.
			if listener.TakeOverflow() {
				if err := h.writeSnapshot(c.Request.Context(), c.Writer); err != nil {
					return
				}
			} else if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()

		case <-clientGone:
			return
		}
	}
}

// fleetViews lê a frota e monta as views com modo e último erro de start.
func (h *Handler) fleetViews(ctx context.Context) ([]deviceView, FleetCounts, error) {
	devices, err := h.manager.ListDevices()
	if err != nil {
		return nil, FleetCounts{}, err
	}

	ids := make([]int32, 0, len(devices))
	for _, d := range devices {
		ids = append(ids, int32(d.ID))
	}
	modos, err := h.getDeviceModes(ctx, ids)
	if err != nil {
		// Mesma decisão da listagem: coluna imprecisa é melhor que stream
		// morto.
		h.tracer.Error("SSE: falha ao ler modos: %v", err)
		modos = map[int]string{}
	}

	views := make([]deviceView, 0, len(devices))
	for _, d := range devices {
		v := newDeviceView(d)
		if modo, ok := modos[d.ID]; ok {
			v.Mode = modo
		}
		v.LastError = h.manager.LastStartError(d.ID)
		views = append(views, v)
	}
	return views, countFleet(devices), nil
}

// writeSnapshot manda a frota inteira, para o cliente partir de um estado
// conhecido em vez de confiar no que veio no HTML.
func (h *Handler) writeSnapshot(ctx context.Context, w io.Writer) error {
	views, counts, err := h.fleetViews(ctx)
	if err != nil {
		return err
	}

	return writeSSE(w, "snapshot", gin.H{
		"devices": views,
		"counts":  counts,
	})
}

// writeDelta manda só os dispositivos que mudaram na janela. Um ID
// alterado que não está mais na frota é tratado como removido.
func (h *Handler) writeDelta(ctx context.Context, w io.Writer, p *pendencias) error {
	views, counts, err := h.fleetViews(ctx)
	if err != nil {
		return err
	}

	mudaram := make([]deviceView, 0, len(p.alterados))
	presentes := make(map[int]bool, len(views))
	for _, v := range views {
		presentes[v.ID] = true
		if p.alterados[v.ID] {
			mudaram = append(mudaram, v)
		}
	}

	removidos := make([]int, 0, len(p.removidos))
	for id := range p.removidos {
		if !presentes[id] {
			removidos = append(removidos, id)
		}
	}
	for id := range p.alterados {
		if !presentes[id] {
			removidos = append(removidos, id)
		}
	}

	return writeSSE(w, "delta", gin.H{
		"devices": mudaram,
		"removed": removidos,
		"counts":  counts,
	})
}
