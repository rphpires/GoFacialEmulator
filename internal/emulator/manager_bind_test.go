package emulator

import (
	"errors"
	"testing"
)

// TestLastStartError garante que o erro de abertura de porta sobrevive à
// chamada de start. Sem isso, "address already in use" morre no log e a
// tela de dispositivos não tem como avisar que aquele emulador não está
// alcançável.
func TestLastStartError(t *testing.T) {
	m := &Manager{}

	if got := m.LastStartError(42); got != "" {
		t.Errorf("antes de qualquer start: %q, quero vazio", got)
	}

	m.recordStartResult(42, errors.New("listen tcp 0.0.0.0:4200: address already in use"))
	if got := m.LastStartError(42); got == "" {
		t.Error("depois de um start com erro: vazio, quero a mensagem do erro")
	}

	m.recordStartResult(42, nil)
	if got := m.LastStartError(42); got != "" {
		t.Errorf("depois de um start bem-sucedido: %q, quero vazio", got)
	}
}

// TestLastStartErrorIsolaDispositivos: o erro de um dispositivo não pode
// contaminar o veredito de outro.
func TestLastStartErrorIsolaDispositivos(t *testing.T) {
	m := &Manager{}
	m.recordStartResult(1, errors.New("boom"))
	m.recordStartResult(2, nil)

	if m.LastStartError(1) == "" {
		t.Error("dispositivo 1: quero erro registrado")
	}
	if got := m.LastStartError(2); got != "" {
		t.Errorf("dispositivo 2: %q, quero vazio", got)
	}
}
