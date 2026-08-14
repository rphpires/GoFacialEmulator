package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"GoFacialEmulator/internal/trace"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgproto3/v2"
	"github.com/jackc/pgx/v4"
)

// linhaFalsa devolve um valor fixo (ou um erro) no Scan, para exercitar
// getDeviceMode sem banco.
type linhaFalsa struct {
	valor string
	err   error
}

func (l linhaFalsa) Scan(dest ...any) error {
	if l.err != nil {
		return l.err
	}
	p, ok := dest[0].(*string)
	if !ok {
		return errors.New("destino não é *string")
	}
	*p = l.valor
	return nil
}

// dbFalso implementa database.DBInterface para os testes deste arquivo.
// QueryRow, Exec e Query importam; Begin e Ping existem para satisfazer a
// interface e devolvem zero.
type dbFalso struct {
	linha        linhaFalsa
	execChamado  bool
	execArgs     []interface{}
	execErr      error
	queryChamado bool
	queryArgs    []interface{}
	queryRows    pgx.Rows
	queryErr     error
}

func (d *dbFalso) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return d.linha
}

func (d *dbFalso) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	d.execChamado = true
	d.execArgs = args
	return pgconn.CommandTag{}, d.execErr
}

func (d *dbFalso) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	d.queryChamado = true
	d.queryArgs = args
	return d.queryRows, d.queryErr
}
func (d *dbFalso) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }
func (d *dbFalso) Ping(ctx context.Context) error            { return nil }

// linhaModo é uma linha (device_id, value) devolvida pela consulta de
// getDeviceModes.
type linhaModo struct {
	id    int
	valor string
}

// rowsFalsas implementa pgx.Rows para exercitar getDeviceModes sem banco.
type rowsFalsas struct {
	linhas []linhaModo
	pos    int
	err    error
}

func (r *rowsFalsas) Close()                                         {}
func (r *rowsFalsas) Err() error                                     { return r.err }
func (r *rowsFalsas) CommandTag() pgconn.CommandTag                  { return pgconn.CommandTag{} }
func (r *rowsFalsas) FieldDescriptions() []pgproto3.FieldDescription { return nil }
func (r *rowsFalsas) Values() ([]interface{}, error)                 { return nil, nil }
func (r *rowsFalsas) RawValues() [][]byte                            { return nil }

func (r *rowsFalsas) Next() bool {
	if r.pos >= len(r.linhas) {
		return false
	}
	r.pos++
	return true
}

func (r *rowsFalsas) Scan(dest ...interface{}) error {
	linha := r.linhas[r.pos-1]
	id, ok := dest[0].(*int)
	if !ok {
		return errors.New("destino[0] não é *int")
	}
	valor, ok := dest[1].(*string)
	if !ok {
		return errors.New("destino[1] não é *string")
	}
	*id = linha.id
	*valor = linha.valor
	return nil
}

// execArgsTexto junta os argumentos do Exec em uma string, para que o
// teste possa afirmar sobre o valor gravado sem depender da posição.
func (d *dbFalso) execArgsTexto() string {
	partes := make([]string, 0, len(d.execArgs))
	for _, a := range d.execArgs {
		partes = append(partes, fmt.Sprint(a))
	}
	return strings.Join(partes, " ")
}

// TestGetDeviceMode traduz o LocalAuthentication do banco para o
// vocabulário da interface. A tradução é o ponto: "0" e "1" não dizem nada
// para quem opera.
func TestGetDeviceMode(t *testing.T) {
	casos := []struct {
		nome  string
		valor string
		quero string
	}{
		{"zero é online", "0", "online"},
		{"um é standalone", "1", "standalone"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := &Handler{serviceDB: &dbFalso{linha: linhaFalsa{valor: c.valor}}}
			tenho, err := h.getDeviceMode(context.Background(), 1)
			if err != nil {
				t.Fatalf("getDeviceMode: %v", err)
			}
			if tenho != c.quero {
				t.Errorf("modo = %q, quero %q", tenho, c.quero)
			}
		})
	}
}

// TestGetDeviceModeSemLinha é o caso mais importante deste arquivo: quase
// todo dispositivo não tem linha em emulator.device_settings (o seed da
// migration grava LocalAuthentication só em device_id = 0, uma linha global
// que uma leitura por dispositivo nunca casa). Repository.GetSetting trata
// a ausência de linha como "1" (standalone). A tela precisa dizer a mesma
// coisa que o emulador realmente faz: nem erro, nem "online".
func TestGetDeviceModeSemLinha(t *testing.T) {
	h := &Handler{serviceDB: &dbFalso{linha: linhaFalsa{err: pgx.ErrNoRows}}}
	tenho, err := h.getDeviceMode(context.Background(), 1)
	if err != nil {
		t.Fatalf("getDeviceMode: %v", err)
	}
	if tenho != modoStandalone {
		t.Errorf("modo = %q, quero %q", tenho, modoStandalone)
	}
}

