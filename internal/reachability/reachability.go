// Package reachability decide se as portas que o W-Access pediu conseguem
// ser alcançadas no ambiente onde a aplicação está rodando.
//
// A aplicação obedece o BaseCommPort de cada controlador e nunca o
// contradiz: este pacote não escolhe porta nem recusa dispositivo, apenas
// aponta quais não vão funcionar e por quê.
package reachability

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind identifica o tipo de ambiente onde a aplicação está rodando.
type Kind int

const (
	KindLinux Kind = iota
	KindWSL
	KindDocker
	KindWindows
)

func (k Kind) String() string {
	switch k {
	case KindLinux:
		return "linux"
	case KindWSL:
		return "wsl"
	case KindDocker:
		return "docker"
	case KindWindows:
		return "windows"
	default:
		return "desconhecido"
	}
}

// PortRange é uma faixa fechada de portas.
type PortRange struct {
	Start int
	End   int
}

func (r PortRange) Contains(port int) bool {
	return port >= r.Start && port <= r.End
}

func (r PortRange) String() string {
	if r.Start == r.End {
		return strconv.Itoa(r.Start)
	}
	return fmt.Sprintf("%d-%d", r.Start, r.End)
}

// Environment é a fotografia do ambiente, tirada uma vez na inicialização.
type Environment struct {
	Kind            Kind        `json:"kind"`
	PublishedRanges []PortRange `json:"published_ranges"`
	// RangesKnown falso significa que não foi possível descobrir o que o
	// host publica. Nesse caso nenhum aviso é inventado.
	RangesKnown  bool   `json:"ranges_known"`
	HostNetwork  bool   `json:"host_network"`
	WSLMirrored  bool   `json:"wsl_mirrored"`
	MaxOpenFiles uint64 `json:"max_open_files"`
}

// Status é o veredito para um dispositivo.
type Status int

const (
	StatusOK Status = iota
	StatusUnreachable
	StatusUnknown
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusUnreachable:
		return "inalcancavel"
	default:
		return "desconhecido"
	}
}

func (s Status) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(s.String())), nil
}

// DevicePort é a entrada de Analyze: o que o W-Access pediu para um
// dispositivo e o que aconteceu quando tentamos abrir aquela porta.
type DevicePort struct {
	DeviceID int
	Port     int
	// Started indica que o emulador chegou a ser iniciado. Sem isso não há
	// informação de bind, e o veredito honesto é "desconhecido".
	Started bool
	// BindError é o erro do sistema operacional na abertura da porta, vazio
	// quando o bind funcionou.
	BindError string
}

// DeviceReachability é o veredito por dispositivo.
type DeviceReachability struct {
	DeviceID int    `json:"device_id"`
	Port     int    `json:"port"`
	Status   Status `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

// Report é o que a interface consome.
type Report struct {
	Environment Environment          `json:"environment"`
	Devices     []DeviceReachability `json:"devices"`
	Unreachable int                  `json:"unreachable"`
	Unknown     int                  `json:"unknown"`
}

// ParseRanges lê o formato gravado em PUBLISHED_PORT_RANGE pelo compose:
// entradas separadas por vírgula, cada uma "porta" ou "inicio-fim".
func ParseRanges(s string) ([]PortRange, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("faixa de portas vazia")
	}

	var faixas []PortRange
	for _, parte := range strings.Split(s, ",") {
		parte = strings.TrimSpace(parte)
		if parte == "" {
			return nil, fmt.Errorf("entrada vazia em %q", s)
		}

		inicio, fim, temHifen := strings.Cut(parte, "-")
		if !temHifen {
			fim = inicio
		}

		p1, err := porta(inicio)
		if err != nil {
			return nil, fmt.Errorf("em %q: %w", parte, err)
		}
		p2, err := porta(fim)
		if err != nil {
			return nil, fmt.Errorf("em %q: %w", parte, err)
		}
		if p1 > p2 {
			return nil, fmt.Errorf("em %q: início maior que fim", parte)
		}
		faixas = append(faixas, PortRange{Start: p1, End: p2})
	}
	return faixas, nil
}

func porta(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%q não é um número de porta", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("porta %d fora do intervalo 1-65535", n)
	}
	return n, nil
}

// Analyze é pura: mesmo par de argumentos, mesmo relatório. Sem I/O, sem
// relógio, sem banco.
func Analyze(ports []DevicePort, env Environment) Report {
	r := Report{Environment: env}

	for _, p := range ports {
		d := DeviceReachability{DeviceID: p.DeviceID, Port: p.Port}

		switch {
		case env.Kind == KindDocker && !env.HostNetwork:
			d.Status, d.Reason = vereditoDocker(p, env)
		case env.Kind == KindWSL && !env.WSLMirrored:
			d.Status = StatusUnreachable
			d.Reason = fmt.Sprintf("nesta WSL sem modo espelhado, só esta máquina alcança a porta %d", p.Port)
		default:
			d.Status, d.Reason = vereditoNativo(p)
		}

		switch d.Status {
		case StatusUnreachable:
			r.Unreachable++
		case StatusUnknown:
			r.Unknown++
		}
		r.Devices = append(r.Devices, d)
	}

	return r
}

func vereditoDocker(p DevicePort, env Environment) (Status, string) {
	if !env.RangesKnown {
		return StatusUnknown, "não foi possível verificar quais portas este ambiente Docker publica"
	}
	for _, faixa := range env.PublishedRanges {
		if faixa.Contains(p.Port) {
			return StatusOK, ""
		}
	}
	return StatusUnreachable, fmt.Sprintf("a porta %d não está publicada neste ambiente Docker", p.Port)
}

func vereditoNativo(p DevicePort) (Status, string) {
	if p.BindError != "" {
		return StatusUnreachable, fmt.Sprintf("a porta %d não pôde ser aberta: %s", p.Port, p.BindError)
	}
	if !p.Started {
		return StatusUnknown, "o emulador ainda não foi iniciado"
	}
	return StatusOK, ""
}
