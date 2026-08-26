package emulator

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Modelos que createEmulator() sabe instanciar. Qualquer outro valor
// cadastraria um dispositivo que nunca conseguiria iniciar.
const (
	ModelHikvision = "Hikvision"
	ModelDahua     = "Dahua"
)

const (
	// MaxRangeSize é a largura do range publicado no docker-compose.yml
	// (4000-4999). Um lote maior que isso cadastra emuladores que, sob
	// Docker, nascem inalcançáveis.
	MaxRangeSize = 1000

	// IPPadrao e IntervaloPadrao valem quando o payload omite os campos.
	IPPadrao        = "127.0.0.1"
	IntervaloPadrao = 10
)

// Origem de um dispositivo em service.devices.
const (
	SourceWXS    = "wxs"
	SourceManual = "manual"
)

var (
	// ErrInvalidSpec é a raiz de tudo que o cliente errou no payload —
	// o handler converte em 400. ConflictError também casa com ele.
	ErrInvalidSpec = errors.New("dados inválidos")

	// ErrSyncDisabled: o vínculo com o W-Access está desligado, ou nunca
	// foi configurado. Não é falha, é estado — 409, não 500.
	ErrSyncDisabled = errors.New("sincronização com o W-Access desligada")

	// ErrDeviceIsManaged: a verdade do dispositivo mora no W-Access, e o
	// próximo sync sobrescreveria a edição em silêncio.
	ErrDeviceIsManaged = errors.New("dispositivo é gerenciado pelo W-Access")

	// ErrDeviceRunning: o emulador guarda uma cópia de models.Device em
	// memória; editar a quente deixaria as respostas ISAPI/CGI mentindo.
	ErrDeviceRunning = errors.New("emulador está rodando")

	ErrDeviceNotFound = errors.New("dispositivo não encontrado")
)

// DeviceSpec é o corpo de POST /api/emulators e de PUT /api/emulators/:id.
// Enabled é ponteiro para distinguir "não informado" (vira true) de
// "informado como false".
type DeviceSpec struct {
	Name          string `json:"name"`
	Model         string `json:"model"`
	IPAddress     string `json:"ip_address"`
	Port          int    `json:"port"`
	EventInterval int    `json:"event_interval"`
	Enabled       *bool  `json:"enabled"`
	AutoStart     bool   `json:"auto_start"`
}

// RangeSpec é o corpo de POST /api/emulators/range. Só a porta varia entre
// os itens: o bind é 0.0.0.0, então um range é N portas no mesmo host.
type RangeSpec struct {
	NamePrefix    string `json:"name_prefix"`
	Model         string `json:"model"`
	IPAddress     string `json:"ip_address"`
	PortStart     int    `json:"port_start"`
	PortEnd       int    `json:"port_end"`
	EventInterval int    `json:"event_interval"`
	Enabled       *bool  `json:"enabled"`
	AutoStart     bool   `json:"auto_start"`
}

// ConflictError lista as portas que impediram a criação. Ports são portas
// de outros dispositivos cadastrados; Reserved são portas do próprio
// serviço, que merecem uma frase diferente porque a causa é outra.
type ConflictError struct {
	Ports    []int
	Reserved []int
}

func (e *ConflictError) Error() string {
	var partes []string
	if len(e.Ports) > 0 {
		partes = append(partes, fmt.Sprintf("portas já usadas por outros emuladores: %v", e.Ports))
	}
	if len(e.Reserved) > 0 {
		partes = append(partes,
			fmt.Sprintf("portas reservadas pelo próprio serviço: %v", e.Reserved))
	}
	if len(partes) == 0 {
		return "conflito de portas"
	}
	return strings.Join(partes, "; ")
}

// Is faz ConflictError casar com ErrInvalidSpec, para o handler mapear
// tudo que é erro de payload num 400 só.
func (e *ConflictError) Is(target error) bool { return target == ErrInvalidSpec }

func modeloValido(modelo string) bool {
	return modelo == ModelHikvision || modelo == ModelDahua
}

func portaValida(p int) bool { return p >= 1 && p <= 65535 }

