package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4"
)

// Os dois modos de operação de um dispositivo, gravados em
// emulator.device_settings, chave LocalAuthentication:
//
//	online     (0) — o Site Controller valida o acesso e responde
//	standalone (1) — o dispositivo valida sozinho e gera o evento
const (
	modoOnline     = "online"
	modoStandalone = "standalone"
)

// chaveModo é a cfg_id usada em emulator.device_settings. O mesmo nome que
// dahua/repository.go lê e grava.
const chaveModo = "LocalAuthentication"

// errModoInvalido separa erro do cliente (modo desconhecido) de falha de
// infraestrutura. Sem essa distinção, um banco fora do ar seria reportado
// como erro de quem clicou.
var errModoInvalido = errors.New("modo inválido")

func modoParaBanco(modo string) (string, error) {
	switch modo {
	case modoOnline:
		return "0", nil
	case modoStandalone:
		return "1", nil
	default:
		return "", fmt.Errorf("%w: %q", errModoInvalido, modo)
	}
}

func bancoParaModo(valor string) string {
	if valor == "1" {
		return modoStandalone
	}
	return modoOnline
}

// getDeviceMode lê o modo atual de um dispositivo em
// emulator.device_settings.
func (h *Handler) getDeviceMode(ctx context.Context, deviceID int) (string, error) {
	var valor string
	err := h.serviceDB.QueryRow(ctx,
		`SELECT value FROM emulator.device_settings
		  WHERE device_id = $1 AND cfg_id = $2`, deviceID, chaveModo).Scan(&valor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Sem linha, o emulador trata o dispositivo como standalone —
			// mesmo padrão de Repository.GetSetting. A tela precisa dizer a
			// mesma coisa que o emulador faz.
			return modoStandalone, nil
		}
		return "", err
	}
	return bancoParaModo(valor), nil
}

// getDeviceModes lê o modo de vários dispositivos numa consulta só. A
// alternativa — uma consulta por dispositivo dentro do laço da listagem —
// seriam centenas de idas ao banco a cada carregamento de página.
// Dispositivo sem linha não aparece no mapa; quem chama usa o padrão
// standalone, o mesmo recuo de getDeviceMode.
func (h *Handler) getDeviceModes(ctx context.Context, ids []int32) (map[int]string, error) {
	modos := make(map[int]string, len(ids))
	if len(ids) == 0 {
		return modos, nil
	}

	rows, err := h.serviceDB.Query(ctx,
		`SELECT device_id, value FROM emulator.device_settings
		  WHERE cfg_id = $1 AND device_id = ANY($2)`, chaveModo, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var valor string
		if err := rows.Scan(&id, &valor); err != nil {
			return nil, err
		}
		modos[id] = bancoParaModo(valor)
	}
	return modos, rows.Err()
}

// setDeviceMode grava o modo de um dispositivo em
// emulator.device_settings.
func (h *Handler) setDeviceMode(ctx context.Context, deviceID int, modo string) error {
	valor, err := modoParaBanco(modo)
	if err != nil {
		return err
	}

	// A linha normalmente não existe: o seed da migration é device_id = 0,
	// global, e leitura por dispositivo nunca casa com ela. UPDATE puro não
	// gravaria nada. Mesmo UPSERT de Repository.SetSetting.
	_, err = h.serviceDB.Exec(ctx,
		`INSERT INTO emulator.device_settings (device_id, cfg_id, value)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (device_id, cfg_id) DO UPDATE SET value = $3, updated_at = NOW()`,
		deviceID, chaveModo, valor)
	return err
}

// apiGetDeviceMode responde o modo atual do dispositivo.
func (h *Handler) apiGetDeviceMode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id inválido"})
		return
	}

	modo, err := h.getDeviceMode(c.Request.Context(), id)
	if err != nil {
		h.tracer.Error("Failed to read device mode for %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Não foi possível ler o modo do dispositivo"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": modo})
}

// apiSetDeviceMode troca o modo do dispositivo.
func (h *Handler) apiSetDeviceMode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id inválido"})
		return
	}

	var corpo struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&corpo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "corpo inválido"})
		return
	}

	if err := h.setDeviceMode(c.Request.Context(), id, corpo.Mode); err != nil {
		if errors.Is(err, errModoInvalido) {
			c.JSON(http.StatusBadRequest, gin.H{"error": `Modo inválido. Use "online" ou "standalone".`})
			return
		}
		h.tracer.Error("Failed to set device mode for %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Não foi possível trocar o modo do dispositivo"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": corpo.Mode})
}
