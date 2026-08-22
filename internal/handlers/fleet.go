package handlers

import "GoFacialEmulator/internal/models"

// paginationWindow é quantos números de página a barra mostra de uma vez.
// Ímpar de propósito: a página atual fica centrada, com a mesma quantidade
// de vizinhos de cada lado.
const paginationWindow = 7

// FleetCounts agrega a frota nos três estados que a tabela de dispositivos
// já distingue. Antes o header agregava só dois e "disabled" era somado a
// "stopped", o que fazia o topo da página discordar da própria tabela
// logo abaixo.
type FleetCounts struct {
	Total    int `json:"total"`
	Running  int `json:"running"`
	Stopped  int `json:"stopped"`
	Disabled int `json:"disabled"`
}

// countFleet classifica cada dispositivo em exatamente um estado.
// Enabled == 0 vence o Status gravado: um dispositivo desabilitado no
// W-Access não está "parado", está fora de operação, e a UI precisa
// mostrar essa diferença porque as ações disponíveis mudam.
func countFleet(devices []models.Device) FleetCounts {
	counts := FleetCounts{Total: len(devices)}

	for _, device := range devices {
		switch {
		case device.Enabled == 0:
			counts.Disabled++
		case device.Status == "running":
			counts.Running++
		default:
			counts.Stopped++
		}
	}

	return counts
}

// toMap devolve as contagens no formato que os templates consomem via
// {{ .counter_cards.running }}.
func (f FleetCounts) toMap() map[string]int {
	return map[string]int{
		"total":    f.Total,
		"running":  f.Running,
		"stopped":  f.Stopped,
		"disabled": f.Disabled,
	}
}

// paginationRange devolve os números de página a renderizar, centrados na
// página atual e limitados a paginationWindow. Devolve sempre uma fatia
// não-nil para o template poder iterar sem checagem.
func paginationRange(page, totalPages int) []int {
	if totalPages < 1 {
		return []int{}
	}

	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := page - paginationWindow/2
	if start < 1 {
		start = 1
	}

	end := start + paginationWindow - 1
	if end > totalPages {
		end = totalPages
		start = end - paginationWindow + 1
		if start < 1 {
			start = 1
		}
	}

	pages := make([]int, 0, end-start+1)
	for p := start; p <= end; p++ {
		pages = append(pages, p)
	}

	return pages
}
