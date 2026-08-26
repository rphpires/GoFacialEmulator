package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"GoFacialEmulator/internal/emulator"

	"github.com/gin-gonic/gin"
)

// statusDoErro traduz erro de domínio em status HTTP. Concentrado numa
// função só para os cinco handlers responderem igual.
func statusDoErro(err error) int {
	switch {
	case errors.Is(err, emulator.ErrDeviceNotFound):
		return http.StatusNotFound
	case errors.Is(err, emulator.ErrDeviceIsManaged), errors.Is(err, emulator.ErrDeviceRunning):
		return http.StatusConflict
	case errors.Is(err, emulator.ErrInvalidSpec):
		// ConflictError também cai aqui: é erro de payload, não de estado.
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// responderErro escreve o corpo de erro. Conflito de porta ganha a lista
// crua além da mensagem, porque a UI destaca as portas problemáticas.
func (h *Handler) responderErro(c *gin.Context, err error) {
	status := statusDoErro(err)

	var conf *emulator.ConflictError
	if errors.As(err, &conf) {
		corpo := gin.H{"error": conf.Error()}
		if len(conf.Ports) > 0 {
			corpo["conflicts"] = conf.Ports
		}
		if len(conf.Reserved) > 0 {
			corpo["reserved"] = conf.Reserved
		}
		c.JSON(status, corpo)
		return
	}

	if status == http.StatusInternalServerError {
		h.tracer.Error("Emulator CRUD failed: %v", err)
		c.JSON(status, gin.H{"error": "Erro interno ao processar a operação"})
		return
	}

	c.JSON(status, gin.H{"error": err.Error()})
}

// apiListEmulators lista todos os dispositivos, com a origem de cada um.
func (h *Handler) apiListEmulators(c *gin.Context) {
	devices, err := h.manager.ListDevices()
	if err != nil {
		h.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"emulators": devices, "count": len(devices)})
}

// apiCreateEmulator cadastra um emulador manual.
func (h *Handler) apiCreateEmulator(c *gin.Context) {
	var spec emulator.DeviceSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dev, err := h.manager.CreateDevice(ctx, spec)
	if err != nil {
		h.responderErro(c, err)
		return
	}

	if spec.AutoStart {
		if err := h.manager.Start(dev.ID); err != nil {
			// O cadastro deu certo; só o start falhou. 201 com o aviso
			// dentro é mais honesto que 500 sobre um recurso criado.
			resposta := gin.H{"emulator": dev, "start_error": err.Error()}
			c.JSON(http.StatusCreated, resposta)
			return
		}
		// dev foi montado por CreateDevice antes do start, com status
		// "stopped" fixo. Sem esta linha, "started": true conviveria com
		// "emulator": {"status": "stopped"} na mesma resposta — dois campos
		// contando histórias diferentes sobre o mesmo recurso.
		dev.Status = "running"
		c.JSON(http.StatusCreated, gin.H{"emulator": dev, "started": true})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"emulator": dev})
}

// apiCreateEmulatorRange cadastra um emulador por porta do intervalo.
func (h *Handler) apiCreateEmulatorRange(c *gin.Context) {
	var spec emulator.RangeSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	// Lote grande com auto_start sobe centenas de servidores HTTP; o
	// timeout precisa acomodar isso.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	devices, err := h.manager.CreateDeviceRange(ctx, spec)
	if err != nil {
		h.responderErro(c, err)
		return
	}

	criados := make([]gin.H, 0, len(devices))
	for _, d := range devices {
		criados = append(criados, gin.H{"id": d.ID, "name": d.Name, "port": d.Port})
	}

	resposta := gin.H{"count": len(devices), "created": criados}

	if spec.AutoStart {
		iniciados := 0
		var falhas []gin.H
		for _, d := range devices {
			if err := h.manager.Start(d.ID); err != nil {
				falhas = append(falhas, gin.H{"id": d.ID, "error": err.Error()})
				continue
			}
			iniciados++
		}
		resposta["started"] = iniciados
		if len(falhas) > 0 {
			resposta["start_errors"] = falhas
		}
	}

	c.JSON(http.StatusCreated, resposta)
}

// apiUpdateEmulator substitui os campos editáveis de um emulador manual.
func (h *Handler) apiUpdateEmulator(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	var spec emulator.DeviceSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dev, err := h.manager.UpdateDevice(ctx, id, spec)
	if err != nil {
		h.responderErro(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"emulator": dev})
}

// apiDeleteEmulator remove um emulador manual e purga os dados dele.
func (h *Handler) apiDeleteEmulator(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := h.manager.DeleteDevice(ctx, id); err != nil {
		h.responderErro(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"removed": id})
}
