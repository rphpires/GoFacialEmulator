# Hikvision Emulator — httpHosts PUT handling and online mode config

- **Date:** 2026-04-17
- **Area:** `internal/emulator/hikvision`
- **Status:** Draft for review

## Problem

The Python client calls two PUT endpoints during `__set_device_acs_config`:

1. `PUT /ISAPI/Event/notification/httpHosts` — carries an `HttpHostNotificationList` XML body telling the device where to send access events (`ipAddress`, `portNo`, `url`).
2. `PUT /ISAPI/AccessControl/AcsCfg` — JSON body with `remoteCheckDoorEnabled`, which selects **Online** (remote) vs **Local** authentication mode.

Today:

- `handlePutHttpHosts` ([internal/emulator/hikvision/handlers.go:959](../../../internal/emulator/hikvision/handlers.go#L959)) is a stub: logs a message and returns plain text `"OK"`. The client runs `xmltodict.parse(reply.content)` on the response and crashes with `xml.parsers.expat.ExpatError: no element found: line 1, column 0`. That crash aborts the whole ACS configuration flow on the client.
- Even if the response were valid, the XML body is never parsed — the `ipAddress`, `portNo`, `url` sent by the client are discarded.
- `handleGetHttpHosts` returns a mostly-hardcoded XML (`portNo=15501`, `url=/w-access` fixed in string).
- The `Emulator` struct caches `remoteServer`, `remotePort`, `remoteServerURL` once at construction via `initializeRemoteSettings()`. Any later update to `device_settings` is not reflected in-memory, so online events keep going to the startup values.
- `sendEventToRemoteServer` and `sendDoorEvent` hardcode the path component (`/notification`) in the outgoing URL, ignoring whatever `<url>` the client asked for.

`handlePutAcsCfg` is already working correctly: it parses JSON, persists `LocalAuthentication` per-device, and `GenerateEvent` already branches on that flag to call `generateOnlineEvent()`.

## Goals

1. Stop the Python client from crashing when it configures the emulator.
2. Make each emulator instance honor the `ipAddress`/`portNo`/`url` it received from the client, per device.
3. Preserve the existing local-vs-online branching (`LocalAuthentication == "0"` → online) — no change required there.

## Non-goals

- Changing the DB schema. All per-device settings continue to live in `emulator.device_settings` with the existing `GetSetting`/`SetSetting` pair.
- Adding auth/heartbeat/SubscribeEvent handling for httpHosts. We only persist and use the three fields actually used by the event-sending path.
- Refactoring the Dahua emulator. This change is scoped to `internal/emulator/hikvision`.
- Touching `handlePutAcsCfg` — it already behaves as required.

## Design

### Approach

Persist the three fields parsed from the PUT body as individual rows in `device_settings`, mirroring the pattern already used by `LocalAuthentication`. Drop the in-memory startup cache so each online event reads the current persisted values.

No new tables, no new in-memory mutable state, no concurrency primitives. Writes come from PUT handlers; reads come from the event-sending path. Each access to `device_settings` is a single SQL round-trip and the event cadence is governed by `EventInterval` (seconds), so cost is negligible.

### Data model

Three keys in `emulator.device_settings` (per `device_id`):

| Key            | Source (PUT body)                     | Default (from `GetSetting` fallback) |
|----------------|---------------------------------------|---------------------------------------|
| `RemoteServer` | `HttpHostNotification.ipAddress`      | `localhost` (already exists)          |
| `RemotePort`   | `HttpHostNotification.portNo`         | `15501` (already exists)              |
| `RemoteURL`    | `HttpHostNotification.url`            | `/notification` (new fallback)        |

`LocalAuthentication` (already in use) is unchanged.

The `/notification` default preserves the current behavior for emulators that never received a PUT: the outgoing URL is still `http://{RemoteServer}:{RemotePort}/notification`.

### Components changed

#### 1. `handlePutHttpHosts` (rewrite)

Location: [internal/emulator/hikvision/handlers.go:959](../../../internal/emulator/hikvision/handlers.go#L959).

Behavior:

- Read raw request body.
- `xml.Unmarshal` into a local struct shaped like:
  ```go
  type httpHostNotificationList struct {
      XMLName xml.Name `xml:"HttpHostNotificationList"`
      Items   []struct {
          ID        string `xml:"id"`
          URL       string `xml:"url"`
          IPAddress string `xml:"ipAddress"`
          PortNo    string `xml:"portNo"`
      } `xml:"HttpHostNotification"`
  }
  ```
- If unmarshal fails or `Items` is empty: respond `400` with `errorXMLResponse("Invalid XML")`.
- Take the first element (the Python client only ever sends one), persist via `e.repo.SetSetting` for each of `RemoteServer`, `RemotePort`, `RemoteURL`. If any `SetSetting` fails, respond `500` with `errorXMLResponse(err.Error())`.
- On success: log the persisted triple and respond `c.XML(200, successXMLResponse())` — same shape already used by `handlePutAcsCfg`, which the Python client accepts.

#### 2. `handleGetHttpHosts` (update)

Location: [internal/emulator/hikvision/handlers.go:920](../../../internal/emulator/hikvision/handlers.go#L920).

Read `RemoteServer`, `RemotePort`, `RemoteURL` from the repo and inject all three into the response XML (currently `portNo` and `url` are literal constants, and `ipAddress` has a bespoke fallback to `172.16.17.20`). Drop the `172.16.17.20` fallback — `GetSetting` already returns `"localhost"` for `RemoteServer` when no row exists, which is the correct default for this emulator-local response. The second `HttpHostNotification` block (id=2, EHome, `0.0.0.0:0`) stays as-is — the client doesn't use it.

#### 3. `repository.go` — default for `RemoteURL`

Location: [internal/emulator/hikvision/repository.go:73](../../../internal/emulator/hikvision/repository.go#L73).

Add a case to the existing switch in `GetSetting`:
```go
case "RemoteURL":
    return "/notification", nil
```

This matches the existing pattern for `RemoteServer`/`RemotePort`/`LocalAuthentication` and keeps the old behavior for emulators that never received a httpHosts PUT.

#### 4. Remove stale cache from `Emulator`

Location: [internal/emulator/hikvision/emulator.go](../../../internal/emulator/hikvision/emulator.go).

- Delete the fields `remoteServer`, `remotePort`, `remoteServerURL` from the `Emulator` struct.
- Delete `initializeRemoteSettings()` and the call to it from `NewEmulator`.
- In `sendEventToRemoteServer` (around line 324) and `sendDoorEvent` (around line 404), build the outgoing URL per call:
  ```go
  server, _ := e.repo.GetSetting("RemoteServer")
  port, _   := e.repo.GetSetting("RemotePort")
  path, _   := e.repo.GetSetting("RemoteURL")
  remoteURL := fmt.Sprintf("http://%s:%s%s", server, port, path)
  ```
- Replace the current literal `e.remoteServerURL + "/notification"` with this `remoteURL`. The path now comes from the setting, not from a hardcoded suffix.

No other use sites of the removed fields exist (confirmed in exploration).

### Request/response contract for `handlePutHttpHosts`

Request body (as sent by the Python client):
```xml
<HttpHostNotificationList version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
    <HttpHostNotification>
        <id>1</id>
        <url>/w-access</url>
        <protocolType>HTTP</protocolType>
        <parameterFormatType>JSON</parameterFormatType>
        <addressingFormatType>ipaddress</addressingFormatType>
        <ipAddress>192.168.x.y</ipAddress>
        <portNo>NNNNN</portNo>
        <userName></userName>
        <httpAuthenticationMethod>none</httpAuthenticationMethod>
    </HttpHostNotification>
</HttpHostNotificationList>
```

Successful response (200 OK, `Content-Type: application/xml`), produced by `c.XML(http.StatusOK, successXMLResponse())`:
```xml
<ResponseStatus version="1.0" xmlns="http://www.hikvision.com/ver10/XMLSchema">
  <requestURL></requestURL>
  <statusCode>1</statusCode>
  <statusString>OK</statusString>
  <subStatusCode>ok</subStatusCode>
</ResponseStatus>
```

This is the exact shape already returned by `handlePutAcsCfg`, and the Python client (`xmltodict.parse`) accepts it.

### End-to-end flow after the change

1. Client opens `PUT /ISAPI/Event/notification/httpHosts` with the XML above.
2. Emulator persists the three settings, responds with `successXMLResponse()`.
3. Client parses the response — no more `ExpatError`.
4. Client opens `PUT /ISAPI/AccessControl/AcsCfg` with `remoteCheckDoorEnabled: true`.
5. Emulator persists `LocalAuthentication="0"` (existing code path).
6. On each tick of the event generator (existing code), `GenerateEvent` sees `LocalAuthentication == "0"` and calls `generateOnlineEvent`.
7. `sendEventToRemoteServer` reads `RemoteServer`/`RemotePort`/`RemoteURL`, builds `http://{ipAddress_from_PUT}:{portNo_from_PUT}{url_from_PUT}`, and POSTs the multipart event there.

## Error handling

- Malformed XML body on PUT httpHosts → 400 with `errorXMLResponse("Invalid XML")`. The client will propagate but at least produces valid XML.
- `SetSetting` DB failure on PUT httpHosts → 500 with `errorXMLResponse(err.Error())`.
- On the event-sending path, a missing/empty `RemoteURL` is covered by the default (`/notification`) in `GetSetting`. If `SetSetting` somehow stored an empty string, the final URL becomes `http://{server}:{port}` with no path, which the remote side will reject with 404 — already handled by the existing `StatusCode != 200` error branch in `sendEventToRemoteServer`.

## Testing (manual)

Automated tests for the HTTP surface do not currently exist for Hikvision in this repo; follow the existing project convention of manual validation:

1. Start the service, enable a Hikvision emulator.
2. Send the Python client's PUT to `/ISAPI/Event/notification/httpHosts` (or use `curl` with the same XML body). Expect:
   - HTTP 200, XML response matching `successXMLResponse()`.
   - Row present in `emulator.device_settings` for `RemoteServer`, `RemotePort`, `RemoteURL` with the submitted values.
3. `GET /ISAPI/Event/notification/httpHosts` returns those same values in the response XML.
4. PUT `/ISAPI/AccessControl/AcsCfg` with `{"AcsCfg":{"remoteCheckDoorEnabled":true}}`. Expect `LocalAuthentication="0"` in settings.
5. With `EventInterval > 0`, observe in the emulator log: `Sending event to server: http://{ipAddress_from_PUT}:{portNo_from_PUT}{url_from_PUT}`.
6. Run the real Python client against the emulator — `__set_device_acs_config` completes without `ExpatError`.

## Files touched

- `internal/emulator/hikvision/handlers.go` — rewrite `handlePutHttpHosts`, update `handleGetHttpHosts`.
- `internal/emulator/hikvision/emulator.go` — drop cache fields, drop `initializeRemoteSettings`, update `sendEventToRemoteServer` and `sendDoorEvent`.
- `internal/emulator/hikvision/repository.go` — add `RemoteURL` default case in `GetSetting`.

## Open questions

None at spec time. If testing reveals that some real client needs an echoed `HttpHostNotification` response (instead of the generic `ResponseStatus`), revisit `handlePutHttpHosts` in a follow-up.
