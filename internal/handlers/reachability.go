package handlers

import (
	"net/http"

	"GoFacialEmulator/internal/models"
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

	portas := portasDeDispositivos(dispositivos, h.manager.LastStartError)

	c.JSON(http.StatusOK, reachability.Analyze(portas, h.env))
}

// portasDeDispositivos traduz a lista de dispositivos para a entrada de
// reachability.Analyze. Fica separada do handler para poder ser testada sem
// Manager nem banco: é aqui que um erro de mapeamento passaria batido.
func portasDeDispositivos(dispositivos []*models.Device, ultimoErro func(int) string) []reachability.DevicePort {
	portas := make([]reachability.DevicePort, 0, len(dispositivos))
	for _, d := range dispositivos {
		portas = append(portas, reachability.DevicePort{
			DeviceID:  d.ID,
			Port:      d.Port,
			Started:   d.Status == "running",
			BindError: ultimoErro(d.ID),
		})
	}
	return portas
}
