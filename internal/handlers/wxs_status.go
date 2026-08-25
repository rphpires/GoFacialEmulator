package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"GoFacialEmulator/internal/database"

	"github.com/gin-gonic/gin"
)

// O sino vermelho do rail consulta este endpoint em toda página. Testar a
// conexão de verdade abre um pool novo no SQL Server e roda uma query de
// validação — caro demais para repetir a cada navegação. O resultado fica
// em cache curto; ?force=1 (usado depois de salvar/testar) o invalida.
const wxsStatusTTL = 30 * time.Second

type wxsStatus struct {
	ok        bool
	erro      string
	aferidoEm time.Time
}

var (
	wxsStatusMu    sync.Mutex
	wxsStatusCache wxsStatus
)

// getWxsStatus informa se as credenciais gravadas do W-Access conectam.
//
// Sempre responde 200: falha de conexão é o estado que a UI quer desenhar,
// não um erro de transporte.
func (h *Handler) getWxsStatus(c *gin.Context) {
	forcar := c.Query("force") == "1"

	wxsStatusMu.Lock()
	cache := wxsStatusCache
	wxsStatusMu.Unlock()

	if !forcar && !cache.aferidoEm.IsZero() && time.Since(cache.aferidoEm) < wxsStatusTTL {
		c.JSON(http.StatusOK, gin.H{"ok": cache.ok, "error": cache.erro, "cached": true})
		return
	}

	ok, erro := h.aferirWxs()

	wxsStatusMu.Lock()
	wxsStatusCache = wxsStatus{ok: ok, erro: erro, aferidoEm: time.Now()}
	wxsStatusMu.Unlock()

	c.JSON(http.StatusOK, gin.H{"ok": ok, "error": erro, "cached": false})
}

// aferirWxs carrega as credenciais gravadas e tenta usá-las de fato.
func (h *Handler) aferirWxs() (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	settings, err := database.GetWxsSettingsFromDB(ctx, h.serviceDB)
	if err != nil {
		h.tracer.Error("WXS status: failed to load settings: %v", err)
		return false, "Não foi possível ler as configurações gravadas."
	}
	if settings == nil || settings.Host == "" {
		return false, "Conexão com o W-Access ainda não configurada."
	}

	if err := database.TestWxsConnection(settings); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// invalidarWxsStatus zera o cache para que a próxima consulta afira de novo.
func invalidarWxsStatus() {
	wxsStatusMu.Lock()
	wxsStatusCache = wxsStatus{}
	wxsStatusMu.Unlock()
}
