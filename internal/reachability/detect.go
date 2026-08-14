package reachability

import (
	"os"
	"runtime"
	"strings"
)

// leitor isola as leituras de sistema de arquivos para que detect possa
// ser testada sem depender do ambiente real da máquina de build.
type leitor interface {
	existe(path string) bool
	ler(path string) ([]byte, error)
}

type leitorReal struct{}

func (leitorReal) existe(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (leitorReal) ler(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Detect fotografa o ambiente. Chamada uma vez na inicialização.
func Detect() Environment {
	return detect(leitorReal{}, os.Getenv, runtime.GOOS, limiteArquivosAbertos())
}

func detect(fs leitor, getenv func(string) string, goos string, maxOpenFiles uint64) Environment {
	e := Environment{MaxOpenFiles: maxOpenFiles}

	switch {
	case fs.existe("/.dockerenv"):
		e.Kind = KindDocker
		e.HostNetwork = getenv("HOST_NETWORK") == "1"
		if !e.HostNetwork {
			if faixas, err := ParseRanges(getenv("PUBLISHED_PORT_RANGE")); err == nil {
				e.PublishedRanges = faixas
				e.RangesKnown = true
			}
		}

	case ehWSL(fs):
		e.Kind = KindWSL
		e.WSLMirrored = wslEspelhado(fs)

	case goos == "windows":
		e.Kind = KindWindows

	default:
		e.Kind = KindLinux
	}

	return e
}

func ehWSL(fs leitor) bool {
	conteudo, err := fs.ler("/proc/version")
	if err != nil {
		return false
	}
	v := strings.ToLower(string(conteudo))
	return strings.Contains(v, "microsoft") || strings.Contains(v, "wsl")
}

// wslEspelhado usa a interface loopback0, criada pelo WSL2 quando
// networkingMode=mirrored está ligado. Verificado empiricamente numa WSL2
// sem modo espelhado (`wsl -e ls /sys/class/net/` listou apenas eth0 e lo,
// sem loopback0): a direção perigosa, em que o marcador ficaria presente
// mesmo sem modo espelhado e o código silenciaria um setup quebrado, não
// se confirmou. A confirmação da direção oposta — ligar networkingMode=
// mirrored e ver loopback0 aparecer — fica para uma passagem futura, pois
// exige mexer no .wslconfig da máquina e reiniciar a WSL.
func wslEspelhado(fs leitor) bool {
	return fs.existe("/sys/class/net/loopback0/address")
}
