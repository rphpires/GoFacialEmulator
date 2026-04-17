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
