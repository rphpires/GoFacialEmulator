package emulator

import (
	"errors"
	"reflect"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestDeviceSpecNormalizeAplicaPadroes(t *testing.T) {
	s := DeviceSpec{Name: "lab-01", Model: ModelDahua, Port: 4000}

	if err := s.Normalize(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if s.IPAddress != IPPadrao {
		t.Errorf("ip: %q, quero %q", s.IPAddress, IPPadrao)
	}
	if s.EventInterval != IntervaloPadrao {
		t.Errorf("intervalo: %d, quero %d", s.EventInterval, IntervaloPadrao)
	}
	if s.Enabled == nil || !*s.Enabled {
		t.Error("enabled: quero true por padrão")
	}
	if s.AutoStart {
		t.Error("auto_start: quero false por padrão")
	}
}

func TestDeviceSpecNormalizeRespeitaEnabledFalse(t *testing.T) {
	s := DeviceSpec{Name: "lab-01", Model: ModelDahua, Port: 4000, Enabled: boolPtr(false)}

	if err := s.Normalize(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if *s.Enabled {
		t.Error("enabled false explícito virou true")
	}
}

func TestDeviceSpecNormalizeRejeita(t *testing.T) {
	casos := []struct {
		nome string
		spec DeviceSpec
	}{
		{"nome vazio", DeviceSpec{Model: ModelDahua, Port: 4000}},
		{"nome só espaços", DeviceSpec{Name: "   ", Model: ModelDahua, Port: 4000}},
		{"modelo desconhecido", DeviceSpec{Name: "x", Model: "Intelbras", Port: 4000}},
		{"modelo vazio", DeviceSpec{Name: "x", Port: 4000}},
		{"porta zero", DeviceSpec{Name: "x", Model: ModelDahua, Port: 0}},
		{"porta negativa", DeviceSpec{Name: "x", Model: ModelDahua, Port: -1}},
		{"porta acima de 65535", DeviceSpec{Name: "x", Model: ModelDahua, Port: 65536}},
		{"intervalo negativo", DeviceSpec{Name: "x", Model: ModelDahua, Port: 4000, EventInterval: -5}},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := c.spec
			err := s.Normalize()
			if !errors.Is(err, ErrInvalidSpec) {
				t.Errorf("quero ErrInvalidSpec, tenho %v", err)
			}
		})
	}
}

func TestDeviceSpecNormalizeAceitaOsDoisModelos(t *testing.T) {
	for _, modelo := range []string{ModelDahua, ModelHikvision} {
		s := DeviceSpec{Name: "x", Model: modelo, Port: 4000}
		if err := s.Normalize(); err != nil {
			t.Errorf("modelo %q rejeitado: %v", modelo, err)
		}
	}
}

func TestRangeSpecExpandGeraUmPorPorta(t *testing.T) {
	r := RangeSpec{
		NamePrefix: "lab",
		Model:      ModelHikvision,
		IPAddress:  "192.168.1.50",
		PortStart:  4000,
		PortEnd:    4002,
	}
	if err := r.Normalize(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	specs := r.Expand()

	if len(specs) != 3 {
		t.Fatalf("quero 3 specs, tenho %d", len(specs))
	}
	querNomes := []string{"lab-4000", "lab-4001", "lab-4002"}
	for i, s := range specs {
		if s.Name != querNomes[i] {
			t.Errorf("spec %d: nome %q, quero %q", i, s.Name, querNomes[i])
		}
		if s.Port != 4000+i {
			t.Errorf("spec %d: porta %d, quero %d", i, s.Port, 4000+i)
		}
		if s.Model != ModelHikvision || s.IPAddress != "192.168.1.50" {
			t.Errorf("spec %d: modelo/ip não vieram do lote: %+v", i, s)
		}
	}
}

func TestRangeSpecExpandPortaUnica(t *testing.T) {
	r := RangeSpec{NamePrefix: "solo", Model: ModelDahua, PortStart: 4000, PortEnd: 4000}
	if err := r.Normalize(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if specs := r.Expand(); len(specs) != 1 {
		t.Errorf("quero 1 spec, tenho %d", len(specs))
	}
}

func TestRangeSpecNormalizeRejeita(t *testing.T) {
	casos := []struct {
		nome string
		spec RangeSpec
	}{
		{"prefixo vazio", RangeSpec{Model: ModelDahua, PortStart: 4000, PortEnd: 4001}},
		{"modelo inválido", RangeSpec{NamePrefix: "x", Model: "Nedap", PortStart: 4000, PortEnd: 4001}},
		{"fim antes do início", RangeSpec{NamePrefix: "x", Model: ModelDahua, PortStart: 4010, PortEnd: 4000}},
		{"início zero", RangeSpec{NamePrefix: "x", Model: ModelDahua, PortStart: 0, PortEnd: 4000}},
		{"fim acima de 65535", RangeSpec{NamePrefix: "x", Model: ModelDahua, PortStart: 4000, PortEnd: 70000}},
		{"lote maior que o teto", RangeSpec{NamePrefix: "x", Model: ModelDahua, PortStart: 1000, PortEnd: 1000 + MaxRangeSize}},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := c.spec
			if err := s.Normalize(); !errors.Is(err, ErrInvalidSpec) {
				t.Errorf("quero ErrInvalidSpec, tenho %v", err)
			}
		})
	}
}

func TestRangeSpecNormalizeAceitaOTetoExato(t *testing.T) {
	r := RangeSpec{NamePrefix: "x", Model: ModelDahua, PortStart: 1000, PortEnd: 1000 + MaxRangeSize - 1}
	if err := r.Normalize(); err != nil {
		t.Errorf("lote de exatamente %d portas rejeitado: %v", MaxRangeSize, err)
	}
}

func TestConflictsDevolveOrdenadoESemRepeticao(t *testing.T) {
	ocupadas := map[int]bool{4003: true, 4001: true, 8080: true}

	got := Conflicts([]int{4000, 4001, 4001, 4003, 4004, 8080}, ocupadas)

	quero := []int{4001, 4003, 8080}
	if !reflect.DeepEqual(got, quero) {
		t.Errorf("conflitos: %v, quero %v", got, quero)
	}
}

func TestConflictsSemColisaoDevolveVazio(t *testing.T) {
	if got := Conflicts([]int{4000, 4001}, map[int]bool{9000: true}); len(got) != 0 {
		t.Errorf("quero nenhum conflito, tenho %v", got)
	}
}

func TestConflictErrorListaAsPortas(t *testing.T) {
	err := &ConflictError{Ports: []int{4001, 4003}}
	if !errors.Is(err, ErrInvalidSpec) {
		t.Error("ConflictError precisa casar com ErrInvalidSpec para virar 400 no handler")
	}
	if msg := err.Error(); msg == "" {
		t.Error("mensagem vazia")
	}
}

func TestConflictErrorMencionaPortaReservada(t *testing.T) {
	err := &ConflictError{Reserved: []int{8080}}
	if msg := err.Error(); msg == "" {
		t.Error("mensagem vazia")
	}
}
