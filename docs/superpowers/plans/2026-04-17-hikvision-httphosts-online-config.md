# Hikvision httpHosts PUT + Online Mode Config — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Hikvision emulator correctly parse and persist the `HttpHostNotificationList` XML sent by the client on PUT `/ISAPI/Event/notification/httpHosts`, respond with valid XML so the Python `xmltodict` client stops crashing, and use the per-device `ipAddress`/`portNo`/`url` received via PUT when sending online events.

**Architecture:** Per-device settings in `emulator.device_settings` table (existing). Three keys — `RemoteServer`, `RemotePort`, `RemoteURL` — written by the PUT handler and read on each outgoing online event. No in-memory cache of these values; the startup cache (`e.remoteServer`, `e.remotePort`, `e.remoteServerURL`) is removed.

**Tech Stack:** Go 1.x, Gin HTTP framework, PostgreSQL (via existing `DBInterface`), Go standard `encoding/xml`.

---

## Testing note (existing codebase convention)

The Hikvision package (`internal/emulator/hikvision`) has **no existing `_test.go` files**. The only Go test in the repo is `testes/db_test.go`. This plan adds one small unit test for the pure XML-parsing logic (no DB required) and relies on manual verification for the integration flow — consistent with how the rest of this package is validated today. Do **not** introduce a new test framework or mocking infra for this change.

---

## File Structure

**Files modified:**
- `internal/emulator/hikvision/handlers.go` — rewrite `handlePutHttpHosts`, update `handleGetHttpHosts`
- `internal/emulator/hikvision/emulator.go` — drop cache fields, drop `initializeRemoteSettings()`, change outgoing-URL construction in `sendEventToRemoteServer` and `sendDoorEvent`
- `internal/emulator/hikvision/repository.go` — add `RemoteURL` fallback in `GetSetting` switch

**Files created:**
- `internal/emulator/hikvision/handlers_httphosts_test.go` — unit test for the XML parser used by `handlePutHttpHosts`

No DB migrations. No new dependencies.

---

## Task 1: Add `RemoteURL` default in repository

**Files:**
- Modify: `internal/emulator/hikvision/repository.go:72-82`

This is a one-liner addition to the existing switch. Doing it first means later tasks can rely on the default.

- [ ] **Step 1: Add the `RemoteURL` case**

In [internal/emulator/hikvision/repository.go](internal/emulator/hikvision/repository.go), find the `GetSetting` function (around line 63) and its switch statement (around line 72). Current code:

```go
		switch key {
		case "RemoteServer":
			return "localhost", nil
		case "RemotePort":
			return "15501", nil
		case "LocalAuthentication":
			return "1", nil
		default:
			return "", err
		}
```

Change to:

```go
		switch key {
		case "RemoteServer":
			return "localhost", nil
		case "RemotePort":
			return "15501", nil
		case "RemoteURL":
			return "/notification", nil
		case "LocalAuthentication":
			return "1", nil
		default:
			return "", err
		}
```

- [ ] **Step 2: Build to verify nothing broke**

Run: `go build ./...`
Expected: exits 0, no output.

- [ ] **Step 3: Commit**

```bash
git add internal/emulator/hikvision/repository.go
git commit -m "feat(hikvision): add RemoteURL default in GetSetting fallback"
```

---

## Task 2: Add XML parser types + unit test

**Files:**
- Modify: `internal/emulator/hikvision/handlers.go` — add XML types near the top of the file (below `XMLResponseStatus`)
- Create: `internal/emulator/hikvision/handlers_httphosts_test.go`

We extract the parsing into a pure function so it can be unit-tested without spinning up a Gin context, DB, or Emulator.

- [ ] **Step 1: Add XML types and pure parser in `handlers.go`**

In [internal/emulator/hikvision/handlers.go](internal/emulator/hikvision/handlers.go), after the `XMLResponseStatus` struct definition (after line 26, before `SetupRoutes`), add:

