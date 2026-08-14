package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"GoFacialEmulator/internal/models"
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
			Kind string `json:"kind"`
		} `json:"environment"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("JSON inválido: %v — corpo: %s", err, w.Body.String())
	}

	if corpo.Environment.Kind != "docker" {
		t.Errorf("environment.kind = %q, quero \"docker\"", corpo.Environment.Kind)
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

// TestPortasDeDispositivos cobre o mapeamento de models.Device para
// reachability.DevicePort, que é a parte que TestGetReachability_ContratoHTTP
// não alcança: aquele teste monta o DevicePort na mão e só fixa os nomes dos
// campos do JSON. Aqui é onde um Status errado ou um erro de bind
// grudado no dispositivo errado seria pego.
func TestPortasDeDispositivos(t *testing.T) {
	dispositivos := []*models.Device{
		{ID: 1, Port: 4001, Status: "running"},
		{ID: 2, Port: 4002, Status: "stopped"},
		{ID: 3, Port: 4003, Status: "running"},
	}

	// Só o dispositivo 3 tem erro de bind registrado — prova que o erro
	// não vaza para os outros dois.
	ultimoErro := func(deviceID int) string {
		if deviceID == 3 {
			return "address already in use"
		}
		return ""
	}

	portas := portasDeDispositivos(dispositivos, ultimoErro)

	if len(portas) != 3 {
		t.Fatalf("len(portas) = %d, quero 3", len(portas))
	}

	if !portas[0].Started {
		t.Errorf("dispositivo 1 (running): Started = false, quero true")
	}
	if portas[0].BindError != "" {
		t.Errorf("dispositivo 1: BindError = %q, quero vazio", portas[0].BindError)
	}

	if portas[1].Started {
		t.Errorf("dispositivo 2 (stopped): Started = true, quero false")
	}

	if portas[2].DeviceID != 3 {
		t.Fatalf("portas[2].DeviceID = %d, quero 3", portas[2].DeviceID)
	}
	if portas[2].BindError != "address already in use" {
		t.Errorf("dispositivo 3: BindError = %q, quero \"address already in use\"", portas[2].BindError)
	}
	if portas[0].BindError != "" || portas[1].BindError != "" {
		t.Error("o erro de bind do dispositivo 3 vazou para outro dispositivo")
	}
}
