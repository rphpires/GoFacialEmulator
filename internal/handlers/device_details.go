package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"GoFacialEmulator/internal/database"

	"github.com/gin-gonic/gin"
)

const (
	deviceUsersDefaultPerPage = 10
	deviceUsersMaxPerPage     = 100
)

// getDeviceUsers lista os usuarios gravados no dispositivo, em formato
// unificado entre Hikvision e Dahua. Le do banco, entao funciona mesmo com
// o emulador parado.
func (h *Handler) getDeviceUsers(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	device, err := h.manager.GetDevice(id)
	if err != nil {
		h.tracer.Error("Failed to get device %d: %v", id, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	page, perPage := parsePagination(c)
	search := strings.TrimSpace(c.Query("search"))

	users, total, err := database.ListDeviceUsers(
		context.Background(), h.manager.EmulatorDB, device.Model, id,
		search, perPage, (page-1)*perPage,
	)
	if err != nil {
		h.tracer.Error("Failed to list users of device %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users":    users,
		"total":    total,
		"page":     page,
		"per_page": perPage,
		"model":    device.Model,
	})
}

// getDeviceSettings lista as configuracoes gravadas para o dispositivo.
func (h *Handler) getDeviceSettings(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	if _, err := h.manager.GetDevice(id); err != nil {
		h.tracer.Error("Failed to get device %d: %v", id, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	settings, err := database.ListDeviceSettings(context.Background(), h.manager.EmulatorDB, id)
	if err != nil {
		h.tracer.Error("Failed to list settings of device %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"settings": settings})
}

// parsePagination extrai page/per_page da query string com limites seguros.
func parsePagination(c *gin.Context) (page, perPage int) {
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil || page < 1 {
		page = 1
	}

	perPage, err = strconv.Atoi(c.Query("per_page"))
	if err != nil || perPage < 1 {
		perPage = deviceUsersDefaultPerPage
	}
	if perPage > deviceUsersMaxPerPage {
		perPage = deviceUsersMaxPerPage
	}

	return page, perPage
}