```go
// HttpHostNotificationList is the payload sent by the client on
// PUT /ISAPI/Event/notification/httpHosts. We only consume the first
// HttpHostNotification entry and only the fields needed for online event
// delivery (ipAddress, portNo, url).
type HttpHostNotificationList struct {
	XMLName xml.Name                   `xml:"HttpHostNotificationList"`
	Items   []HttpHostNotificationItem `xml:"HttpHostNotification"`
}

type HttpHostNotificationItem struct {
	ID        string `xml:"id"`
	URL       string `xml:"url"`
	IPAddress string `xml:"ipAddress"`
	PortNo    string `xml:"portNo"`
}

// parseHttpHostNotification extracts ipAddress, portNo, url from the raw
// XML body sent by the client. Returns an error if the body is not valid
// XML or if no HttpHostNotification entries are present.
func parseHttpHostNotification(body []byte) (HttpHostNotificationItem, error) {
	var list HttpHostNotificationList
	if err := xml.Unmarshal(body, &list); err != nil {
		return HttpHostNotificationItem{}, fmt.Errorf("invalid XML: %w", err)
	}
	if len(list.Items) == 0 {
		return HttpHostNotificationItem{}, fmt.Errorf("no HttpHostNotification entries")
	}
	return list.Items[0], nil
}
```

`encoding/xml` and `fmt` are already imported in this file.

- [ ] **Step 2: Write the failing test**

Create `internal/emulator/hikvision/handlers_httphosts_test.go`:

```go
package hikvision

import "testing"

func TestParseHttpHostNotification_Valid(t *testing.T) {
	body := []byte(`<HttpHostNotificationList version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
    <HttpHostNotification>
        <id>1</id>
        <url>/w-access</url>
        <protocolType>HTTP</protocolType>
        <parameterFormatType>JSON</parameterFormatType>
        <addressingFormatType>ipaddress</addressingFormatType>
        <ipAddress>192.168.10.20</ipAddress>
        <portNo>15501</portNo>
        <userName></userName>
        <httpAuthenticationMethod>none</httpAuthenticationMethod>
    </HttpHostNotification>
</HttpHostNotificationList>`)

	item, err := parseHttpHostNotification(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.IPAddress != "192.168.10.20" {
		t.Errorf("IPAddress: got %q, want %q", item.IPAddress, "192.168.10.20")
	}
	if item.PortNo != "15501" {
		t.Errorf("PortNo: got %q, want %q", item.PortNo, "15501")
	}
	if item.URL != "/w-access" {
		t.Errorf("URL: got %q, want %q", item.URL, "/w-access")
	}
}

func TestParseHttpHostNotification_Malformed(t *testing.T) {
	body := []byte(`not xml at all`)
	_, err := parseHttpHostNotification(body)
	if err == nil {
		t.Fatal("expected error for malformed XML, got nil")
	}
}

func TestParseHttpHostNotification_Empty(t *testing.T) {
	body := []byte(`<HttpHostNotificationList version="2.0"></HttpHostNotificationList>`)
	_, err := parseHttpHostNotification(body)
	if err == nil {
		t.Fatal("expected error for empty list, got nil")
	}
}
```

- [ ] **Step 3: Run the test to verify it passes**

Run: `go test ./internal/emulator/hikvision/ -run TestParseHttpHostNotification -v`
Expected: three PASS lines, `ok` at the end.

Note: the test is expected to PASS on first run because Step 1 already added the parser. This task is "implement + test together" — the test exists to pin the contract before Task 3 wires the parser into the HTTP handler.

- [ ] **Step 4: Commit**

```bash
git add internal/emulator/hikvision/handlers.go internal/emulator/hikvision/handlers_httphosts_test.go
git commit -m "feat(hikvision): add HttpHostNotification XML parser with tests"
```

---

## Task 3: Rewrite `handlePutHttpHosts` to parse, persist, and respond with valid XML

