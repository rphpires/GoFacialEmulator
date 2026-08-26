package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"GoFacialEmulator/internal/database"

	"github.com/gin-gonic/gin"
)

// apiSetSyncEnabled liga ou desliga o vínculo com o Invenzi W-Access.
// Desligado, RefreshDevices recusa e o serviço passa a viver só dos
// emuladores cadastrados na mão.
func (h *Handler) apiSetSyncEnabled(c *gin.Context) {
	var corpo struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&corpo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := database.SetSyncEnabled(ctx, h.serviceDB, corpo.Enabled); err != nil {
		if errors.Is(err, database.ErrNoWxsSettings) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Configure a conexão com o W-Access antes de ligar a sincronização",
			})
			return
		}
		h.tracer.Error("Failed to set sync_enabled: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao gravar configuração"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"enabled": corpo.Enabled})
}
