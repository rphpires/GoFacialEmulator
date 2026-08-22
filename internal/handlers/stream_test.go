package handlers

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestWriteSSE cobre o formato do wire. É frágil de propósito: o
// EventSource do browser descarta silenciosamente um frame malformado, e
// um "\n" faltando aparece como "tempo real não funciona" sem nenhum erro
// visível dos dois lados.
func TestWriteSSE(t *testing.T) {
	var buf bytes.Buffer

	if err := writeSSE(&buf, "device", map[string]int{"id": 7}); err != nil {
		t.Fatalf("writeSSE devolveu erro: %v", err)
	}

	tem := buf.String()

	if !strings.HasPrefix(tem, "event: device\n") {
		t.Errorf("frame não começa com a linha de evento:\n%q", tem)
	}
	if !strings.HasSuffix(tem, "\n\n") {
		t.Errorf("frame não termina em linha em branco, o EventSource nunca vai despachá-lo:\n%q", tem)
	}

	linhaDados := strings.TrimSuffix(strings.SplitN(tem, "\n", 2)[1], "\n\n")
	if !strings.HasPrefix(linhaDados, "data: ") {
		t.Fatalf("segunda linha não é data:\n%q", tem)
	}

	var decodificado map[string]int
	if err := json.Unmarshal([]byte(strings.TrimPrefix(linhaDados, "data: ")), &decodificado); err != nil {
		t.Fatalf("payload não é JSON válido: %v", err)
	}
	if decodificado["id"] != 7 {
		t.Errorf("payload = %v, quero id 7", decodificado)
	}
}

// TestWriteSSE_PayloadSemQuebraDeLinha garante que o JSON serializado não
// carrega "\n" literal: uma quebra dentro de data: encerraria o frame no
// meio e entregaria JSON truncado ao cliente.
func TestWriteSSE_PayloadSemQuebraDeLinha(t *testing.T) {
	var buf bytes.Buffer

	if err := writeSSE(&buf, "device", map[string]string{"name": "Portaria\nNorte"}); err != nil {
		t.Fatalf("writeSSE devolveu erro: %v", err)
	}

	corpo := strings.TrimSuffix(buf.String(), "\n\n")
	if strings.Count(corpo, "\n") != 1 {
		t.Errorf("frame tem quebra de linha extra dentro do payload:\n%q", buf.String())
	}
}