**Files:**
- Modify: `internal/emulator/hikvision/handlers.go:959-962`

- [ ] **Step 1: Replace the stub handler**

In [internal/emulator/hikvision/handlers.go](internal/emulator/hikvision/handlers.go), find:

```go
func (e *Emulator) handlePutHttpHosts(c *gin.Context) {
	e.tracer.Info("Receiving configuration from server")
	c.String(http.StatusOK, "OK")
}
```

Replace with:

```go
func (e *Emulator) handlePutHttpHosts(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		e.tracer.Error("httpHosts: failed to read body: %v", err)
		c.XML(http.StatusBadRequest, errorXMLResponse("Invalid body"))
		return
	}

	item, err := parseHttpHostNotification(body)
	if err != nil {
		e.tracer.Error("httpHosts: parse failed: %v", err)
		c.XML(http.StatusBadRequest, errorXMLResponse("Invalid XML"))
		return
	}

	if err := e.repo.SetSetting("RemoteServer", item.IPAddress); err != nil {
		e.tracer.Error("httpHosts: SetSetting RemoteServer failed: %v", err)
		c.XML(http.StatusInternalServerError, errorXMLResponse(err.Error()))
		return
	}
	if err := e.repo.SetSetting("RemotePort", item.PortNo); err != nil {
		e.tracer.Error("httpHosts: SetSetting RemotePort failed: %v", err)
		c.XML(http.StatusInternalServerError, errorXMLResponse(err.Error()))
		return
	}
	if err := e.repo.SetSetting("RemoteURL", item.URL); err != nil {
		e.tracer.Error("httpHosts: SetSetting RemoteURL failed: %v", err)
		c.XML(http.StatusInternalServerError, errorXMLResponse(err.Error()))
		return
	}

	e.tracer.Info("httpHosts persisted: ipAddress=%s portNo=%s url=%s",
		item.IPAddress, item.PortNo, item.URL)

	c.XML(http.StatusOK, successXMLResponse())
}
```

`io` is already imported in this file (line 8).

- [ ] **Step 2: Build to verify compilation**

Run: `go build ./...`
Expected: exits 0, no output.

- [ ] **Step 3: Run existing tests**

