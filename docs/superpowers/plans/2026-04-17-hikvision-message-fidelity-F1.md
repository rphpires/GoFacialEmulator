# Hikvision Message Fidelity — F1 Bug Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship phase F1 of the Hikvision message-fidelity audit — isolated bug fixes that unblock the Python client's `__get_events` loop (the `xml.parsers.expat.ExpatError: no element found` error stops).

**Architecture:** Edit `internal/emulator/hikvision/handlers.go` and `internal/emulator/hikvision/emulator.go` in place. Fixes: (1) make the multipart push include `Content-Disposition: form-data; name="event_log"` on the JSON part; (2) rewrite `handleGetDateTime` to return the real `<Time>` XML (current handler returns `<DeviceInfo>` body wrapped by Gin's `c.XML(string)` double-encoding); (3) make `handleSetDateTime` return `ResponseStatus` XML via the existing `writeHikvisionXML` helper; (4) change the alertStream `Content-Type` from `multipart/x-mixed-replace` to `multipart/mixed` and drop the `Cache-Control: no-cache` line (the real device sends neither); (5) remove the immediate initial heartbeat so the first body byte arrives only when the 10 s heartbeat or the event interval fires; (6) replace three stray `c.String(200, "OK")` success responses with `ResponseStatus` XML. Two small extractions (`buildPushEventMultipart`, `buildAlertStreamResponseHead`) isolate the pieces that need byte-exact assertions.

**Tech Stack:** Go 1.21+, Gin, `net/http/httptest`, `net.Pipe`. Reference pcap captures live in `aux_files/Emulator hikvision test/` (stream_57, stream_77, stream_80 are the ones this phase verifies against).

---

## File Structure

Files created:

- `internal/emulator/hikvision/test_helpers_test.go` — shared test scaffolding: `TestMain` that suppresses tracer file IO, and `newTestEmulator` helper that constructs a minimal `*Emulator` usable by handler tests.
- `internal/emulator/hikvision/emulator_multipart_test.go` — unit tests for the extracted `buildPushEventMultipart` (push event body) and `buildAlertStreamResponseHead` (alertStream HTTP headers).
- `internal/emulator/hikvision/handlers_system_test.go` — unit tests for `handleGetDateTime`, `handleSetDateTime`, `handleCommandOutput`, `handlePostFingerprintSetup`, `handlePutFingerprintDelete` using `httptest.NewRecorder` + a bare `gin.Engine`.
- `internal/emulator/hikvision/event_stream_test.go` — test for `handleEventStream` that asserts no bytes are written in the first 200 ms (initial heartbeat removed).

Files modified:

- `internal/emulator/hikvision/emulator.go`
  - Extract `buildPushEventMultipart(event *Event, imageData []byte, boundary string) []byte` out of `sendEventToRemoteServer` (lines ~330–354).
  - Add `Content-Disposition: form-data; name="event_log"` line to the JSON part.
  - Delete the initial-heartbeat block (lines ~595–608) from `handleEventStream`.
- `internal/emulator/hikvision/handlers.go`
  - Rewrite `handleGetDateTime` (lines 925–931).
  - Rewrite `handleSetDateTime` (lines 933–935).
  - Extract `buildAlertStreamResponseHead(now time.Time) string` from `handleGetAlertStream` (lines 1103–1109); flip Content-Type to `multipart/mixed`; drop Cache-Control line.
  - Replace `c.String(200, "OK")` with `writeHikvisionXML(...)` in `handleCommandOutput` (970–974), `handlePostFingerprintSetup` (671–673), `handlePutFingerprintDelete` (816 inside 794–817).

---

## Task 1: Package-level test scaffolding

**Files:**
- Create: `internal/emulator/hikvision/test_helpers_test.go`

The singleton `trace.NewTracer()` writes to `traces/trace.log` unless it finds a disable marker in the working directory (see `internal/trace/tracer.go:69`). Tests in this package need a lightweight `*Emulator` and we do not want them spamming the repo's `traces/` folder. Drop `DisableTrace.txt` before any test runs, then construct an Emulator with only the tracer populated (the handlers we test in F1 do not touch `repo` or `device.ID`).

- [ ] **Step 1: Write the scaffolding file**

```go
// internal/emulator/hikvision/test_helpers_test.go
package hikvision

import (
	"os"
	"testing"

	"GoFacialEmulator/internal/trace"
)

// TestMain disables tracer file IO before any test in this package runs.
// trace.NewTracer() is a sync.Once singleton; if it initializes without
// the marker file it opens traces/trace.log under the package working dir.
func TestMain(m *testing.M) {
	_ = os.WriteFile("DisableTrace.txt", []byte(""), 0644)
	defer os.Remove("DisableTrace.txt")
	os.Exit(m.Run())
}

// newTestEmulator returns a minimal *Emulator suitable for handler unit
// tests. Tests that need repo access must set their own fields; this helper
// only guarantees a valid tracer so methods that log do not nil-panic.
func newTestEmulator(t *testing.T) *Emulator {
	t.Helper()
	return &Emulator{
		tracer:   trace.NewTracer(),
		stopChan: make(chan struct{}),
	}
}
```

- [ ] **Step 2: Add a smoke test to prove the helper compiles and runs**

Append to `test_helpers_test.go`:

```go
func TestNewTestEmulator_Smoke(t *testing.T) {
	e := newTestEmulator(t)
	if e.tracer == nil {
		t.Fatal("expected non-nil tracer")
	}
	e.tracer.Info("smoke-test log line from newTestEmulator")
}
```

- [ ] **Step 3: Run the smoke test**

Run: `go test ./internal/emulator/hikvision/ -run TestNewTestEmulator_Smoke -v`
Expected: `--- PASS: TestNewTestEmulator_Smoke` and no file created under `traces/` relative to the test working directory.

- [ ] **Step 4: Commit**

```bash
git add internal/emulator/hikvision/test_helpers_test.go
git commit -m "test(hikvision): add TestMain scaffolding and newTestEmulator helper"
```

---

## Task 2: Extract push multipart builder and add Content-Disposition (spec F1 item 1)

**Files:**
- Modify: `internal/emulator/hikvision/emulator.go` (function `sendEventToRemoteServer`, lines 317–395)
- Test: `internal/emulator/hikvision/emulator_multipart_test.go` (new)

**Why:** The real device's push (`POST /w-access`, stream_80) has the JSON part with `Content-Disposition: form-data; name="event_log"`. The emulator's `sendEventToRemoteServer` currently writes only `Content-Type: application/json; charset="UTF-8"` on that part (emulator.go:337). The Python client parses the multipart as form-data and looks for the `event_log` field; without the header it treats the JSON as an opaque blob and later dies in the XML parser when it expects an XML-shaped response elsewhere in the flow. We extract the builder so we can assert its bytes without spinning up an HTTP server.

- [ ] **Step 1: Write the failing test**

Create `internal/emulator/hikvision/emulator_multipart_test.go`:

```go
package hikvision

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildPushEventMultipart_HasContentDispositionOnJSONPart(t *testing.T) {
	event := &Event{
		IPAddress: "192.168.1.81",
		PortNo:    15501,
		DateTime:  "2026-04-17T00:00:00-03:00",
		EventType: "AccessControllerEvent",
	}
	imageData := []byte("fake-jpeg-bytes")

	body := buildPushEventMultipart(event, imageData, "MIME_boundary")

	wantHeader := "Content-Disposition: form-data; name=\"event_log\""
	if !bytes.Contains(body, []byte(wantHeader)) {
		t.Errorf("missing %q in body:\n%s", wantHeader, body)
	}
	if !bytes.Contains(body, []byte("Content-Type: application/json; charset=\"UTF-8\"")) {
		t.Errorf("missing JSON Content-Type in body:\n%s", body)
	}
	if !bytes.Contains(body, []byte("Content-Disposition: form-data; name=\"Picture\"")) {
		t.Errorf("missing Picture Content-Disposition in body:\n%s", body)
	}
	if !bytes.Contains(body, []byte("--MIME_boundary--")) {
		t.Errorf("missing terminal boundary in body:\n%s", body)
	}
	// event_log header must come before the Picture header (first part ordering)
	eventIdx := bytes.Index(body, []byte("name=\"event_log\""))
	picIdx := bytes.Index(body, []byte("name=\"Picture\""))
	if eventIdx < 0 || picIdx < 0 || eventIdx > picIdx {
		t.Errorf("expected event_log part before Picture part (event=%d, pic=%d)", eventIdx, picIdx)
	}
	// Sanity: JSON body contains ipAddress
	if !strings.Contains(string(body), "\"ipAddress\": \"192.168.1.81\"") {
		t.Errorf("expected marshaled ipAddress in body:\n%s", body)
	}
}
```

- [ ] **Step 2: Run the test and verify it fails because `buildPushEventMultipart` does not exist**

Run: `go test ./internal/emulator/hikvision/ -run TestBuildPushEventMultipart -v`
Expected: compile error `undefined: buildPushEventMultipart`.

- [ ] **Step 3: Extract the builder in `emulator.go`**

In `internal/emulator/hikvision/emulator.go`, above `sendEventToRemoteServer` (currently line 317), add:

```go
// buildPushEventMultipart assembles the POST /w-access body the emulator
// sends to the remote server. The first part carries the JSON event with
// Content-Disposition: form-data; name="event_log" so the Python client
// parses it as the event_log form field (matches real device stream_80).
func buildPushEventMultipart(event *Event, imageData []byte, boundary string) []byte {
	eventJSON, _ := json.MarshalIndent(event, "", "  ")

	var body strings.Builder

	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Disposition: form-data; name=\"event_log\"\r\n")
	body.WriteString("Content-Type: application/json; charset=\"UTF-8\"\r\n")
	body.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(eventJSON)))
	body.WriteString("\r\n")
	body.Write(eventJSON)
	body.WriteString("\r\n")

	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Disposition: form-data; name=\"Picture\"; filename=\"Picture.jpg\"\r\n")
	body.WriteString("Content-Type: image/jpeg\r\n")
	body.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(imageData)))
	body.WriteString("Content-ID: pictureImage\r\n")
	body.WriteString("\r\n")
	body.Write(imageData)
	body.WriteString("\r\n")

	body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	return []byte(body.String())
}
```

- [ ] **Step 4: Replace the body-building block in `sendEventToRemoteServer`**

Replace the block that currently starts at line 319 (`eventJSON, err := json.MarshalIndent(event, "", "  ")`) through line 354 (`body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))`) with:

```go
	imageData, err := GetPhotoImageData()
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	boundary := "MIME_boundary"
	body := buildPushEventMultipart(event, imageData, boundary)
```

Then update the `http.NewRequest` call a few lines later from `strings.NewReader(body.String())` to `bytes.NewReader(body)`. Add `"bytes"` to the import block at the top of the file.

Final shape of `sendEventToRemoteServer` after edit (everything outside the replaced block is unchanged):

```go
func (e *Emulator) sendEventToRemoteServer(event *Event) error {
	imageData, err := GetPhotoImageData()
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	boundary := "MIME_boundary"
	body := buildPushEventMultipart(event, imageData, boundary)

	server, _ := e.repo.GetSetting("RemoteServer")
	port, _ := e.repo.GetSetting("RemotePort")
	path, _ := e.repo.GetSetting("RemoteURL")
	remoteURL := fmt.Sprintf("http://%s:%s%s", server, port, path)
	e.tracer.Info("Sending event to server: %s", remoteURL)

	req, err := http.NewRequest("POST", remoteURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", boundary))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %s", resp.Status)
	}
	return nil
}
```

(Leave the `multipart/x-mixed-replace` Content-Type on this outbound request as-is — F4 handles payload whitespace; this task only adds the disposition header and extracts the builder.)

- [ ] **Step 5: Run the test and verify it passes**

Run: `go test ./internal/emulator/hikvision/ -run TestBuildPushEventMultipart -v`
Expected: `--- PASS`.

- [ ] **Step 6: Run the whole package to catch regressions**

Run: `go test ./internal/emulator/hikvision/ -v`
Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/emulator/hikvision/emulator.go internal/emulator/hikvision/emulator_multipart_test.go
git commit -m "fix(hikvision): push multipart JSON part includes Content-Disposition event_log"
```

---

## Task 3: Rewrite handleGetDateTime to return real `<Time>` XML (spec F1 item 2)

**Files:**
- Modify: `internal/emulator/hikvision/handlers.go` (lines 925–931)
- Test: `internal/emulator/hikvision/handlers_system_test.go` (new)

**Why:** Current code calls `c.XML(http.StatusOK, "<?xml...<DeviceInfo>...")` passing a *string*. Gin's `c.XML` marshals the value via `encoding/xml`, so a string gets wrapped in `<string>...</string>` producing `<string>&lt;?xml...&lt;DeviceInfo&gt;...&lt;/DeviceInfo&gt;</string>` — exactly the garbage that breaks the client's `xml.parsers.expat` with `no element found: line 1, column 0`. Also the body shape is wrong (`<DeviceInfo>` instead of `<Time>`). Reference: real device `GET /ISAPI/System/time` returns a `<Time>` document with `timeMode`, `localTime`, `timeZone` (see the PUT body in stream_57 and Hikvision ISAPI docs).

- [ ] **Step 1: Write the failing test**

Create `internal/emulator/hikvision/handlers_system_test.go`:

```go
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
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/emulator/hikvision/ -run TestHandleGetDateTime -v`
Expected: FAIL. Body contains `<string>` wrapping and `<DeviceInfo>` element.

- [ ] **Step 3: Rewrite the handler**

In `internal/emulator/hikvision/handlers.go`, replace lines 925–931 (`handleGetDateTime` body) with:

```go
func (e *Emulator) handleGetDateTime(c *gin.Context) {
	e.tracer.Info("Polling message received")
	now := time.Now().Format("2006-01-02T15:04:05-07:00")
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Time version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
<timeMode>manual</timeMode>
<localTime>%s</localTime>
<timeZone>CST+3:00:00</timeZone>
</Time>
`, now)
	c.Header("Content-Type", "application/xml")
	c.String(http.StatusOK, body)
}
```

`fmt` and `time` are already imported in this file (see existing `handleGetDeviceInfo` at line 937 and `handleGetStatus` at line 127).

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/emulator/hikvision/ -run TestHandleGetDateTime -v`
Expected: `--- PASS`.

- [ ] **Step 5: Run the whole package**

Run: `go test ./internal/emulator/hikvision/ -v`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/emulator/hikvision/handlers.go internal/emulator/hikvision/handlers_system_test.go
git commit -m "fix(hikvision): GET /System/time returns <Time> XML (no Gin double-encoding)"
```

---

## Task 4: Rewrite handleSetDateTime to return ResponseStatus XML (spec F1 item 3)

**Files:**
- Modify: `internal/emulator/hikvision/handlers.go` (lines 933–935)
- Test: `internal/emulator/hikvision/handlers_system_test.go` (append)

**Why:** Real device responds `PUT /ISAPI/System/time` with `ResponseStatus` XML (see stream_57 lines 74–80). Emulator returns `c.String(200, "OK")`. Plain-text body breaks the client when it tries `xmltodict.parse` on the response. Use the existing `writeHikvisionXML` helper (handlers.go:1063).

- [ ] **Step 1: Write the failing test**

Append to `internal/emulator/hikvision/handlers_system_test.go`:

```go
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
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/emulator/hikvision/ -run TestHandleSetDateTime -v`
Expected: FAIL. Body is `OK`.

- [ ] **Step 3: Rewrite the handler**

In `internal/emulator/hikvision/handlers.go`, replace lines 933–935 with:

```go
func (e *Emulator) handleSetDateTime(c *gin.Context) {
	writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/emulator/hikvision/ -run TestHandleSetDateTime -v`
Expected: `--- PASS`.

- [ ] **Step 5: Run the whole package**

Run: `go test ./internal/emulator/hikvision/ -v`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/emulator/hikvision/handlers.go internal/emulator/hikvision/handlers_system_test.go
git commit -m "fix(hikvision): PUT /System/time returns ResponseStatus XML"
```

---

## Task 5: Fix alertStream response headers — Content-Type + drop Cache-Control (spec F1 items 4 and 6)

**Files:**
- Modify: `internal/emulator/hikvision/handlers.go` (`handleGetAlertStream`, lines 1103–1109)
- Test: `internal/emulator/hikvision/emulator_multipart_test.go` (append)

**Why:** Real device (stream_77) sends `Content-Type: multipart/mixed; boundary=MIME_boundary` without `Cache-Control`. Emulator currently sends `multipart/x-mixed-replace` + `Cache-Control: no-cache`. `x-mixed-replace` has "replace previous part" semantics (from Netscape push); the Python client expects the `multipart/mixed` RFC 2046 variant and its parser branches on this header. We extract a tiny pure function to assert the header bytes without dealing with connection hijacking in tests.

- [ ] **Step 1: Write the failing test**

Append to `internal/emulator/hikvision/emulator_multipart_test.go`:

```go
import (
	"strings"
	"time"
)

func TestBuildAlertStreamResponseHead_UsesMultipartMixed(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	head := buildAlertStreamResponseHead(now)

	if !strings.HasPrefix(head, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("head must start with HTTP/1.1 200 OK\\r\\n; got:\n%s", head)
	}
	if !strings.Contains(head, "Content-Type: multipart/mixed; boundary=MIME_boundary\r\n") {
		t.Errorf("expected multipart/mixed; got:\n%s", head)
	}
	if strings.Contains(head, "multipart/x-mixed-replace") {
		t.Errorf("legacy x-mixed-replace Content-Type still present:\n%s", head)
	}
	if strings.Contains(head, "Cache-Control") {
		t.Errorf("Cache-Control must be dropped to match real device; got:\n%s", head)
	}
	if !strings.HasSuffix(head, "\r\n\r\n") {
		t.Errorf("head must terminate with blank line; got:\n%q", head)
	}
	if !strings.Contains(head, "Date: ") {
		t.Errorf("missing Date header in head:\n%s", head)
	}
}
```

(Make sure the `strings` and `time` imports are present. If the file already imports them from Task 2, do not re-add.)

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/emulator/hikvision/ -run TestBuildAlertStreamResponseHead -v`
Expected: compile error `undefined: buildAlertStreamResponseHead`.

- [ ] **Step 3: Add the helper and update the handler**

In `internal/emulator/hikvision/handlers.go`, above `handleGetAlertStream` (currently line 1076), add:

```go
// buildAlertStreamResponseHead returns the raw HTTP response status line +
// headers the emulator writes into the hijacked socket when the client
// opens GET /ISAPI/Event/notification/alertStream. Matches the real
// DS-K1T673DX-BR (stream_77): multipart/mixed and no Cache-Control.
func buildAlertStreamResponseHead(now time.Time) string {
	return "HTTP/1.1 200 OK\r\n" +
		"Date: " + now.UTC().Format(http.TimeFormat) + "\r\n" +
		"Server: App-webs/\r\n" +
		"Content-Type: multipart/mixed; boundary=MIME_boundary\r\n" +
		"Connection: keep-alive\r\n" +
		"\r\n"
}
```

Then inside `handleGetAlertStream`, replace the block starting at `httpResponse := "HTTP/1.1 200 OK\r\n" +` (currently line 1103) through the closing `"\r\n"` (line 1109) with:

```go
	httpResponse := buildAlertStreamResponseHead(time.Now())
```

Leave the subsequent `bufrw.WriteString(httpResponse)` / `Flush` lines untouched.

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/emulator/hikvision/ -run TestBuildAlertStreamResponseHead -v`
Expected: `--- PASS`.

- [ ] **Step 5: Run the whole package**

Run: `go test ./internal/emulator/hikvision/ -v`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/emulator/hikvision/handlers.go internal/emulator/hikvision/emulator_multipart_test.go
git commit -m "fix(hikvision): alertStream uses multipart/mixed, drops Cache-Control"
```

---

## Task 6: Remove immediate initial heartbeat from handleEventStream (spec F1 item 5)

**Files:**
- Modify: `internal/emulator/hikvision/emulator.go` (`handleEventStream`, lines 595–608)
- Test: `internal/emulator/hikvision/event_stream_test.go` (new)

**Why:** The real device does not send any body byte until the first actual heartbeat or event fires. The emulator currently writes a heartbeat immediately after the headers. With the header fix in Task 5 the client's multipart parser starts looking for parts *before* the first part is ready; an immediate heartbeat with the legacy Content-Disposition also drags us further from the real device. Remove it; rely on the existing 10 s heartbeat loop already downstream.

The test uses `net.Pipe()` to give the handler a real `net.Conn` + `*bufio.ReadWriter`, starts the stream in a goroutine, then reads with a short deadline on the peer side. With the initial heartbeat gone, the read must time out (no bytes in 200 ms). The 10 s heartbeat interval and the event-generation interval (we leave `e.device.EventInterval = 0`) guarantee the loop will not fire inside the test window.

- [ ] **Step 1: Write the failing test**

Create `internal/emulator/hikvision/event_stream_test.go`:

```go
package hikvision

import (
	"bufio"
	"errors"
	"net"
	"testing"
	"time"
)

func TestHandleEventStream_NoBytesBeforeFirstInterval(t *testing.T) {
	e := newTestEmulator(t)
	// EventInterval 0 means the event branch never fires; 10 s heartbeat
	// also will not fire inside the 200 ms test window. Any byte we
	// observe must therefore be from the removed initial heartbeat.

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	bufrw := bufio.NewReadWriter(
		bufio.NewReader(serverSide),
		bufio.NewWriter(serverSide),
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.handleEventStream(serverSide, bufrw)
	}()

	if err := clientSide.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 1)
	n, err := clientSide.Read(buf)

	if n > 0 {
		t.Errorf("expected no bytes in 200ms (initial heartbeat removed), got %d", n)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("expected read timeout, got n=%d err=%v", n, err)
	}

	// Cleanly stop the goroutine.
	close(e.stopChan)
	serverSide.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("handleEventStream goroutine did not exit after close")
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/emulator/hikvision/ -run TestHandleEventStream_NoBytesBeforeFirstInterval -v -timeout 30s`
Expected: FAIL — reads the initial heartbeat byte immediately, so `n > 0` and no timeout.

- [ ] **Step 3: Delete the initial-heartbeat block**

In `internal/emulator/hikvision/emulator.go`, inside `handleEventStream`, delete the block at lines 595–608 — from the comment `// Heartbeat inicial imediato: o cliente Python bloqueia em iter_content()` through the closing `e.tracer.Info("[alertStream] initial heartbeat sent (%d bytes)", n)` line.

After the edit, the function body opens with:

```go
func (e *Emulator) handleEventStream(conn net.Conn, bufrw *bufio.ReadWriter) {
	e.tracer.Info("[GET] /alertStream - Starting event stream")

	heartbeatCounter := time.Now()
	generatedEventCounter := time.Now()

	// Loop principal de streaming. Falhas de escrita indicam que o cliente
	// desconectou (equivalente a c.Request.Context().Done()).
	for {
		select {
		case <-e.stopChan:
			// … (unchanged)
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/emulator/hikvision/ -run TestHandleEventStream_NoBytesBeforeFirstInterval -v -timeout 30s`
Expected: `--- PASS`.

- [ ] **Step 5: Run the whole package**

Run: `go test ./internal/emulator/hikvision/ -v`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/emulator/hikvision/emulator.go internal/emulator/hikvision/event_stream_test.go
git commit -m "fix(hikvision): alertStream no longer sends immediate initial heartbeat"
```

---

## Task 7: Replace plain-text "OK" responses in three handlers (spec F1 item 7)

**Files:**
- Modify: `internal/emulator/hikvision/handlers.go`
  - `handlePostFingerprintSetup` (lines 671–673)
  - `handlePutFingerprintDelete` (line 816 inside 794–817)
  - `handleCommandOutput` (lines 970–974)
- Test: `internal/emulator/hikvision/handlers_system_test.go` (append)

**Why:** Same root issue as Task 4 — plain-text success bodies break any client that `xmltodict.parse`s the response. Three handlers still return `c.String(http.StatusOK, "OK")`; replace with `writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")` so every success response is the canonical `ResponseStatus` XML.

- [ ] **Step 1: Write failing tests for all three handlers**

Append to `internal/emulator/hikvision/handlers_system_test.go`:

```go
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
```

Note: `handlePutFingerprintDelete` touches `e.repo` (calls `e.repo.DeleteFace`) so a unit test of its success path would need a real or faked repository. For F1 scope we verify the replacement *only* at the source-code level with a grep-style assertion (cheap, adequate). Add one more test:

```go
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
		// Grab roughly the body: until next "\nfunc " or EOF.
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
```

Add `"os"` to the imports of `handlers_system_test.go` if not already present.

- [ ] **Step 2: Run the tests, verify they fail**

Run: `go test ./internal/emulator/hikvision/ -run "TestHandleCommandOutput|TestHandlePostFingerprintSetup|TestHandlersNoLongerReturnPlainOK" -v`
Expected: FAIL.

- [ ] **Step 3: Fix `handlePostFingerprintSetup`**

In `internal/emulator/hikvision/handlers.go`, replace the body of `handlePostFingerprintSetup` (lines 671–673):

```go
func (e *Emulator) handlePostFingerprintSetup(c *gin.Context) {
	writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")
}
```

- [ ] **Step 4: Fix `handlePutFingerprintDelete`**

In `internal/emulator/hikvision/handlers.go`, replace the `c.String(http.StatusOK, "OK")` on line 816 (inside `handlePutFingerprintDelete`) with:

```go
	writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")
```

Nothing else in that function changes.

- [ ] **Step 5: Fix `handleCommandOutput`**

In `internal/emulator/hikvision/handlers.go`, replace the body of `handleCommandOutput` (lines 970–974):

```go
func (e *Emulator) handleCommandOutput(c *gin.Context) {
	outputID := c.Param("output_id")
	e.tracer.Info("Receiving command for output: %s", outputID)
	writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")
}
```

- [ ] **Step 6: Run the tests, verify they pass**

Run: `go test ./internal/emulator/hikvision/ -run "TestHandleCommandOutput|TestHandlePostFingerprintSetup|TestHandlersNoLongerReturnPlainOK" -v`
Expected: all `--- PASS`.

- [ ] **Step 7: Run the whole package**

Run: `go test ./internal/emulator/hikvision/ -v`
Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add internal/emulator/hikvision/handlers.go internal/emulator/hikvision/handlers_system_test.go
git commit -m "fix(hikvision): three OK handlers now return ResponseStatus XML"
```

---

## Final verification

- [ ] **Full build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Full test suite**

Run: `go test ./...`
Expected: all packages pass (at minimum the hikvision package; the `testes/` directory is a `package main` helper and will be skipped by `go test` — ignore it).

- [ ] **Manual end-to-end validation against the Python client**

1. Build the emulator: `go build -o bin/emulator ./cmd/emulator-service`.
2. Start it against the configured DB (`./bin/emulator`) and connect the user's Python client (`IoHikvisionCommunication`) to it.
3. Confirm in the client log:
   - `xml.parsers.expat.ExpatError: no element found` no longer appears.
   - `__get_events` proceeds past the polling phase and begins processing event payloads.
   - The device time-sync handshake completes (PUT `/ISAPI/System/time` → 200).
4. Leave the session running for at least one heartbeat cycle (≥10 s) and verify a heartbeat arrives via the alertStream. No client-side timeout should fire during that window.

If any check fails, the plan is not done — loop back to the failing task.

---

## What is deliberately NOT in F1

These belong to F2/F3/F4 and must not creep into this plan's commits:

- `PUT /ISAPI/AccessControl/remoteCheck` — F2 (gets its own plan).
- `parameterFormatType` persistence on httpHosts + `?format=json` negotiation on capabilities — F3.
- `ShortSerialNumber`, `TurnstileTurned`, `LicensePlateNo` struct fields + indentation change + removing `Content-Disposition` from the *alertStream* (not the push) part — F4.
- Any change to the inbound `multipart/x-mixed-replace` Content-Type on the *outbound* push request (emulator.go:368) — out of scope; the spec calls this ok.
- Any refactor of the management endpoints (`UserInfo/*`, `CardInfo/*`, `FDLib/*`, `FingerPrint*` beyond the two plain-text fixes in Task 7).
