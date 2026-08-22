package handlers

import (
	"reflect"
	"testing"

	"GoFacialEmulator/internal/models"
)

// TestCountFleet cobre a decisão que hoje faz o header divergir da tabela:
// um dispositivo com Enabled == 0 é "disabled" mesmo que o Status gravado
// diga outra coisa, e nunca deve ser contado como stopped. A tabela já
// aplica essa regra (handlers.go, getCurrentDevicesWithFilters); os
// contadores não aplicavam.
func TestCountFleet(t *testing.T) {
	casos := []struct {
		nome    string
		devices []models.Device
		quero   FleetCounts
	}{
		{
			nome:    "frota vazia",
			devices: nil,
			quero:   FleetCounts{Total: 0, Running: 0, Stopped: 0, Disabled: 0},
		},
		{
			nome: "desabilitado não conta como parado",
			devices: []models.Device{
				{ID: 1, Enabled: 1, Status: "running"},
				{ID: 2, Enabled: 1, Status: "stopped"},
				{ID: 3, Enabled: 0, Status: "stopped"},
			},
			quero: FleetCounts{Total: 3, Running: 1, Stopped: 1, Disabled: 1},
		},
		{
			nome: "desabilitado com status running ainda é desabilitado",
			devices: []models.Device{
				{ID: 1, Enabled: 0, Status: "running"},
			},
			quero: FleetCounts{Total: 1, Running: 0, Stopped: 0, Disabled: 1},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if tem := countFleet(caso.devices); tem != caso.quero {
				t.Errorf("countFleet() = %+v, quero %+v", tem, caso.quero)
			}
		})
	}
}

// TestPaginationRange cobre o bug que deixava a paginação sem números:
// os templates iteram .page_range e nenhum handler definia a chave, então
// o range corria sobre nil e só sobravam "Anterior"/"Próxima".
func TestPaginationRange(t *testing.T) {
	casos := []struct {
		nome       string
		page       int
		totalPages int
		quero      []int
	}{
		{"página única", 1, 1, []int{1}},
		{"sem páginas devolve vazio", 1, 0, []int{}},
		{"poucas páginas mostra todas", 2, 5, []int{1, 2, 3, 4, 5}},
		{"janela no começo", 1, 20, []int{1, 2, 3, 4, 5, 6, 7}},
		{"janela no meio", 10, 20, []int{7, 8, 9, 10, 11, 12, 13}},
		{"janela no fim", 20, 20, []int{14, 15, 16, 17, 18, 19, 20}},
		{"página fora do intervalo é fixada no limite", 99, 3, []int{1, 2, 3}},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			tem := paginationRange(caso.page, caso.totalPages)
			if !reflect.DeepEqual(tem, caso.quero) {
				t.Errorf("paginationRange(%d, %d) = %v, quero %v",
					caso.page, caso.totalPages, tem, caso.quero)
			}
		})
	}
}
