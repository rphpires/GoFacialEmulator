package hikvision

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestBuildDoorEvent_UsesISAPIShape(t *testing.T) {
	e := newTestEmulator(t)
	e.macAddress = "bc:5e:33:57:a5:cb"

	ev := e.buildDoorEvent(MinorDoorOpenNormal)

	if ev.EventType != "AccessControllerEvent" {
		t.Errorf("eventType: got %q, want AccessControllerEvent", ev.EventType)
	}
	if ev.MacAddress != "bc:5e:33:57:a5:cb" {
		t.Errorf("macAddress: got %q, want the emulator MAC", ev.MacAddress)
	}
	if ev.DateTime == "" {
		t.Error("dateTime must be set; the SC parses it with strptime ISO8601 + offset")
	}
	if ev.AccessControllerEvent.MajorEventType != 5 {
		t.Errorf("majorEventType: got %d, want 5", ev.AccessControllerEvent.MajorEventType)
	}
	if ev.AccessControllerEvent.SubEventType != MinorDoorOpenNormal {
		t.Errorf("subEventType: got %d, want %d", ev.AccessControllerEvent.SubEventType, MinorDoorOpenNormal)
	}
	if ev.AccessControllerEvent.CardNo != "" {
		t.Errorf("cardNo must be empty on a door event; got %q", ev.AccessControllerEvent.CardNo)
	}
	if ev.AccessControllerEvent.FrontSerialNo == nil {
		t.Error("frontSerialNo must be present on a door event (it is already decided)")
	}

	// O corpo tem de ser serializável e conter os campos que o SC lê primeiro.
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]interface{}
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"dateTime", "macAddress", "AccessControllerEvent"} {
		if _, ok := back[k]; !ok {
			t.Errorf("campo %q ausente no JSON do evento", k)
		}
	}
	if _, ok := back["Events"]; ok {
		t.Error("JSON traz a chave Dahua \"Events\"; o formato Dahua regrediu")
	}
}

func TestBuildDoorEvent_SerialNoIsMonotonic(t *testing.T) {
	e := newTestEmulator(t)

	primeiro := e.buildDoorEvent(MinorDoorOpenNormal)
	segundo := e.buildDoorEvent(MinorDoorCloseNormal)

	if segundo.AccessControllerEvent.SerialNo <= primeiro.AccessControllerEvent.SerialNo {
		t.Errorf("serialNo deve crescer: %d depois de %d",
			segundo.AccessControllerEvent.SerialNo, primeiro.AccessControllerEvent.SerialNo)
	}
	if *segundo.AccessControllerEvent.FrontSerialNo != segundo.AccessControllerEvent.SerialNo-1 {
		t.Errorf("frontSerialNo deve ser o serialNo anterior: front=%d serial=%d",
			*segundo.AccessControllerEvent.FrontSerialNo, segundo.AccessControllerEvent.SerialNo)
	}
}

func TestHandlePutCardDelete_ParsesEmployeeNoList(t *testing.T) {
	e := newTestEmulator(t)
	var apagados []string
	e.deleteCardsFn = func(employeeNo string) (int, error) {
		apagados = append(apagados, employeeNo)
		return 1, nil
	}

	r := gin.New()
	r.PUT("/ISAPI/AccessControl/CardInfo/Delete", e.handlePutCardDelete)

	body := `{"CardInfoDelCond":{"EmployeeNoList":[{"employeeNo":"1001"},{"employeeNo":"1002"}]}}`
	req := httptest.NewRequest(http.MethodPut, "/ISAPI/AccessControl/CardInfo/Delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(apagados) != 2 || apagados[0] != "1001" || apagados[1] != "1002" {
		t.Errorf("employeeNos apagados: got %v, want [1001 1002]", apagados)
	}
	if !strings.Contains(w.Body.String(), "<statusCode>1</statusCode>") {
		t.Errorf("resposta deve ser ResponseStatus de sucesso; got:\n%s", w.Body.String())
	}
}

