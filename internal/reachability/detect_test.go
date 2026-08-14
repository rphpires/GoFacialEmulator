package reachability

import (
	"fmt"
	"testing"
)

// leitorFalso é um sistema de arquivos de mentira: só os caminhos
// presentes no mapa existem.
type leitorFalso map[string]string

func (l leitorFalso) existe(path string) bool {
	_, ok := l[path]
	return ok
}

func (l leitorFalso) ler(path string) ([]byte, error) {
	v, ok := l[path]
	if !ok {
		return nil, fmt.Errorf("não existe: %s", path)
	}
	return []byte(v), nil
}

func semEnv(string) string { return "" }

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// TestDetect cobre como o ambiente é reconhecido. O caso mais delicado é
// o Docker: de dentro do container não há como enxergar o mapeamento de
// portas do host, então a única fonte é a variável que o compose grava.
func TestDetect(t *testing.T) {
	casos := []struct {
		nome            string
		fs              leitorFalso
		getenv          func(string) string
		goos            string
		queroKind       Kind
		queroRangesOK   bool
		queroFaixas     []PortRange
		queroHostNet    bool
		queroWSLEspelho bool
	}{
		{
			nome:      "windows nativo",
			fs:        leitorFalso{},
			getenv:    semEnv,
			goos:      "windows",
			queroKind: KindWindows,
		},
		{
			nome:      "linux nativo",
			fs:        leitorFalso{},
			getenv:    semEnv,
			goos:      "linux",
			queroKind: KindLinux,
		},
		{
			nome:      "wsl sem modo espelhado",
			fs:        leitorFalso{"/proc/version": "Linux version 5.15.0-microsoft-standard-WSL2"},
			getenv:    semEnv,
			goos:      "linux",
			queroKind: KindWSL,
		},
		{
			nome: "wsl com modo espelhado",
			fs: leitorFalso{
				"/proc/version":                   "Linux version 5.15.0-microsoft-standard-WSL2",
				"/sys/class/net/loopback0/address": "00:00:00:00:00:00",
			},
			getenv:          semEnv,
			goos:            "linux",
			queroKind:       KindWSL,
			queroWSLEspelho: true,
		},
		{
			nome:          "docker com faixa publicada",
			fs:            leitorFalso{"/.dockerenv": ""},
			getenv:        env(map[string]string{"PUBLISHED_PORT_RANGE": "4000-4499"}),
			goos:          "linux",
			queroKind:     KindDocker,
			queroRangesOK: true,
			queroFaixas:   []PortRange{{4000, 4499}},
		},
		{
			nome:          "docker sem a variável: faixa desconhecida",
			fs:            leitorFalso{"/.dockerenv": ""},
			getenv:        semEnv,
			goos:          "linux",
			queroKind:     KindDocker,
			queroRangesOK: false,
		},
		{
			nome:          "docker com variável malformada: faixa desconhecida",
			fs:            leitorFalso{"/.dockerenv": ""},
			getenv:        env(map[string]string{"PUBLISHED_PORT_RANGE": "4499-4000"}),
			goos:          "linux",
			queroKind:     KindDocker,
			queroRangesOK: false,
		},
		{
			nome:         "docker com rede de host",
			fs:           leitorFalso{"/.dockerenv": ""},
			getenv:       env(map[string]string{"HOST_NETWORK": "1"}),
			goos:         "linux",
			queroKind:    KindDocker,
			queroHostNet: true,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			e := detect(c.fs, c.getenv, c.goos, 1024)

			if e.Kind != c.queroKind {
				t.Errorf("Kind = %v, quero %v", e.Kind, c.queroKind)
			}
			if e.RangesKnown != c.queroRangesOK {
				t.Errorf("RangesKnown = %v, quero %v", e.RangesKnown, c.queroRangesOK)
			}
			if e.HostNetwork != c.queroHostNet {
				t.Errorf("HostNetwork = %v, quero %v", e.HostNetwork, c.queroHostNet)
			}
			if e.WSLMirrored != c.queroWSLEspelho {
				t.Errorf("WSLMirrored = %v, quero %v", e.WSLMirrored, c.queroWSLEspelho)
			}
			if len(e.PublishedRanges) != len(c.queroFaixas) {
				t.Fatalf("PublishedRanges = %v, quero %v", e.PublishedRanges, c.queroFaixas)
			}
			for i := range e.PublishedRanges {
				if e.PublishedRanges[i] != c.queroFaixas[i] {
					t.Errorf("faixa %d = %v, quero %v", i, e.PublishedRanges[i], c.queroFaixas[i])
				}
			}
		})
	}
}
