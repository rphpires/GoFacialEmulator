package hikvision

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func TestHandleGetDateTime_ReturnsTimeXML(t *testing.T) {
	e := newTestEmulator(t)
	r := gin.New()
	r.GET("/ISAPI/System/time", e.handleGetDateTime)

	req := httptest.NewRequest(http.MethodGet, "/ISAPI/System/time", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("Content-Type: got %q, want application/xml prefix", ct)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("body must start with XML prolog; got:\n%s", body)
	}
	if !strings.Contains(body, `<Time version="2.0"`) {
		t.Errorf("expected <Time version=\"2.0\"> element; got:\n%s", body)
	}
	for _, tag := range []string{"<timeMode>", "<localTime>", "<timeZone>"} {
		if !strings.Contains(body, tag) {
			t.Errorf("expected %s in body; got:\n%s", tag, body)
		}
	}
	// Guard against the double-encoding regression
	if strings.Contains(body, "<string>") {
		t.Errorf("body wraps content in <string>; Gin c.XML(string) double-encoding regressed:\n%s", body)
	}
	if strings.Contains(body, "<DeviceInfo") {
		t.Errorf("body contains legacy DeviceInfo element:\n%s", body)
	}
}

func TestHandleSetDateTime_ReturnsResponseStatusXML(t *testing.T) {
	e := newTestEmulator(t)
	r := gin.New()
	r.PUT("/ISAPI/System/time", e.handleSetDateTime)

	req := httptest.NewRequest(http.MethodPut, "/ISAPI/System/time", strings.NewReader("<Time/>"))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("Content-Type: got %q, want application/xml prefix", ct)
	}
	body := w.Body.String()
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<ResponseStatus `,
		`<statusCode>1</statusCode>`,
		`<statusString>OK</statusString>`,
		`<subStatusCode>ok</subStatusCode>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in body:\n%s", want, body)
		}
	}
	if strings.TrimSpace(body) == "OK" {
		t.Errorf("handler still returns plain OK; got body:\n%s", body)
	}
}