// Normalize preenche os padrões e valida. Chamar antes de qualquer uso.
func (s *DeviceSpec) Normalize() error {
	s.Name = strings.TrimSpace(s.Name)
	s.IPAddress = strings.TrimSpace(s.IPAddress)

	if s.Name == "" {
		return fmt.Errorf("%w: nome é obrigatório", ErrInvalidSpec)
	}
	if !modeloValido(s.Model) {
		return fmt.Errorf("%w: modelo deve ser %q ou %q", ErrInvalidSpec, ModelHikvision, ModelDahua)
	}
	if !portaValida(s.Port) {
		return fmt.Errorf("%w: porta deve estar entre 1 e 65535", ErrInvalidSpec)
	}
	if s.EventInterval < 0 {
		return fmt.Errorf("%w: intervalo de eventos não pode ser negativo", ErrInvalidSpec)
	}

	if s.IPAddress == "" {
		s.IPAddress = IPPadrao
	}
	if s.EventInterval == 0 {
		s.EventInterval = IntervaloPadrao
	}
	if s.Enabled == nil {
		verdadeiro := true
		s.Enabled = &verdadeiro
	}
	return nil
}

// Normalize preenche os padrões do lote e valida os limites do range.
func (s *RangeSpec) Normalize() error {
	s.NamePrefix = strings.TrimSpace(s.NamePrefix)
	s.IPAddress = strings.TrimSpace(s.IPAddress)

	if s.NamePrefix == "" {
		return fmt.Errorf("%w: prefixo de nome é obrigatório", ErrInvalidSpec)
	}
	if !modeloValido(s.Model) {
		return fmt.Errorf("%w: modelo deve ser %q ou %q", ErrInvalidSpec, ModelHikvision, ModelDahua)
	}
	if !portaValida(s.PortStart) || !portaValida(s.PortEnd) {
		return fmt.Errorf("%w: portas devem estar entre 1 e 65535", ErrInvalidSpec)
	}
	if s.PortEnd < s.PortStart {
		return fmt.Errorf("%w: porta final não pode ser menor que a inicial", ErrInvalidSpec)
	}
	if tamanho := s.PortEnd - s.PortStart + 1; tamanho > MaxRangeSize {
		return fmt.Errorf("%w: lote de %d portas excede o máximo de %d",
			ErrInvalidSpec, tamanho, MaxRangeSize)
	}
	if s.EventInterval < 0 {
		return fmt.Errorf("%w: intervalo de eventos não pode ser negativo", ErrInvalidSpec)
	}

	if s.IPAddress == "" {
		s.IPAddress = IPPadrao
	}
	if s.EventInterval == 0 {
		s.EventInterval = IntervaloPadrao
	}
	if s.Enabled == nil {
		verdadeiro := true
		s.Enabled = &verdadeiro
	}
	return nil
}

// Expand converte o lote em um DeviceSpec por porta. O nome carrega a
// porta, e não um índice: a porta é o que o operador procura quando algo
// falha. Chamar só depois de Normalize.
func (s RangeSpec) Expand() []DeviceSpec {
	specs := make([]DeviceSpec, 0, s.PortEnd-s.PortStart+1)
	for porta := s.PortStart; porta <= s.PortEnd; porta++ {
		specs = append(specs, DeviceSpec{
			Name:          fmt.Sprintf("%s-%d", s.NamePrefix, porta),
			Model:         s.Model,
			IPAddress:     s.IPAddress,
			Port:          porta,
			EventInterval: s.EventInterval,
			Enabled:       s.Enabled,
			AutoStart:     s.AutoStart,
		})
	}
	return specs
}

// Conflicts devolve, ordenadas e sem repetição, as portas desejadas que já
// estão ocupadas.
func Conflicts(desejadas []int, ocupadas map[int]bool) []int {
	vistas := map[int]bool{}
	var conflitos []int
	for _, p := range desejadas {
		if ocupadas[p] && !vistas[p] {
			vistas[p] = true
			conflitos = append(conflitos, p)
		}
	}
	sort.Ints(conflitos)
	return conflitos
}
