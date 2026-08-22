package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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

// deviceView é a forma de um dispositivo no wire do SSE. Espelha as
// colunas que a tabela mostra, para o cliente conseguir redesenhar uma
// linha inteira sem uma segunda requisição.
type deviceView struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Model      string `json:"model"`
	Port       int    `json:"port"`
	Status     string `json:"status"`
	Enabled    int    `json:"enabled"`
	LogEnabled int    `json:"log_enabled"`
	Interval   int    `json:"interval"`
	TotalUsers int    `json:"total_users"`
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
		Port:       d.Port,
		Status:     status,
		Enabled:    d.Enabled,
		LogEnabled: d.LogEnabled,
		Interval:   d.EventInterval,
		TotalUsers: d.TotalUsers,
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

// handleStream serve /events.
//
// Três coisas que a versão anterior não fazia e que apareciam como "o
// tempo real não funciona":
//
//  1. Snapshot no connect. Antes o stream só falava em mudança de estado,
//     então uma aba aberta depois de uma transição ficava com o HTML
//     server-rendered antigo por tempo indeterminado.
//  2. Keepalive. Sem tráfego, um proxy derruba a conexão ociosa; o browser
//     reconecta, mas o ciclo se repetia sem ninguém perceber.
//  3. retry:. Sem ele o browser usa o default próprio, que varia entre
//     implementações.
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

	listener := h.manager.AddStatusListener()
	defer h.manager.RemoveStatusListener(listener)

	if err := h.writeSnapshot(c.Writer); err != nil {
		h.tracer.Error("SSE: falha ao enviar snapshot: %v", err)
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(streamKeepalive)
	defer ticker.Stop()

	clientGone := c.Request.Context().Done()

	for {
		select {
		case event, ok := <-listener:
			if !ok {
				return
			}

			devices, err := h.manager.ListDevices()
			if err != nil {
				h.tracer.Error("SSE: falha ao listar dispositivos: %v", err)
				continue
			}

			payload := gin.H{
				"device_id": event.DeviceID,
				"status":    event.Status,
				"counts":    countFleet(devices),
			}

			// A linha inteira acompanha o evento, para o cliente atualizar
			// contadores de usuários e flags sem uma segunda requisição.
			for _, d := range devices {
				if d.ID == event.DeviceID {
					payload["device"] = newDeviceView(d)
					break
				}
			}

			if err := writeSSE(c.Writer, "device", payload); err != nil {
				return
			}
			flusher.Flush()

		case <-ticker.C:
			// Comentário SSE: mantém a conexão quente e é ignorado pelo
			// EventSource.
			if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()

		case <-clientGone:
			return
		}
	}
}

// writeSnapshot manda a frota inteira, para o cliente partir de um estado
// conhecido em vez de confiar no que veio no HTML.
func (h *Handler) writeSnapshot(w io.Writer) error {
	devices, err := h.manager.ListDevices()
	if err != nil {
		return err
	}

	views := make([]deviceView, 0, len(devices))
	for _, d := range devices {
		views = append(views, newDeviceView(d))
	}

	return writeSSE(w, "snapshot", gin.H{
		"devices": views,
		"counts":  countFleet(devices),
	})
}
