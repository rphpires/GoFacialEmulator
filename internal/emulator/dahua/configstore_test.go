package dahua

import (
	"net/url"
	"strings"
	"testing"
)

func TestConfigStore_RenderAccessControlDefaults(t *testing.T) {
	s := newConfigStore("00:11:22:33:44:55")

	saida, ok := s.Render("AccessControl")
	if !ok {
		t.Fatal("AccessControl deve ser uma tabela conhecida")
	}
	// __parse_table_response corta o prefixo "table.AccessControl" e depois
	// exige que a chave comece com "[0]." ou "." — IoDahuaCommunication.py:1522.
	for _, obrigatorio := range []string{
		"table.AccessControl[0].CardStoreFormat=",
		"table.AccessControl[0].UnlockHoldInterval=",
		"table.AccessControl[0].SensorEnable=",
		"table.AccessControl[0].CloseTimeout=",
		"table.AccessControl[0].State=",
	} {
		if !strings.Contains(saida, obrigatorio) {
			t.Errorf("falta %q na resposta:\n%s", obrigatorio, saida)
		}
	}
	if !strings.Contains(saida, "\r\n") {
		t.Error("as linhas devem ser separadas por CRLF, como o device real")
	}
	// AccessControlGeneral nao pode vazar para dentro de AccessControl: o
	// prefixo casa por string e um match ingenuo levaria as duas juntas.
	if strings.Contains(saida, "AccessControlGeneral") {
		t.Errorf("AccessControlGeneral vazou para a tabela AccessControl:\n%s", saida)
	}
}

func TestConfigStore_SetThenRenderRoundTrip(t *testing.T) {
	s := newConfigStore("00:11:22:33:44:55")
	s.Set("AccessControl[0].CardStoreFormat", "1")

	saida, _ := s.Render("AccessControl")
	if !strings.Contains(saida, "table.AccessControl[0].CardStoreFormat=1") {
		t.Errorf("valor setado nao voltou no getConfig:\n%s", saida)
	}
}

func TestConfigStore_SetAllFromQuerySkipsControlParams(t *testing.T) {
	s := newConfigStore("00:11:22:33:44:55")
	q := url.Values{}
	q.Set("action", "setConfig")
	q.Set("AccessControl[0].SensorEnable", "true")
	q.Set("Intelbras_ModeCfg.DeviceMode", "2")

	n := s.SetAllFromQuery(q)
	if n != 2 {
		t.Errorf("chaves gravadas: got %d, want 2 (action nao conta)", n)
	}
	if s.Get("Intelbras_ModeCfg.DeviceMode") != "2" {
		t.Errorf("DeviceMode: got %q, want 2", s.Get("Intelbras_ModeCfg.DeviceMode"))
	}
	if s.Get("action") != "" {
		t.Error("action nao pode virar configuracao")
	}
}

func TestConfigStore_RenderUnknownTable(t *testing.T) {
	s := newConfigStore("00:11:22:33:44:55")
	if _, ok := s.Render("NaoExiste"); ok {
		t.Error("tabela desconhecida deve devolver ok=false")
	}
}

func TestConfigStore_RenderNetworkKeepsMacDelimiters(t *testing.T) {
	s := newConfigStore("00:11:22:33:44:55")
	saida, ok := s.Render("Network")
	if !ok {
		t.Fatal("Network deve ser conhecida")
	}
	// __get_mac_address fatia entre "PhysicalAddress=" e o delimitador abaixo.
	if !strings.Contains(saida, "table.Network.eth0.PhysicalAddress=00:11:22:33:44:55") {
		t.Errorf("MAC ausente:\n%s", saida)
	}
	if !strings.Contains(saida, "\r\ntable.Network.eth0.SubnetMask=") {
		t.Error("o delimitador que __get_mac_address usa sumiu")
	}
}
