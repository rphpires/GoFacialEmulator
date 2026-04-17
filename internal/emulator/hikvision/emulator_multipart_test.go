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
