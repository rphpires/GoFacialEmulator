package dahua

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"GoFacialEmulator/internal/trace"

	"github.com/gin-gonic/gin"
)

func TestEventDataDetails_OptionalFieldsOmittedWhenUnset(t *testing.T) {
	d := EventDataDetails{ReaderID: "1", UserID: "10"}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	for _, proibido := range []string{"QRCodeStr", "TrafficCar", "CollectResult"} {
		if strings.Contains(s, proibido) {
			t.Errorf("%s nao pode aparecer num evento comum: %s", proibido, s)
		}
	}
}

func TestEventDataDetails_QRCodeAndPlateSerialize(t *testing.T) {
	verdadeiro := true
	d := EventDataDetails{
		ReaderID:      "1",
		Method:        14, // EventMethod.QRCODE
		QRCodeStr:     "ABC123456",
		TrafficCar:    &TrafficCar{PlateNumber: "ABC1D23"},
		CollectResult: &verdadeiro,
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	// extract_card_number_from_event le exatamente estas chaves.
	if !strings.Contains(s, `"QRCodeStr":"ABC123456"`) {
		t.Errorf("QRCodeStr mal serializado: %s", s)
	}
	if !strings.Contains(s, `"TrafficCar":{"PlateNumber":"ABC1D23"}`) {
		t.Errorf("TrafficCar mal serializado: %s", s)
	}
	if !strings.Contains(s, `"CollectResult":true`) {
		t.Errorf("CollectResult mal serializado: %s", s)
	}
}

func TestEventDataDetails_CollectResultFalseIsExplicit(t *testing.T) {
	falso := false
	d := EventDataDetails{CollectResult: &falso}
	raw, _ := json.Marshal(d)
	// Ponteiro, e nao bool, justamente para que "coleta falhou" nao suma do
	// JSON por causa do omitempty.
	if !strings.Contains(string(raw), `"CollectResult":false`) {
		t.Errorf("CollectResult=false precisa aparecer: %s", raw)
	}
}

func newRecordTestEmulator() *Emulator {
	return &Emulator{
		tracer:   trace.NewTracer(),
		stopChan: make(chan struct{}),
		eventLog: newDahuaEventLog(10),
	}
}

func TestHandleRecordFinder_CardRecUsesEventLog(t *testing.T) {
	e := newRecordTestEmulator()
	agora := time.Now()
	e.eventLog.Append(CardRecEntry{
		CreateTime: agora.Add(-30 * time.Minute),
		CardNo:     "0A0B", UserID: 7, ReaderID: 1, Method: 15,
	})

	r := gin.New()
	r.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	url := fmt.Sprintf("/cgi-bin/recordFinder.cgi?action=find&name=AccessControlCardRec&StartTime=%d&EndTime=%d",
		agora.Add(-time.Hour).Unix(), agora.Unix())
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "found=1") || !strings.Contains(body, "records[0].CardNo=0A0B") {
		t.Errorf("evento offline nao veio do event log:\n%s", body)
	}
	if strings.Contains(body, "ValidDateStart") {
		t.Error("resposta traz campos de cartao; o handler continua ignorando name")
	}
}

func TestHandleRecordFinder_CardRecOutOfWindowIsEmpty(t *testing.T) {
	e := newRecordTestEmulator()
	e.eventLog.Append(CardRecEntry{CreateTime: time.Now().Add(-48 * time.Hour), CardNo: "0A0B"})

	r := gin.New()
	r.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	agora := time.Now()
	url := fmt.Sprintf("/cgi-bin/recordFinder.cgi?action=find&name=AccessControlCardRec&StartTime=%d&EndTime=%d",
		agora.Add(-time.Hour).Unix(), agora.Unix())
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "found=0") {
		t.Errorf("evento fora da janela nao pode aparecer:\n%s", w.Body.String())
	}
}

