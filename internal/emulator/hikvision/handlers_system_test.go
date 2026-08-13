package hikvision

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

func assertResponseStatusOK(t *testing.T, body string) {
	t.Helper()
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
		t.Errorf("handler still returns plain OK:\n%s", body)
	}
}

func TestHandleCommandOutput_ReturnsResponseStatusXML(t *testing.T) {
	e := newTestEmulator(t)
	r := gin.New()
	r.PUT("/ISAPI/System/IO/outputs/:output_id/trigger", e.handleCommandOutput)

	req := httptest.NewRequest(http.MethodPut, "/ISAPI/System/IO/outputs/1/trigger", strings.NewReader(""))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("Content-Type: got %q, want application/xml", ct)
	}
	assertResponseStatusOK(t, w.Body.String())
}

func TestHandlePostFingerprintSetup_ReturnsResponseStatusXML(t *testing.T) {
	e := newTestEmulator(t)
	r := gin.New()
	r.POST("/ISAPI/AccessControl/FingerPrint/SetUp", e.handlePostFingerprintSetup)

	req := httptest.NewRequest(http.MethodPost, "/ISAPI/AccessControl/FingerPrint/SetUp", strings.NewReader(""))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("Content-Type: got %q, want application/xml", ct)
	}
	assertResponseStatusOK(t, w.Body.String())
}

func TestHandlersNoLongerReturnPlainOK(t *testing.T) {
	// Sentinel: none of the three fixed handlers should reference
	// c.String(http.StatusOK, "OK") after Task 7.
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("read handlers.go: %v", err)
	}
	src := string(b)
	for _, fn := range []string{
		"handleCommandOutput",
		"handlePostFingerprintSetup",
		"handlePutFingerprintDelete",
	} {
		start := strings.Index(src, "func (e *Emulator) "+fn+"(")
		if start < 0 {
			t.Fatalf("did not find %s in handlers.go", fn)
		}
		rest := src[start:]
		end := strings.Index(rest[1:], "\nfunc ")
		body := rest
		if end > 0 {
			body = rest[:end+1]
		}
		if strings.Contains(body, `c.String(http.StatusOK, "OK")`) {
			t.Errorf("%s still returns plain-text OK; must use writeHikvisionXML", fn)
		}
	}
}

func TestHandleGetAlertStream_HeadersMatchRealDevice(t *testing.T) {
	e := newTestEmulator(t)
	// Close stopChan BEFORE calling the handler so the event loop
	// exits on its first select iteration without writing events.
	close(e.stopChan)

	r := gin.New()
	r.GET("/ISAPI/Event/notification/alertStream", e.handleGetAlertStream)

	// httptest.NewRecorder() does not implement http.Hijacker, and the
	// handler hijacks the connection to write raw bytes. A real server is
	// required to exercise that path at all.
	srv := httptest.NewServer(r)
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// net/http's Client parses/normalizes headers when building an
	// http.Response, which can hide exactly what the hijacked connection
	// wrote to the wire. Speak raw HTTP instead so the assertion below
	// proves what actually left the socket, same as the real device.
	req := "GET /ISAPI/Event/notification/alertStream HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Connection: keep-alive\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write request: %v", err)
	}

	// The handler closes the connection once the event loop exits
	// (stopChan already closed above), so reading to EOF is safe and
	// the deadline is just a guard against the test hanging otherwise.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	raw, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	headerBlock, _, found := strings.Cut(string(raw), "\r\n\r\n")
	if !found {
		t.Fatalf("response has no header/body separator; got:\n%q", raw)
	}

	const wantHeaders = "HTTP/1.1 200 OK\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Connection: keep-alive\r\n" +
		"Content-Type: multipart/mixed; boundary=MIME_boundary"

	if headerBlock != wantHeaders {
		t.Errorf("headers: got %q, want %q", headerBlock, wantHeaders)
	}
	if strings.Contains(headerBlock, "Cache-Control") {
		t.Errorf("Cache-Control must be absent to match real device; got %q", headerBlock)
	}
	// Regression guard for the legacy "x-mixed-replace" value.
	if strings.Contains(headerBlock, "x-mixed-replace") {
		t.Errorf("legacy x-mixed-replace Content-Type still present: %q", headerBlock)
	}
	if !strings.Contains(headerBlock, "MIME-Version: 1.0") {
		t.Errorf("MIME-Version: 1.0 must be present to match real device; got %q", headerBlock)
	}
}