// TestGetDeviceModeErroDeBanco confere que um erro de banco genuíno (não
// pgx.ErrNoRows) continua sendo propagado como erro, e não mascarado como
// um modo qualquer.
func TestGetDeviceModeErroDeBanco(t *testing.T) {
	erroBanco := errors.New("conexão perdida")
	h := &Handler{serviceDB: &dbFalso{linha: linhaFalsa{err: erroBanco}}}

	_, err := h.getDeviceMode(context.Background(), 1)
	if err == nil {
		t.Fatal("getDeviceMode não retornou erro para uma falha de banco")
	}
	if !errors.Is(err, erroBanco) {
		t.Errorf("erro = %v, quero que contenha %v", err, erroBanco)
	}
}

// TestGetDeviceModes cobre a leitura em lote usada pela listagem de
// dispositivos: sem isso, a tela faria uma consulta por dispositivo dentro
// do laço — centenas de idas ao banco a cada carregamento de página.
func TestGetDeviceModes(t *testing.T) {
	t.Run("lista vazia não consulta o banco", func(t *testing.T) {
		db := &dbFalso{}
		h := &Handler{serviceDB: db}

		modos, err := h.getDeviceModes(context.Background(), nil)
		if err != nil {
			t.Fatalf("getDeviceModes: %v", err)
		}
		if len(modos) != 0 {
			t.Errorf("modos = %v, quero mapa vazio", modos)
		}
		if db.queryChamado {
			t.Error("o banco foi consultado mesmo com a lista de ids vazia")
		}
	})

	t.Run("dois dispositivos: 0 vira online, 1 vira standalone", func(t *testing.T) {
		db := &dbFalso{queryRows: &rowsFalsas{linhas: []linhaModo{
			{id: 1, valor: "0"},
			{id: 2, valor: "1"},
		}}}
		h := &Handler{serviceDB: db}

		modos, err := h.getDeviceModes(context.Background(), []int32{1, 2})
		if err != nil {
			t.Fatalf("getDeviceModes: %v", err)
		}
		if modos[1] != "online" {
			t.Errorf("modos[1] = %q, quero %q", modos[1], "online")
		}
		if modos[2] != "standalone" {
			t.Errorf("modos[2] = %q, quero %q", modos[2], "standalone")
		}
		if !db.queryChamado {
			t.Error("o banco não foi consultado")
		}
	})

	t.Run("dispositivo sem linha fica ausente do mapa", func(t *testing.T) {
		// Só o dispositivo 1 tem linha em emulator.device_settings; o 2 é
		// pedido mas não devolvido pela consulta — quem chama usa o padrão
		// standalone, o mesmo recuo de getDeviceMode.
		db := &dbFalso{queryRows: &rowsFalsas{linhas: []linhaModo{
			{id: 1, valor: "0"},
		}}}
		h := &Handler{serviceDB: db}

		modos, err := h.getDeviceModes(context.Background(), []int32{1, 2})
		if err != nil {
			t.Fatalf("getDeviceModes: %v", err)
		}
		if _, ok := modos[2]; ok {
			t.Errorf("modos = %v, dispositivo 2 não deveria aparecer no mapa", modos)
		}
		if len(modos) != 1 {
			t.Errorf("modos = %v, quero só o dispositivo 1", modos)
		}
	})
}

// TestSetDeviceModeRecusaValorInvalido: um modo desconhecido não pode
// chegar ao banco, senão grava lixo em LocalAuthentication.
func TestSetDeviceModeRecusaValorInvalido(t *testing.T) {
	db := &dbFalso{}
	h := &Handler{serviceDB: db}

	err := h.setDeviceMode(context.Background(), 1, "turbo")
	if err == nil {
		t.Fatal("setDeviceMode(\"turbo\") = nil, quero erro")
	}
	if db.execChamado {
		t.Error("o banco foi chamado com um modo inválido")
	}
}

// TestSetDeviceModeGravaOValorCerto confere a tradução na direção oposta e
// que o id do dispositivo chega até a query.
func TestSetDeviceModeGravaOValorCerto(t *testing.T) {
	casos := []struct {
		modo  string
		quero string
	}{
		{"online", "0"},
		{"standalone", "1"},
	}

	for _, c := range casos {
		t.Run(c.modo, func(t *testing.T) {
			db := &dbFalso{}
			h := &Handler{serviceDB: db}

			if err := h.setDeviceMode(context.Background(), 7, c.modo); err != nil {
				t.Fatalf("setDeviceMode: %v", err)
			}
			if !db.execChamado {
				t.Fatal("o banco não foi chamado")
			}
			if !strings.Contains(db.execArgsTexto(), c.quero) {
				t.Errorf("argumentos = %s, quero conter %q", db.execArgsTexto(), c.quero)
			}
			if !strings.Contains(db.execArgsTexto(), "7") {
				t.Errorf("argumentos = %s, quero que o id do dispositivo (7) apareça", db.execArgsTexto())
			}
		})
	}
}

