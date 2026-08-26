package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"GoFacialEmulator/internal/emulator"

	"github.com/gin-gonic/gin"
)

// errBancoDeTeste representa qualquer falha de infraestrutura.
var errBancoDeTeste = errors.New("connection refused")

// requisicao monta um contexto Gin com corpo JSON e parâmetros de rota.
func requisicao(t *testing.T, metodo, alvo, corpo string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(metodo, alvo, strings.NewReader(corpo))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	return c, rec
}

func TestApiCreateEmulatorRejeitaJSONQuebrado(t *testing.T) {
	h := &Handler{tracer: tracerDeTeste(t)}

	c, rec := requisicao(t, http.MethodPost, "/api/emulators", `{"name":`, nil)
	h.apiCreateEmulator(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, quero 400", rec.Code)
	}
}

func TestApiUpdateEmulatorRejeitaIDNaoNumerico(t *testing.T) {
	h := &Handler{tracer: tracerDeTeste(t)}

	c, rec := requisicao(t, http.MethodPut, "/api/emulators/abc", `{}`,
		gin.Params{{Key: "id", Value: "abc"}})
	h.apiUpdateEmulator(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, quero 400", rec.Code)
	}
}

func TestApiDeleteEmulatorRejeitaIDNaoNumerico(t *testing.T) {
	h := &Handler{tracer: tracerDeTeste(t)}

	c, rec := requisicao(t, http.MethodDelete, "/api/emulators/xyz", "",
		gin.Params{{Key: "id", Value: "xyz"}})
	h.apiDeleteEmulator(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, quero 400", rec.Code)
	}
}

// A tradução de erro de domínio em status HTTP é o contrato que a UI e
// qualquer integração consomem — vale testar cada ramo.
func TestStatusDoErroDeDominio(t *testing.T) {
	casos := []struct {
		nome  string
		err   error
		quero int
	}{
		{"spec inválida", emulator.ErrInvalidSpec, http.StatusBadRequest},
		{"conflito de porta", &emulator.ConflictError{Ports: []int{4000}}, http.StatusBadRequest},
		{"não encontrado", emulator.ErrDeviceNotFound, http.StatusNotFound},
		{"gerenciado pelo W-Access", emulator.ErrDeviceIsManaged, http.StatusConflict},
		{"emulador rodando", emulator.ErrDeviceRunning, http.StatusConflict},
		{"falha de banco", errBancoDeTeste, http.StatusInternalServerError},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := statusDoErro(c.err); got != c.quero {
				t.Errorf("status %d, quero %d", got, c.quero)
			}
		})
	}
}

// O corpo do erro de conflito precisa listar as portas: a UI monta a
// mensagem em cima dessa lista.
func TestCorpoDoErroDeConflitoListaPortas(t *testing.T) {
	h := &Handler{tracer: tracerDeTeste(t)}

	c, rec := requisicao(t, http.MethodPost, "/api/emulators", "", nil)
	h.responderErro(c, &emulator.ConflictError{Ports: []int{4001, 4003}})

	var corpo struct {
		Error     string `json:"error"`
		Conflicts []int  `json:"conflicts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo não é JSON: %v — %s", err, rec.Body.String())
	}
	if len(corpo.Conflicts) != 2 || corpo.Conflicts[0] != 4001 {
		t.Errorf("conflicts: %v, quero [4001 4003]", corpo.Conflicts)
	}
	if corpo.Error == "" {
		t.Error("quero uma mensagem legível em error")
	}
}

// As rotas de controle da frota (/start, /stop, /refresh) convivem com
// PUT|DELETE /:id no mesmo grupo. Registro de rota entra em pânico na
// subida, não na compilação: sem este teste, uma regressão aqui só
// apareceria quando o serviço não subisse.
func TestRotasDeEmuladoresConvivem(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{tracer: tracerDeTeste(t)}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registro de rotas entrou em pânico: %v", r)
		}
	}()

	router := gin.New()
	h.setupAPIRoutes(router)
}
