package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestApiSetSyncEnabledRejeitaCorpoInvalido(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{serviceDB: &dbFalso{}, tracer: tracerDeTeste(t)}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/settings/sync",
		strings.NewReader(`{"enabled":"talvez"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.apiSetSyncEnabled(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, quero 400", rec.Code)
	}
}
