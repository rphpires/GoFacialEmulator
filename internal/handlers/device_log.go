package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"GoFacialEmulator/internal/devicelog"

	"github.com/gin-gonic/gin"
)

// idDoLog lê o :id. O caminho do arquivo sai só deste número, nunca de texto
// vindo da requisição.
func idDoLog(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return 0, false
	}
	return id, true
}

// getDeviceLog serve o arquivo de log mais recente do dispositivo para abrir
// no navegador ("Ver log" na gaveta).
func (h *Handler) getDeviceLog(c *gin.Context) {
	id, ok := idDoLog(c)
	if !ok {
		return
	}
	f, err := os.Open(h.logs.ActivePath(id))
	if errors.Is(err, os.ErrNotExist) {
		c.String(http.StatusNotFound, "Log desligado ou ainda vazio para o dispositivo %d.", id)
		return
	}
	if err != nil {
		h.tracer.Error("Abrindo log do dispositivo %d: %v", id, err)
		c.String(http.StatusInternalServerError, "Não foi possível ler o log.")
		return
	}
	defer f.Close()

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="device-%d.log"`, id))
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, f); err != nil {
		h.tracer.Warning("Enviando log do dispositivo %d: %v", id, err)
	}
}

// getDeviceLogZip devolve um zip com todos os arquivos de log do dispositivo.
func (h *Handler) getDeviceLogZip(c *gin.Context) {
	id, ok := idDoLog(c)
	if !ok {
		return
	}
	var buf bytes.Buffer
	err := h.logs.WriteZip(id, &buf)
	if errors.Is(err, devicelog.ErrNoLogs) {
		c.String(http.StatusNotFound, "Nenhum log para o dispositivo %d.", id)
		return
	}
	if err != nil {
		h.tracer.Error("Montando zip de logs do dispositivo %d: %v", id, err)
		c.String(http.StatusInternalServerError, "Não foi possível montar o zip.")
		return
	}

	nome := fmt.Sprintf("device-%d-logs-%s.zip", id, time.Now().Format("20060102-1504"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, nome))
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}