Run: `go test ./internal/emulator/hikvision/ -v`
Expected: the three `TestParseHttpHostNotification_*` tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/emulator/hikvision/handlers.go
git commit -m "fix(hikvision): parse and persist httpHosts PUT, return valid XML response"
```

---

## Task 4: Update `handleGetHttpHosts` to return persisted values

**Files:**
- Modify: `internal/emulator/hikvision/handlers.go:920-958`

- [ ] **Step 1: Replace the handler body**

In [internal/emulator/hikvision/handlers.go](internal/emulator/hikvision/handlers.go), find `handleGetHttpHosts`:

```go
func (e *Emulator) handleGetHttpHosts(c *gin.Context) {
	e.tracer.Info("Getting Info httpHosts")

	remoteServer, err := e.repo.GetSetting("RemoteServer")
	if err != nil || remoteServer == "" {
		remoteServer = "172.16.17.20" // valor padrão
	}

	xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<HttpHostNotificationList version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
    <HttpHostNotification>
        <id>1</id>
        <url>/w-access</url>
        <protocolType>HTTP</protocolType>
        <parameterFormatType>XML</parameterFormatType>
        <addressingFormatType>ipaddress</addressingFormatType>
        <ipAddress>%s</ipAddress>
        <portNo>15501</portNo>
        <httpAuthenticationMethod>none</httpAuthenticationMethod>
        <SubscribeEvent>
            <heartbeat>30</heartbeat>
            <eventMode>all</eventMode>
        </SubscribeEvent>
    </HttpHostNotification>
    <HttpHostNotification>
        <id>2</id>
        <url></url>
        <protocolType>EHome</protocolType>
        <parameterFormatType>XML</parameterFormatType>
        <addressingFormatType>ipaddress</addressingFormatType>
	    <ipAddress>0.0.0.0</ipAddress>
        <portNo>0</portNo>
        <httpAuthenticationMethod>none</httpAuthenticationMethod>
    </HttpHostNotification>
</HttpHostNotificationList>`, remoteServer)

	c.Header("Content-Type", "application/xml")
	c.String(http.StatusOK, xmlContent)
}
```

Replace with:

```go
func (e *Emulator) handleGetHttpHosts(c *gin.Context) {
	e.tracer.Info("Getting Info httpHosts")

	remoteServer, _ := e.repo.GetSetting("RemoteServer")
	remotePort, _ := e.repo.GetSetting("RemotePort")
	remoteURL, _ := e.repo.GetSetting("RemoteURL")

	xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<HttpHostNotificationList version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
    <HttpHostNotification>
        <id>1</id>
        <url>%s</url>
        <protocolType>HTTP</protocolType>
        <parameterFormatType>XML</parameterFormatType>
        <addressingFormatType>ipaddress</addressingFormatType>
        <ipAddress>%s</ipAddress>
        <portNo>%s</portNo>
        <httpAuthenticationMethod>none</httpAuthenticationMethod>
        <SubscribeEvent>
            <heartbeat>30</heartbeat>
            <eventMode>all</eventMode>
        </SubscribeEvent>
    </HttpHostNotification>
    <HttpHostNotification>
        <id>2</id>
        <url></url>
        <protocolType>EHome</protocolType>
        <parameterFormatType>XML</parameterFormatType>
        <addressingFormatType>ipaddress</addressingFormatType>
	    <ipAddress>0.0.0.0</ipAddress>
        <portNo>0</portNo>
        <httpAuthenticationMethod>none</httpAuthenticationMethod>
    </HttpHostNotification>
</HttpHostNotificationList>`, remoteURL, remoteServer, remotePort)

	c.Header("Content-Type", "application/xml")
	c.String(http.StatusOK, xmlContent)
}
```

The hardcoded `172.16.17.20` fallback is removed — `GetSetting` already returns `"localhost"` for `RemoteServer` when no row exists, and now returns `"15501"`/`"/notification"` for the other two. Ignoring the error is intentional: `GetSetting` returns the default string with a non-nil error in that case, and we want the default.

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add internal/emulator/hikvision/handlers.go
git commit -m "feat(hikvision): GET httpHosts returns per-device persisted values"
```

---

## Task 5: Remove startup cache and use per-event settings for outgoing URL

**Files:**
- Modify: `internal/emulator/hikvision/emulator.go` — remove struct fields, remove `initializeRemoteSettings`, remove its call, update two event-sending sites

This is the largest single edit. Do it in one commit because the struct-field removal, the init removal, and the call-site changes must all happen together or the build breaks.

- [ ] **Step 1: Remove the three cache fields from the `Emulator` struct**

In [internal/emulator/hikvision/emulator.go](internal/emulator/hikvision/emulator.go), lines 32-35:

```go
	startTime        *time.Time // Timestamp de quando o emulador foi iniciado

	// Configurações do servidor remoto
	remoteServer    string
	remotePort      string
	remoteServerURL string
}
```

Change to:

```go
	startTime        *time.Time // Timestamp de quando o emulador foi iniciado
}
```

(Remove the blank line, the comment, and the three field lines.)

- [ ] **Step 2: Remove the `initializeRemoteSettings()` call from `NewEmulator`**

In the same file, around lines 54-57:

```go
	// Inicializar configurações do servidor remoto
	emulator.initializeRemoteSettings()

	return emulator
```

Change to:

```go
	return emulator
```

- [ ] **Step 3: Delete the `initializeRemoteSettings` function**

In the same file, lines 61-77. Delete the entire function:

