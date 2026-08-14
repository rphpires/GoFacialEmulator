package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgconn"
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
// Só QueryRow e Exec importam; Query, Begin e Ping existem para satisfazer
// a interface e devolvem zero.
type dbFalso struct {
	linha       linhaFalsa
	execChamado bool
	execArgs    []interface{}
}

func (d *dbFalso) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return d.linha
}

func (d *dbFalso) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	d.execChamado = true
	d.execArgs = args
	return pgconn.CommandTag{}, nil
}

func (d *dbFalso) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return nil, nil
}
func (d *dbFalso) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }
func (d *dbFalso) Ping(ctx context.Context) error            { return nil }

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
