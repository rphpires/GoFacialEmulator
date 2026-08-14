package reachability

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestComposeCoerenteComVariavel: o aviso da tela depende de
// PUBLISHED_PORT_RANGE descrever exatamente o que o compose publica. Se os
// dois divergirem, a aplicação acusa dispositivo saudável ou deixa passar
// dispositivo inalcançável — os dois piores desfechos possíveis para essa
// funcionalidade.
func TestComposeCoerenteComVariavel(t *testing.T) {
	conteudo, err := os.ReadFile("../../packaging/docker/docker-compose.yml")
	if err != nil {
		t.Fatalf("lendo o compose: %v", err)
	}

	var compose struct {
		Services struct {
			App struct {
				Environment map[string]string `yaml:"environment"`
				Ports       []string          `yaml:"ports"`
			} `yaml:"app"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(conteudo, &compose); err != nil {
		t.Fatalf("parseando o compose: %v", err)
	}

	declarado := compose.Services.App.Environment["PUBLISHED_PORT_RANGE"]
	if declarado == "" {
		t.Fatal("PUBLISHED_PORT_RANGE ausente no compose do pacote Docker")
	}

	faixas, err := ParseRanges(declarado)
	if err != nil {
		t.Fatalf("PUBLISHED_PORT_RANGE = %q é inválida: %v", declarado, err)
	}

	// Toda porta publicada que não seja a da interface web precisa estar
	// coberta pela variável.
	for _, p := range compose.Services.App.Ports {
		host, _, _ := cortarMapeamento(p)
		if host == "7070" {
			continue
		}
		publicadas, err := ParseRanges(host)
		if err != nil {
			t.Fatalf("mapeamento %q não pôde ser lido: %v", p, err)
		}
		for _, pub := range publicadas {
			if !cobre(faixas, pub) {
				t.Errorf("o compose publica %s mas PUBLISHED_PORT_RANGE=%q não cobre", pub, declarado)
			}
		}
	}
}

// TestComposeLinuxCoerente: em docker-compose.linux.yml a variável
// HOST_NETWORK é uma AFIRMAÇÃO sobre o modo de rede, não o modo em si —
// quem garante o acesso direto às portas é network_mode: host. As duas
// coisas não podem divergir: se alguém remover network_mode: host e
// esquecer a variável, Analyze continua confiando em HOST_NETWORK=1 e
// declara todo dispositivo alcançável enquanto nenhuma porta está
// publicada, exatamente o falso-verde que este pacote existe para evitar.
func TestComposeLinuxCoerente(t *testing.T) {
	conteudo, err := os.ReadFile("../../packaging/docker/docker-compose.linux.yml")
	if err != nil {
		t.Fatalf("lendo o compose: %v", err)
	}

	var compose struct {
		Services struct {
			App struct {
				NetworkMode string            `yaml:"network_mode"`
				Environment map[string]string `yaml:"environment"`
				Ports       []string          `yaml:"ports"`
			} `yaml:"app"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(conteudo, &compose); err != nil {
		t.Fatalf("parseando o compose: %v", err)
	}

	if compose.Services.App.NetworkMode != "host" {
		t.Errorf("network_mode = %q, quero \"host\"", compose.Services.App.NetworkMode)
	}
	if compose.Services.App.Environment["HOST_NETWORK"] != "1" {
		t.Errorf("HOST_NETWORK = %q, quero \"1\"", compose.Services.App.Environment["HOST_NETWORK"])
	}
	if len(compose.Services.App.Ports) != 0 {
		t.Errorf("ports = %v, quero nenhuma: rede de host não publica porta nenhuma", compose.Services.App.Ports)
	}
	if _, ok := compose.Services.App.Environment["PUBLISHED_PORT_RANGE"]; ok {
		t.Error("PUBLISHED_PORT_RANGE presente no compose Linux: rede de host não tem faixa publicada")
	}
}

func cortarMapeamento(s string) (host, container string, ok bool) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return s[:i], s[i+1:], true
		}
	}
	return s, s, false
}

func cobre(faixas []PortRange, alvo PortRange) bool {
	for _, f := range faixas {
		if f.Contains(alvo.Start) && f.Contains(alvo.End) {
			return true
		}
	}
	return false
}
