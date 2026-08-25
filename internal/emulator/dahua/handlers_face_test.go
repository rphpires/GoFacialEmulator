package dahua

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"GoFacialEmulator/internal/trace"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

// TestMain desliga o IO do tracer antes de qualquer teste do pacote: o
// trace.NewTracer() e singleton e sem o marcador abriria logs/trace.log
// dentro do diretorio do pacote.
func TestMain(m *testing.M) {
	_ = os.WriteFile("DisableTrace.txt", []byte(""), 0644)
	code := m.Run()
	_ = os.Remove("DisableTrace.txt")
	os.Exit(code)
}

// fakeDB registra os Exec recebidos; o resto da DBInterface nao e exercitado
// pelo caminho de gravacao de face.
type fakeDB struct {
	execArgs [][]interface{}
}

func (f *fakeDB) Query(ctx context.Context, q string, args ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (f *fakeDB) QueryRow(ctx context.Context, q string, args ...interface{}) pgx.Row { return nil }

func (f *fakeDB) Exec(ctx context.Context, q string, args ...interface{}) (pgconn.CommandTag, error) {
	f.execArgs = append(f.execArgs, args)
	return pgconn.CommandTag("INSERT 0 1"), nil
}

func (f *fakeDB) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }

func (f *fakeDB) Ping(ctx context.Context) error { return nil }

func newFaceTestEmulator(db *fakeDB, deviceID int) *Emulator {
	return &Emulator{
		tracer:   trace.NewTracer(),
		repo:     NewRepository(db, deviceID),
		stopChan: make(chan struct{}),
	}
}

// O gerenciador (W-Access) envia UserID como string JSON — "UserID": "1" —
// e sem Content-Type. Um binder que so aceita numero descarta a face com 400
// e o usuario aparece na tela como se nao tivesse face cadastrada.
func TestFaceInfoManagerPost_AceitaUserIDNumeroEString(t *testing.T) {
	gin.SetMode(gin.TestMode)

	casos := []struct {
		nome string
		body string
	}{
		{"UserID numerico", `{"UserID":1,"Info":{"PhotoData":["ZmFrZQ=="]}}`},
		{"UserID string (formato do W-Access)", `{"UserID":"1","Info":{"PhotoData":["ZmFrZQ=="]}}`},
		{"PhotoData como string solta", `{"UserID":"1","Info":{"PhotoData":"ZmFrZQ=="}}`},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			db := &fakeDB{}
			e := newFaceTestEmulator(db, 111)

			router := gin.New()
			router.POST("/cgi-bin/FaceInfoManager.cgi", e.handleFaceInfoManagerPost)

			req := httptest.NewRequest(http.MethodPost,
				"/cgi-bin/FaceInfoManager.cgi?action=add", strings.NewReader(caso.body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200; corpo=%q", rec.Code, rec.Body.String())
			}
			if len(db.execArgs) != 1 {
				t.Fatalf("gravacoes = %d, quero 1", len(db.execArgs))
			}

			args := db.execArgs[0]
			if len(args) != 3 {
				t.Fatalf("args do INSERT = %d, quero 3: %v", len(args), args)
			}
			if got, quero := args[0], 111; got != quero {
				t.Errorf("device_id = %v, quero %v", got, quero)
			}
			if got, quero := args[1], 1; got != quero {
				t.Errorf("user_id = %v, quero %v", got, quero)
			}
			if md5, ok := args[2].(string); !ok || md5 == "" {
				t.Errorf("md5_hash = %v, quero hash nao vazio", args[2])
			}
		})
	}
}
