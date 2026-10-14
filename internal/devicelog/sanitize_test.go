package devicelog

import (
	"net/url"
	"strings"
	"testing"
)

func TestDescribeTrocaBase64PorTamanho(t *testing.T) {
	foto := strings.Repeat("QUJD", 1000) // 4000 caracteres base64
	got := Describe("application/json", []byte(`{"faceURL":"","img":"`+foto+`"}`))
	if strings.Contains(got, "QUJDQUJD") {
		t.Errorf("base64 não substituído: %s", got)
	}
	if !strings.Contains(got, "<base64 3KB>") {
		t.Errorf("quero <base64 3KB>: %s", got)
	}
}

func TestDescribeMascaraSenha(t *testing.T) {
	casos := map[string]struct{ ct, corpo, quero string }{
		"json": {"application/json", `{"userName":"a","password":"segredo"}`, `"password":"***"`},
		"xml":  {"application/xml", `<User><password>segredo</password></User>`, `<password>***</password>`},
		"form": {"application/x-www-form-urlencoded", `user=a&pwd=segredo`, `pwd=***`},
	}
	for nome, c := range casos {
		got := Describe(c.ct, []byte(c.corpo))
		if strings.Contains(got, "segredo") || !strings.Contains(got, c.quero) {
			t.Errorf("%s: %s", nome, got)
		}
	}
}

func TestDescribeCompactaEmUmaLinha(t *testing.T) {
	got := Describe("application/xml", []byte("<A>\n  <B>1</B>\n</A>\n"))
	if got != "<A><B>1</B></A>" {
		t.Errorf("got %q", got)
	}
}

func TestDescribeTrunca(t *testing.T) {
	got := Describe("text/plain", []byte(strings.Repeat("a ", 3000)))
	if !strings.HasSuffix(got, " …(cortado)") {
		t.Errorf("sem sufixo de corte")
	}
	if len(got) > maxBody+len(" …(cortado)") {
		t.Errorf("tamanho %d", len(got))
	}
}

func TestDescribeBinario(t *testing.T) {
	if got := Describe("image/jpeg", make([]byte, 48*1024)); got != "<image/jpeg 48KB>" {
		t.Errorf("got %q", got)
	}
	if got := Describe("", []byte{0xff, 0xd8, 0xff, 0x00}); got != "<application/octet-stream 4B>" {
		t.Errorf("got %q", got)
	}
}

const multipartPush = "--MIME_boundary\r\n" +
	"Content-Disposition: form-data; name=\"event_log\"\r\n" +
	"Content-Type: application/json\r\n\r\n" +
	`{"eventType":"AccessControllerEvent","AccessControllerEvent":{"majorEventType":5,"subEventType":75,"cardNo":"999","name":"Ana"}}` + "\r\n" +
	"--MIME_boundary\r\n" +
	"Content-Disposition: form-data; name=\"Picture\"; filename=\"Picture.jpg\"\r\n" +
	"Content-Type: image/jpeg\r\n\r\n" +
	"\xff\xd8\xff\x00\r\n" +
	"--MIME_boundary--\r\n"

func TestDescribeMultipart(t *testing.T) {
	got := Describe("multipart/form-data; boundary=MIME_boundary", []byte(multipartPush))
	if !strings.Contains(got, `[event_log] {"eventType":"AccessControllerEvent"`) {
		t.Errorf("parte JSON ausente: %s", got)
	}
	if !strings.Contains(got, "[Picture] <image/jpeg 4B>") {
		t.Errorf("parte imagem ausente: %s", got)
	}
}

func TestKeyFieldsJSONAninhado(t *testing.T) {
	got := KeyFields("application/json",
		[]byte(`{"UserInfo":{"employeeNo":"123","name":"Ana Paula","userType":"normal"}}`), nil)
	if got != `employeeNo=123 name="Ana Paula" userType=normal` {
		t.Errorf("got %q", got)
	}
}

func TestKeyFieldsXMLFormEQuery(t *testing.T) {
	if got := KeyFields("application/xml", []byte(`<CardInfo><employeeNo>7</employeeNo><cardNo>55</cardNo></CardInfo>`), nil); got != "employeeNo=7 cardNo=55" {
		t.Errorf("xml: %q", got)
	}
	if got := KeyFields("application/x-www-form-urlencoded", []byte("CardNo=9&CardName=Bia"), nil); got != "CardName=Bia CardNo=9" {
		t.Errorf("form: %q", got)
	}
	q := url.Values{"action": {"insert"}, "name": {"AccessUserCard"}, "ignorado": {"x"}}
	if got := KeyFields("", nil, q); got != "action=insert name=AccessUserCard" {
		t.Errorf("query: %q", got)
	}
}

func TestKeyFieldsLimites(t *testing.T) {
	// Ordem alfabética (maiúsculas antes): CardName vem primeiro, então o
	// valor longo entra e precisa sair cortado.
	corpo := `{"employeeNo":"1","name":"x","cardNo":"2","UserID":"3","CardName":"` + strings.Repeat("n", 60) + `","major":5,"minor":6,"searchID":"7"}`
	got := KeyFields("application/json", []byte(corpo), nil)
	if n := len(strings.Fields(got)); n != 6 {
		t.Errorf("%d campos, quero 6: %s", n, got)
	}
	if !strings.Contains(got, "CardName="+strings.Repeat("n", 40)+"…") || strings.Contains(got, strings.Repeat("n", 41)) {
		t.Errorf("valor não cortado em 40: %s", got)
	}
}

func TestKeyFieldsMultipartSemContentType(t *testing.T) {
	got := KeyFields("", []byte(multipartPush), nil)
	if got != "eventType=AccessControllerEvent cardNo=999 majorEventType=5 name=Ana subEventType=75" {
		t.Errorf("got %q", got)
	}
	// Part de stream do Dahua: começa com CRLF e não tem boundary final.
	dahua := "\r\n--myboundary\r\nContent-Type: text/plain\r\nContent-Length: 30\r\n\r\n" +
		`{"Events":[{"Code":"AccessControl","Data":{"UserID":"10"}}]}` + "\r\n"
	if got := KeyFields("", []byte(dahua), nil); got != "Code=AccessControl UserID=10" {
		t.Errorf("dahua: %q", got)
	}
}

func TestUserFromAuth(t *testing.T) {
	casos := map[string]string{
		`Digest username="admin", realm="x", nonce="y"`: "admin",
		"Basic YWRtaW46c2VncmVkbw==":                    "admin",
		"":                                              "",
		"Bearer abc":                                    "",
	}
	for h, quero := range casos {
		if got := UserFromAuth(h); got != quero {
			t.Errorf("UserFromAuth(%q) = %q, quero %q", h, got, quero)
		}
	}
}

func TestHumanSize(t *testing.T) {
	casos := map[int]string{0: "0B", 512: "512B", 1024: "1KB", 48 * 1024: "48KB", 3 << 20: "3.0MB"}
	for n, quero := range casos {
		if got := humanSize(n); got != quero {
			t.Errorf("humanSize(%d) = %q, quero %q", n, got, quero)
		}
	}
}
