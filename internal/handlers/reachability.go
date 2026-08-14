package handlers

import (
	"net/http"

	"GoFacialEmulator/internal/reachability"

	"github.com/gin-gonic/gin"
)

// getReachability responde quais dispositivos não vão ser alcançados pelo
// Site Controller neste ambiente. É a informação que falta hoje quando a
// tela mostra tudo verde e o W-Access mostra tudo offline.
func (h *Handler) getReachability(c *gin.Context) {
	dispositivos, err := h.manager.ListDevicesWithFilters(map[string]string{})
	if err != nil {
		h.tracer.Error("Failed to list devices for reachability: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Não foi possível verificar as portas dos dispositivos",
		})
		return
	}

	portas := make([]reachability.DevicePort, 0, len(dispositivos))
	for _, d := range dispositivos {
		portas = append(portas, reachability.DevicePort{
			DeviceID:  d.ID,
			Port:      d.Port,
			Started:   d.Status == "running",
			BindError: h.manager.LastStartError(d.ID),
		})
	}

	c.JSON(http.StatusOK, reachability.Analyze(portas, h.env))
}