```go
// initializeRemoteSettings inicializa as configurações do servidor remoto
func (e *Emulator) initializeRemoteSettings() {
	if server, err := e.repo.GetSetting("RemoteServer"); err == nil && server != "" {
		e.remoteServer = server
	} else {
		e.remoteServer = "localhost"
	}

	if port, err := e.repo.GetSetting("RemotePort"); err == nil && port != "" {
		e.remotePort = port
	} else {
		e.remotePort = "15501"
	}

	e.remoteServerURL = fmt.Sprintf("http://%s:%s", e.remoteServer, e.remotePort)
	e.tracer.Info("Remote server URL: %s", e.remoteServerURL)
}
```

Remove the leading blank line too so `Start` follows `NewEmulator` cleanly.

- [ ] **Step 4: Update `sendEventToRemoteServer` to build URL from settings**

In the same file, find the line (around 364):

```go
	// Envia o evento para o servidor remoto
	remoteURL := e.remoteServerURL + "/notification"
	e.tracer.Info("Sending event to server: %s", remoteURL)
```

Replace with:

```go
	// Monta a URL a partir das settings por dispositivo
	server, _ := e.repo.GetSetting("RemoteServer")
	port, _ := e.repo.GetSetting("RemotePort")
	path, _ := e.repo.GetSetting("RemoteURL")
	remoteURL := fmt.Sprintf("http://%s:%s%s", server, port, path)
	e.tracer.Info("Sending event to server: %s", remoteURL)
```

- [ ] **Step 5: Update `sendDoorEvent` the same way**

In the same file, find the line (around 435):

```go
	// Envia o evento para o servidor remoto
	remoteURL := e.remoteServerURL + "/notification"
```

Replace with:

```go
	// Monta a URL a partir das settings por dispositivo
	server, _ := e.repo.GetSetting("RemoteServer")
	port, _ := e.repo.GetSetting("RemotePort")
	path, _ := e.repo.GetSetting("RemoteURL")
	remoteURL := fmt.Sprintf("http://%s:%s%s", server, port, path)
```

- [ ] **Step 6: Build and verify no references remain**

Run: `go build ./...`
Expected: exits 0.

Run: `grep -rn "remoteServer\|remotePort\|remoteServerURL\|initializeRemoteSettings" internal/emulator/hikvision/`
Expected: only local-variable matches inside `handleGetHttpHosts` (the `remoteServer`/`remotePort`/`remoteURL` locals added in Task 4). No matches referencing `e.remoteServer` / `e.remotePort` / `e.remoteServerURL` / `initializeRemoteSettings`.

- [ ] **Step 7: Run tests**

Run: `go test ./internal/emulator/hikvision/ -v`
Expected: `TestParseHttpHostNotification_*` tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/emulator/hikvision/emulator.go
git commit -m "refactor(hikvision): drop startup remote-URL cache, read per event"
```

---

## Task 6: Full build + manual end-to-end verification

**Files:** none modified.

This task validates the integration. If any step fails, investigate before declaring the plan done.

- [ ] **Step 1: Full build and test suite**

Run: `go build ./...`
Expected: exits 0.

Run: `go test ./...`
Expected: existing `testes/db_test.go` and the new `TestParseHttpHostNotification_*` pass. No new failures.

- [ ] **Step 2: Start the service**

Follow the repo's normal startup procedure (e.g. `run-local.bat` on Windows, or `go run cmd/emulator-service/main.go`). Ensure at least one Hikvision device is configured in `service.devices` and enabled.

- [ ] **Step 3: Verify PUT httpHosts accepts valid XML and persists**

From another shell, against an enabled device on its HTTP port, run:

```bash
curl -s -X PUT \
  -H "Content-Type: application/xml" \
  --data-binary '<HttpHostNotificationList version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema"><HttpHostNotification><id>1</id><url>/w-access</url><protocolType>HTTP</protocolType><parameterFormatType>JSON</parameterFormatType><addressingFormatType>ipaddress</addressingFormatType><ipAddress>127.0.0.1</ipAddress><portNo>19999</portNo><userName></userName><httpAuthenticationMethod>none</httpAuthenticationMethod></HttpHostNotification></HttpHostNotificationList>' \
  http://localhost:<DEVICE_PORT>/ISAPI/Event/notification/httpHosts
