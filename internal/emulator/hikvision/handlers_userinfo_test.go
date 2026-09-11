package hikvision

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// O gateway escreve utilizadores por PUT /ISAPI/AccessControl/UserInfo/SetUp
// (upsert). Enquanto a rota não existiu, cada AddUser devolveu 404 e nenhum
// utilizador chegou ao emulador — mesmo com force-sync. Este teste falha se a
// rota voltar a desaparecer.
func TestSetupRoutes_RegistersUserWriteEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := newTestEmulator(t)
	router := gin.New()
	e.SetupRoutes(router)

	registered := map[string]bool{}
	for _, r := range router.Routes() {
		registered[r.Method+" "+r.Path] = true
	}

	want := []string{
		"PUT /ISAPI/AccessControl/UserInfo/SetUp",
		"POST /ISAPI/AccessControl/UserInfo/Record",
		"PUT /ISAPI/AccessControl/UserInfo/Modify",
	}
	for _, route := range want {
		if !registered[route] {
			t.Errorf("rota em falta: %s", route)
		}
	}
}
