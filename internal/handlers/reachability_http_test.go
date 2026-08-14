package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"GoFacialEmulator/internal/reachability"

	"github.com/gin-gonic/gin"
)

// TestGetReachability_ContratoHTTP fixa a forma do JSON que a tela de
// dispositivos consome. O aviso da interface lê "unreachable" e a lista
// "devices": mudar qualquer um dos dois quebra a tela em silêncio.
func TestGetReachability_ContratoHTTP(t *testing.T) {
	env := reachability.Environment{
		Kind:            reachability.KindDocker,
		PublishedRanges: []reachability.PortRange{{Start: 4000, End: 4099}},
		RangesKnown:     true,
	}
	portas := []reachability.DevicePort{
		{DeviceID: 1, Port: 4001, Started: true},
		{DeviceID: 2, Port: 4200, Started: true},
	}

	h := &Handler{env: env}
	r := gin.New()
	r.GET("/api/reachability", func(c *gin.Context) {
		c.JSON(http.StatusOK, reachability.Analyze(portas, h.env))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/reachability", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", w.Code)
	}

	var corpo struct {
		Unreachable int `json:"unreachable"`
		Unknown     int `json:"unknown"`
		Devices     []struct {
			DeviceID int    `json:"device_id"`
			Port     int    `json:"port"`
			Status   string `json:"status"`
			Reason   string `json:"reason"`
		} `json:"devices"`
		Environment struct {
			Kind int `json:"kind"`
		} `json:"environment"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("JSON inválido: %v — corpo: %s", err, w.Body.String())
	}

	if corpo.Unreachable != 1 {
		t.Errorf("unreachable = %d, quero 1", corpo.Unreachable)
	}
	if len(corpo.Devices) != 2 {
		t.Fatalf("len(devices) = %d, quero 2", len(corpo.Devices))
	}
	if corpo.Devices[0].Status != "ok" {
		t.Errorf("devices[0].status = %q, quero \"ok\"", corpo.Devices[0].Status)
	}
	if corpo.Devices[1].Status != "inalcancavel" {
		t.Errorf("devices[1].status = %q, quero \"inalcancavel\"", corpo.Devices[1].Status)
	}
	if corpo.Devices[1].Reason == "" {
		t.Error("devices[1].reason vazio, quero a explicação em português")
	}
}