```

Expected response:
```xml
<ResponseStatus version="1.0" xmlns="http://www.hikvision.com/ver10/XMLSchema"><requestURL></requestURL><statusCode>1</statusCode><statusString>OK</statusString><subStatusCode>ok</subStatusCode></ResponseStatus>
```

Verify in the emulator database:
```sql
SELECT cfg_id, value FROM emulator.device_settings
WHERE device_id = <DEVICE_ID>
  AND cfg_id IN ('RemoteServer', 'RemotePort', 'RemoteURL');
```
Expected three rows: `RemoteServer=127.0.0.1`, `RemotePort=19999`, `RemoteURL=/w-access`.

- [ ] **Step 4: Verify GET httpHosts returns the persisted values**

```bash
curl -s http://localhost:<DEVICE_PORT>/ISAPI/Event/notification/httpHosts
```

Expected: the XML response contains `<ipAddress>127.0.0.1</ipAddress>`, `<portNo>19999</portNo>`, `<url>/w-access</url>` inside the first `<HttpHostNotification>`.

- [ ] **Step 5: Verify malformed XML returns error XML, not plain text**

```bash
curl -s -X PUT \
  -H "Content-Type: application/xml" \
  --data-binary 'not xml' \
  http://localhost:<DEVICE_PORT>/ISAPI/Event/notification/httpHosts
```

Expected: HTTP 400, body is a valid `<ResponseStatus>` XML with `<statusCode>6</statusCode>` and `<subStatusCode>Invalid XML</subStatusCode>`. Running `python -c "import xmltodict, sys; xmltodict.parse(sys.stdin.read())"` piped with the output must succeed (no `ExpatError`).

- [ ] **Step 6: Verify PUT AcsCfg still works (no regression)**

```bash
curl -s -X PUT \
  -H "Content-Type: application/json" \
  -d '{"AcsCfg":{"remoteCheckDoorEnabled":true}}' \
  http://localhost:<DEVICE_PORT>/ISAPI/AccessControl/AcsCfg
```

Expected: the `ResponseStatus` success XML. `device_settings` row `LocalAuthentication=0`.

- [ ] **Step 7: Verify online event goes to the configured host**

Start a simple listener on port 19999 (the port we persisted in Step 3):

```bash
# In a separate shell:
python -m http.server 19999
# Or: ncat -lk 19999
```

Ensure the device row has `event_interval > 0` (e.g. 5 seconds) and is started.

In the emulator service log, within one `event_interval`, look for:
```
Sending event to server: http://127.0.0.1:19999/w-access
```

The listener receives a POST request.

- [ ] **Step 8: Run the real Python client once end-to-end**

Point the Python client at the emulator's port. The `__set_device_acs_config` sequence (PUT httpHosts, then PUT AcsCfg) must complete without logging `xml.parsers.expat.ExpatError: no element found`.

- [ ] **Step 9: Commit (if anything in the task required log-only adjustments)**

If no code change was needed, skip. Otherwise:

```bash
git add -u
git commit -m "chore(hikvision): post-verification log/formatting tweaks"
```

---

## Summary of commits produced by this plan

1. `feat(hikvision): add RemoteURL default in GetSetting fallback`
2. `feat(hikvision): add HttpHostNotification XML parser with tests`
3. `fix(hikvision): parse and persist httpHosts PUT, return valid XML response`
4. `feat(hikvision): GET httpHosts returns per-device persisted values`
5. `refactor(hikvision): drop startup remote-URL cache, read per event`
6. (optional) `chore(hikvision): post-verification log/formatting tweaks`

Each commit leaves the build green. After commit 3, the Python client already stops crashing. After commit 5, online events honor the per-device httpHosts configuration.
