package dahua

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// configStore guarda a configuração do dispositivo em memória. Antes o
// emulador respondia "OK" a qualquer setConfig sem guardar nada e só sabia
// responder getConfig para a tabela Network — o gerenciador nunca conseguia
// ler de volta CardStoreFormat, SensorEnable e companhia.
type configStore struct {
	mu     sync.RWMutex
	values map[string]string
	mac    string
}

// tabelasConhecidas são os `name` que o getConfig aceita. Qualquer outro
// devolve 400, como um device real que não tem aquela tabela.
var tabelasConhecidas = []string{
	"AccessControl",
	"AccessControlGeneral",
	"AccessTimeSchedule",
	"Intelbras_ModeCfg",
	"Network",
	"PictureHttpUpload",
	"PrivacySet",
}

func newConfigStore(mac string) *configStore {
	s := &configStore{values: map[string]string{}, mac: mac}

	// Defaults observados num controlador de acesso Dahua/Intelbras.
	padroes := map[string]string{
		"AccessControl[0].CardStoreFormat":     "0", // 0 HEXADECIMAL, 1 DECIMAL
		"AccessControl[0].UnlockHoldInterval":  "3000",
		"AccessControl[0].SensorEnable":        "false",
		"AccessControl[0].CloseTimeout":        "60",
		"AccessControl[0].State":               "Normal",
		"AccessControl[0].UnlockReloadEnable":  "true",
		"AccessControlGeneral.SnapshotUpload":  "false",
		"PrivacySet.IDPrivacyEnable":           "false",
		"PrivacySet.NamePrivacyEnable":         "false",
		"PrivacySet.NumPrivacyEnable":          "false",
		"Intelbras_ModeCfg.DeviceMode":         "0",
		"Intelbras_ModeCfg.KeepAlive.Enable":   "false",
		"Intelbras_ModeCfg.KeepAlive.Interval": "5",
		"Intelbras_ModeCfg.KeepAlive.Path":     "/keepalive",
		"Intelbras_ModeCfg.KeepAlive.TimeOut":  "2000",
		"AccessTimeSchedule[0].Enable":         "true",
		"AccessTimeSchedule[0].Name":           "AllDay",
	}
	for k, v := range padroes {
		s.values[k] = v
	}
	return s
}

func (s *configStore) Set(chave, valor string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[chave] = valor
}

func (s *configStore) Get(chave string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values[chave]
}

// SetAllFromQuery grava todos os parâmetros de um setConfig, ignorando os de
// controle da própria CGI. Devolve quantas chaves foram gravadas.
func (s *configStore) SetAllFromQuery(q url.Values) int {
	controle := map[string]bool{"action": true, "name": true, "channel": true}

	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for chave, vals := range q {
		if controle[chave] || len(vals) == 0 {
			continue
		}
		s.values[chave] = vals[0]
		n++
	}
	return n
}

// Render devolve a tabela no formato `table.<Nome>...=valor`, CRLF-separado,
// que __parse_table_response espera (IoDahuaCommunication.py:1522).
func (s *configStore) Render(tabela string) (string, bool) {
	if strings.EqualFold(tabela, "Network") {
		return s.renderNetwork(), true
	}

	conhecida := false
	for _, t := range tabelasConhecidas {
		if strings.EqualFold(t, tabela) {
			tabela = t
			conhecida = true
			break
		}
	}
	if !conhecida {
		return "", false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var linhas []string
	for chave, valor := range s.values {
		if chave == tabela || strings.HasPrefix(chave, tabela+"[") || strings.HasPrefix(chave, tabela+".") {
			linhas = append(linhas, fmt.Sprintf("table.%s=%s", chave, valor))
		}
	}
	sort.Strings(linhas)
	return strings.Join(linhas, "\r\n"), true
}

// renderNetwork mantém a resposta que o emulador já dava; __get_mac_address
// fatia a string entre "PhysicalAddress=" e "\r\ntable.Network.eth0.SubnetMask=".
func (s *configStore) renderNetwork() string {
	return fmt.Sprintf("table.Network.DefaultInterface=eth0\r\n"+
		"table.Network.Domain=dahua\r\n"+
		"table.Network.Hostname=BSC\r\n"+
		"table.Network.eth0.DefaultGateway=192.168.0.1\r\n"+
		"table.Network.eth0.DhcpEnable=false\r\n"+
		"table.Network.eth0.DnsServers[0]=8.8.8.8\r\n"+
		"table.Network.eth0.DnsServers[1]=8.8.4.4\r\n"+
		"table.Network.eth0.EnableDhcpReservedIP=false\r\n"+
		"table.Network.eth0.IPAddress=192.168.0.100\r\n"+
		"table.Network.eth0.MTU=1500\r\n"+
		"table.Network.eth0.PhysicalAddress=%s\r\n"+
		"table.Network.eth0.SubnetMask=255.255.255.0\r\n"+
		"table.Network.eth2.DefaultGateway=192.168.0.1\r\n"+
		"table.Network.eth2.DhcpEnable=true\r\n"+
		"table.Network.eth2.DnsServers[0]=8.8.8.8\r\n"+
		"table.Network.eth2.DnsServers[1]=8.8.4.4\r\n"+
		"table.Network.eth2.EnableDhcpReservedIP=false\r\n"+
		"table.Network.eth2.IPAddress=192.168.0.101\r\n"+
		"table.Network.eth2.MTU=1500\r\n"+
		"table.Network.eth2.PhysicalAddress=00:00:00:00:00:00\r\n"+
		"table.Network.eth2.SubnetMask=255.255.0.0", s.mac)
}
