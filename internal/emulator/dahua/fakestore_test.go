package dahua

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgproto3/v2"
	"github.com/jackc/pgx/v4"
)

// Este arquivo implementa um DBInterface fake em memória, usado pelos testes
// de repository/handlers que precisam exercitar as regras de negócio de
// cartão e face (AddCard, CheckIfFaceExists, GetCardUserIDByRecNo, etc.) sem
// um Postgres real. As queries do Repository são constantes dentro do
// pacote, então o fake resolve por trecho do texto SQL - igual ao padrão já
// usado em internal/database/fakes_test.go, só que adaptado para as tabelas
// dahua_cards/dahua_faces.

var errFakeNaoEncontrado = errors.New("fakestore: linha nao encontrada")

// fakeRow implementa pgx.Row devolvendo valores fixos ou um erro no Scan.
type fakeRow struct {
	valores []interface{}
	err     error
}

func (r fakeRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		if i >= len(r.valores) {
			continue
		}
		switch dv := d.(type) {
		case *int:
			*dv = r.valores[i].(int)
		case *string:
			*dv = r.valores[i].(string)
		case *time.Time:
			*dv = r.valores[i].(time.Time)
		default:
			return errors.New("fakestore: tipo de destino nao suportado no Scan")
		}
	}
	return nil
}

// fakeRows implementa pgx.Rows sobre uma matriz de valores.
type fakeRows struct {
	linhas [][]interface{}
	pos    int
}

func (r *fakeRows) Close()                                         {}
func (r *fakeRows) Err() error                                     { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag                  { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgproto3.FieldDescription { return nil }
func (r *fakeRows) Values() ([]interface{}, error)                 { return nil, nil }
func (r *fakeRows) RawValues() [][]byte                            { return nil }

func (r *fakeRows) Next() bool {
	if r.pos >= len(r.linhas) {
		return false
	}
	r.pos++
	return true
}

func (r *fakeRows) Scan(dest ...interface{}) error {
	return fakeRow{valores: r.linhas[r.pos-1]}.Scan(dest...)
}

// fakeStore é o DBInterface fake em memória. Guarda cartões e faces em
// slices/maps e resolve cada Query/QueryRow/Exec combinando pelo texto do
// SQL recebido.
type fakeStore struct {
	cards []Card
	faces map[int]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{faces: map[int]string{}}
}

func (f *fakeStore) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	switch {
	case strings.Contains(sql, "dahua_cards") && strings.Contains(sql, "LIMIT"):
		// GetCards(count, offset): args = [deviceID, count, offset].
		count := args[1].(int)
		offset := args[2].(int)

		ordenados := append([]Card{}, f.cards...)
		sort.Slice(ordenados, func(i, j int) bool { return ordenados[i].RecNo < ordenados[j].RecNo })

		var linhas [][]interface{}
		for i, c := range ordenados {
			if i < offset {
				continue
			}
			if count > 0 && len(linhas) >= count {
				break
			}
			linhas = append(linhas, []interface{}{c.RecNo, c.CardName, c.UserID, c.CardNo, c.ValidDateStart, c.ValidDateEnd})
		}
		return &fakeRows{linhas: linhas}, nil

	case strings.Contains(sql, "dahua_cards") && strings.Contains(sql, "user_id = $2"):
		// FindCard(userID): args = [deviceID, userID].
		userID := args[1].(int)
		var linhas [][]interface{}
		for _, c := range f.cards {
			if c.UserID == userID {
				linhas = append(linhas, []interface{}{c.RecNo, c.CardName, c.UserID, c.CardNo, c.ValidDateStart, c.ValidDateEnd})
			}
		}
		return &fakeRows{linhas: linhas}, nil

	default:
		return &fakeRows{}, nil
	}
}

func (f *fakeStore) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(sql, "dahua_faces") && strings.Contains(sql, "COUNT(*)"):
		// CheckIfFaceExists(userID): args = [deviceID, userID].
		userID := args[1].(int)
		n := 0
		if _, ok := f.faces[userID]; ok {
			n = 1
		}
		return fakeRow{valores: []interface{}{n}}

	case strings.Contains(sql, "COALESCE(MIN"):
		// GetNextRecNo(): args = [deviceID].
		usados := map[int]bool{}
		for _, c := range f.cards {
			usados[c.RecNo] = true
		}
		next := 1
		for usados[next] {
			next++
		}
		return fakeRow{valores: []interface{}{next}}

	case strings.Contains(sql, "SELECT user_id FROM emulator.dahua_cards"):
		// GetCardUserIDByRecNo(recNo): args = [deviceID, recNo].
		recNo := args[1].(int)
		for _, c := range f.cards {
			if c.RecNo == recNo {
				return fakeRow{valores: []interface{}{c.UserID}}
			}
		}
		return fakeRow{err: errFakeNaoEncontrado}

	case strings.Contains(sql, "card_no = $2") && strings.Contains(sql, "user_id <> $3"):
		// cardNoBelongsToOtherUser(cardNo, userID): args = [deviceID, cardNo, userID].
		cardNo := args[1].(string)
		userID := args[2].(int)
		n := 0
		for _, c := range f.cards {
			if c.CardNo == cardNo && c.UserID != userID {
				n++
			}
		}
		return fakeRow{valores: []interface{}{n}}

	case strings.Contains(sql, "dahua_cards") && strings.Contains(sql, "COUNT(*)"):
		// cardExistsForUserID(userID): args = [deviceID, userID].
		userID := args[1].(int)
		n := 0
		for _, c := range f.cards {
			if c.UserID == userID {
				n++
			}
		}
		return fakeRow{valores: []interface{}{n}}

	default:
		return fakeRow{valores: []interface{}{0}}
	}
}

func (f *fakeStore) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	switch {
	case strings.Contains(sql, "INSERT INTO emulator.dahua_cards"):
		// args = [deviceID, recNo, cardName, userID, cardNo, validDateStart, validDateEnd].
		f.cards = append(f.cards, Card{
			RecNo:          args[1].(int),
			CardName:       args[2].(string),
			UserID:         args[3].(int),
			CardNo:         args[4].(string),
			ValidDateStart: args[5].(time.Time),
			ValidDateEnd:   args[6].(time.Time),
		})

	case strings.Contains(sql, "DELETE FROM emulator.dahua_cards"):
		// args = [deviceID, recNo].
		recNo := args[1].(int)
		var restantes []Card
		for _, c := range f.cards {
			if c.RecNo != recNo {
				restantes = append(restantes, c)
			}
		}
		f.cards = restantes

	case strings.Contains(sql, "INSERT INTO emulator.dahua_faces"):
		// args = [deviceID, userID, md5].
		f.faces[args[1].(int)] = args[2].(string)

	case strings.Contains(sql, "DELETE FROM emulator.dahua_faces"):
		// args = [deviceID, userID].
		delete(f.faces, args[1].(int))
	}

	return pgconn.CommandTag("OK"), nil
}

func (f *fakeStore) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }
func (f *fakeStore) Ping(ctx context.Context) error            { return nil }