func TestHandleRecordFinder_GetQuerySizeCards(t *testing.T) {
	e := newRecordTestEmulator()
	e.countCardsFn = func() (int, error) { return 42, nil }

	r := gin.New()
	r.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	req := httptest.NewRequest(http.MethodGet, "/cgi-bin/recordFinder.cgi?action=getQuerySize&name=AccessControlCard", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// __get_device_cards_count aceita count=, size= ou recnum=.
	if !strings.Contains(w.Body.String(), "count=42") {
		t.Errorf("getQuerySize: got %q, want linha count=42", w.Body.String())
	}
}

func TestSetupRoutes_RegistersEventManager(t *testing.T) {
	e := newRecordTestEmulator()
	r := gin.New()
	e.SetupRoutes(r)

	achou := false
	for _, ri := range r.Routes() {
		if ri.Method == http.MethodGet && ri.Path == "/cgi-bin/eventManager.cgi" {
			achou = true
		}
	}
	if !achou {
		t.Error("GET /cgi-bin/eventManager.cgi nao registrado; standalone sem snapshot fica mudo")
	}
}

func TestHandleEventManager_RejectsUnknownAction(t *testing.T) {
	e := newRecordTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/eventManager.cgi", e.handleEventManager)

	req := httptest.NewRequest(http.MethodGet, "/cgi-bin/eventManager.cgi?action=detach", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

func TestHandleAccessControl_CaptureFingerprintAccepted(t *testing.T) {
	e := newRecordTestEmulator()
	var pedidos []string
	e.enrollFn = func(userID string) { pedidos = append(pedidos, userID) }

	r := gin.New()
	r.GET("/cgi-bin/accessControl.cgi", e.handleAccessControl)

	req := httptest.NewRequest(http.MethodGet,
		"/cgi-bin/accessControl.cgi?action=captureFingerprint&info.ReaderID=1&info.UserID=42&heartbeat=5&timeout=60", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "OK") {
		t.Errorf("resposta deve ser OK; got %q", w.Body.String())
	}
	if len(pedidos) != 1 || pedidos[0] != "42" {
		t.Errorf("enroll disparado com: %v, want [42]", pedidos)
	}
}

func TestBuildEnrollEvent_CarriesCollectResult(t *testing.T) {
	ev := buildEnrollEvent("00:11:22:33:44:55", "42", true)
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, `"CollectResult":true`) {
		t.Errorf("o gerenciador desvia o evento pela chave CollectResult: %s", s)
	}
	if !strings.Contains(s, `"UserID":"42"`) {
		t.Errorf("UserID ausente: %s", s)
	}
	if !strings.Contains(s, `"PhysicalAddress":"00:11:22:33:44:55"`) {
		t.Errorf("PhysicalAddress ausente: %s", s)
	}
}

func TestPushStreamEvent_DropsWhenNobodyIsListening(t *testing.T) {
	e := newRecordTestEmulator()
	e.pushChan = make(chan []byte, 1)

	// Primeiro cabe no buffer; o segundo tem de ser descartado sem bloquear.
	e.pushStreamEvent([]byte("um"))
	e.pushStreamEvent([]byte("dois"))

	if n := len(e.pushChan); n != 1 {
		t.Errorf("canal deveria ter exatamente 1 item; got %d", n)
	}
}

func TestBuildStreamTextPart_HasBoundaryAndLength(t *testing.T) {
	part := buildStreamTextPart([]byte(`{"a":1}`))
	s := string(part)
	if !strings.Contains(s, "--myboundary") {
		t.Errorf("falta o boundary: %q", s)
	}
	if !strings.Contains(s, "Content-Type: text/plain") {
		t.Errorf("falta o Content-Type: %q", s)
	}
	// parse_event_data fatia o corpo por Content-Length; sem ele o payload
	// binario/multilinha e truncado.
	if !strings.Contains(s, "Content-Length: 7") {
		t.Errorf("Content-Length errado: %q", s)
	}
}

// truncarCardName foi removido: o device real REJEITA um CardName longo
// demais, não trunca. Os testes de aceitação/rejeição do limite de 32 bytes
// (maxCardNameBytes) estão em handlers_cardface_test.go, junto com o resto
// da fidelidade de recordUpdater.cgi/AccessControlCard.