func TestHandlePutCardDelete_RejectsGarbage(t *testing.T) {
	e := newTestEmulator(t)
	e.deleteCardsFn = func(string) (int, error) { return 0, nil }

	r := gin.New()
	r.PUT("/ISAPI/AccessControl/CardInfo/Delete", e.handlePutCardDelete)

	req := httptest.NewRequest(http.MethodPut, "/ISAPI/AccessControl/CardInfo/Delete", strings.NewReader("nao-e-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

func TestHandlePutFingerPrintDelete_ParsesCondition(t *testing.T) {
	e := newTestEmulator(t)
	var apagados []string
	e.deleteFingersFn = func(employeeNo string) (int, error) {
		apagados = append(apagados, employeeNo)
		return 2, nil
	}

	r := gin.New()
	r.PUT("/ISAPI/AccessControl/FingerPrint/Delete", e.handlePutFingerPrintDelete)

	body := `{"FingerPrintDelete":{"mode":"byEmployeeNo","EmployeeNoDetail":{"employeeNo":"1001"}}}`
	req := httptest.NewRequest(http.MethodPut, "/ISAPI/AccessControl/FingerPrint/Delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(apagados) != 1 || apagados[0] != "1001" {
		t.Errorf("employeeNos apagados: got %v, want [1001]", apagados)
	}
	if !strings.Contains(w.Body.String(), "<statusCode>1</statusCode>") {
		t.Errorf("resposta deve ser ResponseStatus de sucesso; got:\n%s", w.Body.String())
	}
}

func TestHandlePutFingerPrintDelete_AcceptsList(t *testing.T) {
	e := newTestEmulator(t)
	var apagados []string
	e.deleteFingersFn = func(employeeNo string) (int, error) {
		apagados = append(apagados, employeeNo)
		return 1, nil
	}

	r := gin.New()
	r.PUT("/ISAPI/AccessControl/FingerPrint/Delete", e.handlePutFingerPrintDelete)

	body := `{"FingerPrintDelete":{"EmployeeNoList":[{"employeeNo":"7"},{"employeeNo":"8"}]}}`
	req := httptest.NewRequest(http.MethodPut, "/ISAPI/AccessControl/FingerPrint/Delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if len(apagados) != 2 {
		t.Errorf("apagados: got %v, want dois employeeNos", apagados)
	}
}

func TestHandlePostCaptureFingerPrint_ReturnsFingerData(t *testing.T) {
	e := newTestEmulator(t)
	r := gin.New()
	r.POST("/ISAPI/AccessControl/CaptureFingerPrint", e.handlePostCaptureFingerPrint)

	body := `<CaptureFingerPrintCond version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
    <fingerNo>3</fingerNo>
</CaptureFingerPrintCond>`
	req := httptest.NewRequest(http.MethodPost, "/ISAPI/AccessControl/CaptureFingerPrint", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("Content-Type: got %q, want application/xml", ct)
	}
	got := w.Body.String()
	// xmltodict.parse(reply.content)["CaptureFingerPrint"]["fingerData"]
	if !strings.Contains(got, "<CaptureFingerPrint") {
		t.Errorf("falta o elemento raiz CaptureFingerPrint:\n%s", got)
	}
	if !strings.Contains(got, "<fingerNo>3</fingerNo>") {
		t.Errorf("fingerNo deve ecoar o pedido:\n%s", got)
	}
	if !strings.Contains(got, "<fingerData>") {
		t.Errorf("falta fingerData, o unico campo que o gerenciador le:\n%s", got)
	}
	if !strings.Contains(got, "<fingerPrintQuality>") {
		t.Errorf("falta fingerPrintQuality:\n%s", got)
	}
}

func TestHandlePostAcsEvent_ReturnsStoredEvents(t *testing.T) {
	e := newTestEmulator(t)
	e.eventLog = newEventLog(10)
	base := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	e.eventLog.Append(AcsEventRecord{
		Time: base, Major: 5, Minor: MinorFaceVerifyPass,
		CardNo: "12345", Name: "Fulano", EmployeeNo: "1001", CardReaderNo: 1, DoorNo: 1, SerialNo: 10,
	})

	r := gin.New()
	r.POST("/ISAPI/AccessControl/AcsEvent", e.handlePostAcsEvent)

	body := `{"AcsEventCond":{"searchID":"x","searchResultPosition":0,"maxResults":30,"major":0,"minor":0}}`
	req := httptest.NewRequest(http.MethodPost, "/ISAPI/AccessControl/AcsEvent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var resp struct {
		AcsEvent struct {
			ResponseStatusStrg string `json:"responseStatusStrg"`
			NumOfMatches       int    `json:"numOfMatches"`
			TotalMatches       int    `json:"totalMatches"`
			InfoList           []struct {
				Major        int    `json:"major"`
				Minor        int    `json:"minor"`
				CardNo       string `json:"cardNo"`
				CardReaderNo int    `json:"cardReaderNo"`
				Time         string `json:"time"`
			} `json:"InfoList"`
		} `json:"AcsEvent"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
	}
	if resp.AcsEvent.ResponseStatusStrg != "OK" {
		t.Errorf("responseStatusStrg: got %q, want OK", resp.AcsEvent.ResponseStatusStrg)
	}
	if len(resp.AcsEvent.InfoList) != 1 {
		t.Fatalf("InfoList: got %d itens, want 1", len(resp.AcsEvent.InfoList))
	}
	it := resp.AcsEvent.InfoList[0]
	if it.Major != 5 || it.Minor != MinorFaceVerifyPass || it.CardNo != "12345" || it.CardReaderNo != 1 {
		t.Errorf("evento mal serializado: %+v", it)
	}
	// datetime.strptime(str(time).replace("T"," ")[:-6], "%Y-%m-%d %H:%M:%S")
	if len(it.Time) < 25 || it.Time[10] != 'T' {
		t.Errorf("time deve ser ISO8601 com offset; got %q", it.Time)
	}
}

func TestHandlePostAcsEvent_EmptyLogReturnsNoMatch(t *testing.T) {
	e := newTestEmulator(t)
	e.eventLog = newEventLog(10)

	r := gin.New()
	r.POST("/ISAPI/AccessControl/AcsEvent", e.handlePostAcsEvent)

	body := `{"AcsEventCond":{"searchResultPosition":0,"maxResults":30}}`
	req := httptest.NewRequest(http.MethodPost, "/ISAPI/AccessControl/AcsEvent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), `"NO MATCH"`) {
		t.Errorf("log vazio deve responder NO MATCH; got:\n%s", w.Body.String())
	}
}

func TestHandlePutStorageCfg_PurgesByCheckTime(t *testing.T) {
	e := newTestEmulator(t)
	e.eventLog = newEventLog(10)
	e.eventLog.Append(AcsEventRecord{Time: time.Now().Add(-2 * time.Hour)})
	e.eventLog.Append(AcsEventRecord{Time: time.Now().Add(time.Hour)})

	r := gin.New()
	r.PUT("/ISAPI/AccessControl/AcsEvent/StorageCfg", e.handlePutStorageCfg)

	corte := time.Now().Format("2006-01-02 15:04:05")
	body := `{"EventStorageCfg":{"mode":"time","checkTime":"` + corte + `"}}`
	req := httptest.NewRequest(http.MethodPut, "/ISAPI/AccessControl/AcsEvent/StorageCfg", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if _, total := e.eventLog.Page(0, 10); total != 1 {
		t.Errorf("apos a purga deveria restar 1 evento; got %d", total)
	}
}

func TestHandlePutStorageCfg_RegularModeKeepsEvents(t *testing.T) {
	e := newTestEmulator(t)
	e.eventLog = newEventLog(10)
	e.eventLog.Append(AcsEventRecord{Time: time.Now().Add(-2 * time.Hour)})

	r := gin.New()
	r.PUT("/ISAPI/AccessControl/AcsEvent/StorageCfg", e.handlePutStorageCfg)

	body := `{"EventStorageCfg":{"mode":"regular","period":10}}`
	req := httptest.NewRequest(http.MethodPut, "/ISAPI/AccessControl/AcsEvent/StorageCfg", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if _, total := e.eventLog.Page(0, 10); total != 1 {
		t.Errorf("mode=regular nao pode apagar nada; restaram %d", total)
	}
}

// Regressao encontrada no deploy 2026-08-26: o container roda em UTC e o
// gerenciador manda checkTime em horario local (-03). Lido com time.Local a
// janela saia 3h deslocada e o StorageCfg nunca apagava nada.
func TestHandlePutStorageCfg_CheckTimeUsaFusoDoDispositivo(t *testing.T) {
	e := newTestEmulator(t)
	e.eventLog = newEventLog(10)

	loc := fusoDoDispositivo()
	agora := time.Now().In(loc)

	// Evento de 10 minutos atras, carimbado como o emulador carimba.
	e.eventLog.Append(AcsEventRecord{Time: agora.Add(-10 * time.Minute)})

	r := gin.New()
	r.PUT("/ISAPI/AccessControl/AcsEvent/StorageCfg", e.handlePutStorageCfg)

	// checkTime = agora, escrito no fuso do dispositivo e SEM offset, que e o
	// formato que datetime.now().strftime() do gerenciador produz.
	corte := agora.Format("2006-01-02 15:04:05")
	body := `{"EventStorageCfg":{"mode":"time","checkTime":"` + corte + `"}}`
	req := httptest.NewRequest(http.MethodPut, "/ISAPI/AccessControl/AcsEvent/StorageCfg", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if _, total := e.eventLog.Page(0, 10); total != 0 {
		t.Errorf("evento anterior ao checkTime deveria ter sido purgado; restaram %d", total)
	}
}