// tracerDeTeste devolve um *trace.Tracer utilizável em testes deste
// pacote, sem gravar nada em disco. trace.NewTracer() é um singleton
// (sync.Once): a primeira chamada no processo decide o comportamento para
// sempre. tracer.init() só evita abrir logs/trace.log quando encontra um
// arquivo-marcador (DisableTrace.txt) no diretório de trabalho atual — o
// mesmo mecanismo já usado por
// internal/emulator/hikvision/test_helpers_test.go. Por isso escrevemos o
// marcador antes da primeira chamada e o removemos logo em seguida: nenhum
// arquivo fica no repositório depois do teste.
func tracerDeTeste(t *testing.T) *trace.Tracer {
	t.Helper()
	const marcador = "DisableTrace.txt"
	if err := os.WriteFile(marcador, []byte(""), 0644); err != nil {
		t.Fatalf("escrevendo marcador do tracer: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(marcador) })
	return trace.NewTracer()
}

// TestApiSetDeviceModeHTTP dirige o handler de verdade (apiSetDeviceMode)
// através de um motor gin com httptest, cobrindo o contrato HTTP que a
// finding do fix round 1 corrigiu: modo inválido não pode expor erro de
// driver nem virar 500, id inválido continua 400, e um pedido válido grava
// e responde 200 com o modo já traduzido.
func TestApiSetDeviceModeHTTP(t *testing.T) {
	t.Run("modo invalido responde 400 em portugues, sem tocar no banco", func(t *testing.T) {
		db := &dbFalso{}
		h := &Handler{serviceDB: db, tracer: tracerDeTeste(t)}
		r := gin.New()
		r.POST("/api/devices/:id/mode", h.apiSetDeviceMode)

		req := httptest.NewRequest(http.MethodPost, "/api/devices/1/mode", strings.NewReader(`{"mode":"turbo"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("HTTP status = %d, quero %d — corpo: %s", w.Code, http.StatusBadRequest, w.Body.String())
		}
		corpo := w.Body.String()
		if !strings.Contains(corpo, "Modo inválido") {
			t.Errorf("corpo = %s, quero a mensagem em português sobre modo inválido", corpo)
		}
		if strings.Contains(corpo, `"turbo"`) {
			t.Errorf("corpo = %s, não deveria ecoar o valor cru enviado pelo cliente", corpo)
		}
		if db.execChamado {
			t.Error("o banco foi chamado mesmo com um modo inválido")
		}
	})

	t.Run("id nao numerico responde 400", func(t *testing.T) {
		db := &dbFalso{}
		h := &Handler{serviceDB: db, tracer: tracerDeTeste(t)}
		r := gin.New()
		r.POST("/api/devices/:id/mode", h.apiSetDeviceMode)

		req := httptest.NewRequest(http.MethodPost, "/api/devices/abc/mode", strings.NewReader(`{"mode":"online"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("HTTP status = %d, quero %d — corpo: %s", w.Code, http.StatusBadRequest, w.Body.String())
		}
		if db.execChamado {
			t.Error("o banco foi chamado mesmo com um id inválido")
		}
	})

	t.Run("modo valido grava e responde 200 com o modo traduzido", func(t *testing.T) {
		db := &dbFalso{}
		h := &Handler{serviceDB: db, tracer: tracerDeTeste(t)}
		r := gin.New()
		r.POST("/api/devices/:id/mode", h.apiSetDeviceMode)

		req := httptest.NewRequest(http.MethodPost, "/api/devices/1/mode", strings.NewReader(`{"mode":"standalone"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("HTTP status = %d, quero %d — corpo: %s", w.Code, http.StatusOK, w.Body.String())
		}
		if !db.execChamado {
			t.Error("o banco não foi chamado para um modo válido")
		}
		corpo := w.Body.String()
		if !strings.Contains(corpo, `"mode":"standalone"`) {
			t.Errorf("corpo = %s, quero {\"mode\":\"standalone\"}", corpo)
		}
	})

	t.Run("falha de banco responde 500 generico em portugues, sem vazar erro de driver", func(t *testing.T) {
		erroDriver := errors.New("pq: connection refused at 10.0.0.5:5432")
		db := &dbFalso{execErr: erroDriver}
		h := &Handler{serviceDB: db, tracer: tracerDeTeste(t)}
		r := gin.New()
		r.POST("/api/devices/:id/mode", h.apiSetDeviceMode)

		req := httptest.NewRequest(http.MethodPost, "/api/devices/1/mode", strings.NewReader(`{"mode":"online"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("HTTP status = %d, quero %d — corpo: %s", w.Code, http.StatusInternalServerError, w.Body.String())
		}
		corpo := w.Body.String()
		if strings.Contains(corpo, "connection refused") || strings.Contains(corpo, "pq:") {
			t.Errorf("corpo = %s, não deveria vazar a string crua do driver", corpo)
		}
		if !strings.Contains(corpo, "Não foi possível trocar o modo do dispositivo") {
			t.Errorf("corpo = %s, quero a mensagem genérica em português", corpo)
		}
	})
}
