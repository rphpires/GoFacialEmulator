package handlers

import (
	"encoding/json"
	"net/http"
)

// APIHandler gerencia as rotas da API
type APIHandler struct {
	// Adicione dependências necessárias aqui
}

// NewAPIHandler cria uma nova instância de APIHandler
func NewAPIHandler() *APIHandler {
	return &APIHandler{}
}

// RegisterRoutes registra as rotas da API no router fornecido
func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/status", h.getStatus)
	// Adicione mais rotas conforme necessário
}

// getStatus retorna o status do serviço
func (h *APIHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"status": "running",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
