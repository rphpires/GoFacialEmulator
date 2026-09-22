package dahua

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"GoFacialEmulator/internal/trace"

	"github.com/gin-gonic/gin"
)

// Testes de fidelidade byte-a-byte com o device real (Intelbras SS 5531 MF
// EX, firmware 3.002.00IB000.0.R), medido na bancada por curl direto contra
// 192.168.1.111. Cobrem recordFinder.cgi/recordUpdater.cgi (tabela
// AccessControlCard) e AccessFace.cgi/FaceInfoManager.cgi/AccessUser.cgi.

func newCardTestEmulator() (*Emulator, *fakeStore) {
	store := newFakeStore()
	repo := NewRepository(store, 999)
	e := &Emulator{
		tracer:   trace.NewTracer(),
		repo:     repo,
		stopChan: make(chan struct{}),
	}
	e.countCardsFn = func() (int, error) { return len(store.cards), nil }
	return e, store
}

func novaData(str string) time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05", str)
	return t
}

// ---------------------------------------------------------------------------
// A. recordFinder.cgi, tabela AccessControlCard
// ---------------------------------------------------------------------------

// A1: find SEM condition.UserID devolve os primeiros `count` registros da
// tabela, IGNORANDO offset - medido na bancada com offset=0,100,260,500,12000
// devolvendo todos found=50 com o mesmo RecNo=15 na primeira posição.
func TestHandleRecordFinder_FindWithoutUserIDIgnoresOffset(t *testing.T) {
	e, store := newCardTestEmulator()
	for i := 1; i <= 5; i++ {
		store.cards = append(store.cards, Card{
			RecNo: i, CardName: fmt.Sprintf("Fulano%d", i), UserID: i, CardNo: fmt.Sprintf("CARD%d", i),
			ValidDateStart: novaData("2026-01-01 00:00:00"), ValidDateEnd: novaData("2027-01-01 00:00:00"),
		})
	}

	r := gin.New()
	r.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	for _, offset := range []string{"0", "100", "12000"} {
		url := fmt.Sprintf("/cgi-bin/recordFinder.cgi?action=find&name=AccessControlCard&count=2&offset=%s", offset)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := w.Body.String()
		if !strings.Contains(body, "found=2") || !strings.Contains(body, "records[0].RecNo=1") {
			t.Errorf("offset=%s: got %q, want found=2 com RecNo=1 na primeira posicao (offset deve ser ignorado)", offset, body)
		}
	}
}

// A1 (continuação): find COM condition.UserID continua indo por FindCard.
func TestHandleRecordFinder_FindWithUserIDFiltersByCard(t *testing.T) {
	e, store := newCardTestEmulator()
	store.cards = append(store.cards,
		Card{RecNo: 1, CardName: "A", UserID: 7, CardNo: "AAA", ValidDateStart: novaData("2026-01-01 00:00:00"), ValidDateEnd: novaData("2027-01-01 00:00:00")},
		Card{RecNo: 2, CardName: "B", UserID: 8, CardNo: "BBB", ValidDateStart: novaData("2026-01-01 00:00:00"), ValidDateEnd: novaData("2027-01-01 00:00:00")},
	)

	r := gin.New()
	r.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	req := httptest.NewRequest(http.MethodGet,
		"/cgi-bin/recordFinder.cgi?action=find&name=AccessControlCard&condition.UserID=7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "found=1") || !strings.Contains(body, "records[0].UserID=7") {
		t.Errorf("got %q, want found=1 com UserID=7", body)
	}
	if strings.Contains(body, "UserID=8") {
		t.Errorf("got %q, cartao de outro UserID nao deveria aparecer", body)
	}
}

// A2: doSeekFind pagina corretamente com offset/count e termina (found menor
// que count na última página, found=0 além do fim). GetCards já delega isso
// para LIMIT/OFFSET do Postgres - aqui confirmamos que o handler passa os
// parâmetros corretos e que o comportamento de paginação é fiel.
func TestHandleRecordFinder_DoSeekFindPaginatesAndTerminates(t *testing.T) {
	e, store := newCardTestEmulator()
	for i := 1; i <= 14; i++ {
		store.cards = append(store.cards, Card{
			RecNo: i, CardName: fmt.Sprintf("Nome%d", i), UserID: i, CardNo: fmt.Sprintf("C%d", i),
			ValidDateStart: novaData("2026-01-01 00:00:00"), ValidDateEnd: novaData("2027-01-01 00:00:00"),
		})
	}

	r := gin.New()
	r.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	casos := []struct {
		offset, count int
		wantFound     int
	}{
		{0, 10, 10},
		{10, 10, 4}, // última página parcial
		{14, 10, 0}, // além do fim
	}
	for _, c := range casos {
		url := fmt.Sprintf("/cgi-bin/recordFinder.cgi?action=doSeekFind&name=AccessControlCard&offset=%d&count=%d", c.offset, c.count)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		want := fmt.Sprintf("found=%d", c.wantFound)
		if c.wantFound == 0 {
			want = "found=0"
		}
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("offset=%d count=%d: got %q, want conter %q", c.offset, c.count, w.Body.String(), want)
		}
	}
}

// A3: startFind e stopFind -> 501 "Error\nNot Implemented!" exato.
func TestHandleRecordFinder_StartStopFindNotImplemented(t *testing.T) {
	e, _ := newCardTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	for _, action := range []string{"startFind", "stopFind"} {
		req := httptest.NewRequest(http.MethodGet,
			"/cgi-bin/recordFinder.cgi?action="+action+"&name=AccessControlCard", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotImplemented {
			t.Errorf("action=%s: status = %d, want 501", action, w.Code)
		}
		if got := w.Body.String(); got != "Error\nNot Implemented!" {
			t.Errorf("action=%s: body = %q, want %q", action, got, "Error\nNot Implemented!")
		}
	}
}

// A4: getQuerySize devolve as DUAS linhas, Size= e count=, com o mesmo valor.
func TestHandleRecordFinder_GetQuerySizeHasSizeAndCountLines(t *testing.T) {
	e, store := newCardTestEmulator()
	for i := 1; i <= 3; i++ {
		store.cards = append(store.cards, Card{RecNo: i, UserID: i, CardNo: fmt.Sprintf("C%d", i)})
	}

	r := gin.New()
	r.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	req := httptest.NewRequest(http.MethodGet,
		"/cgi-bin/recordFinder.cgi?action=getQuerySize&name=AccessControlCard", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if want := "Size=3\ncount=3"; w.Body.String() != want {
		t.Errorf("body = %q, want %q", w.Body.String(), want)
	}
}

// A5: tabela desconhecida (medido com name=AccessFace) -> 400 JSON
// {"code":285278242,"message":""}, tanto para find quanto para getQuerySize.
func TestHandleRecordFinder_UnknownTableReturnsJSONError(t *testing.T) {
	e, _ := newCardTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	for _, action := range []string{"find", "getQuerySize"} {
		req := httptest.NewRequest(http.MethodGet,
			"/cgi-bin/recordFinder.cgi?action="+action+"&name=AccessFace", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("action=%s: status = %d, want 400", action, w.Code)
		}
		want := `{"code":285278242,"message":""}`
		if got := w.Body.String(); got != want {
			t.Errorf("action=%s: body = %q, want %q", action, got, want)
		}
	}
}

// A6: find de um UserID sem cartão -> 200 found=0.
func TestHandleRecordFinder_FindUserWithNoCardsReturnsFoundZero(t *testing.T) {
	e, _ := newCardTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	req := httptest.NewRequest(http.MethodGet,
		"/cgi-bin/recordFinder.cgi?action=find&name=AccessControlCard&condition.UserID=999999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "found=0" {
		t.Errorf("body = %q, want %q", got, "found=0")
	}
}

// ---------------------------------------------------------------------------
// B. recordUpdater.cgi, tabela AccessControlCard
// ---------------------------------------------------------------------------

func inserirCartao(r *gin.Engine, userID int, cardNo string) *httptest.ResponseRecorder {
	return inserirCartaoComNome(r, userID, cardNo, "Fulano")
}

func inserirCartaoComNome(r *gin.Engine, userID int, cardNo, cardName string) *httptest.ResponseRecorder {
	q := url.Values{}
	q.Set("action", "insert")
	q.Set("CardName", cardName)
	q.Set("UserID", strconv.Itoa(userID))
	q.Set("CardNo", cardNo)
	q.Set("ValidDateStart", "2026-01-01 00:00:00")
	q.Set("ValidDateEnd", "2027-01-01 00:00:00")

	req := httptest.NewRequest(http.MethodGet, "/cgi-bin/recordUpdater.cgi?"+q.Encode(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// CardName: limite medido na bancada é 32 BYTES de UTF-8, não runas, e o
// device REJEITA (não trunca) o que passa disso - ver comentário de
// maxCardNameBytes em handlers.go.
func TestHandleRecordUpdater_InsertCardNameByteLimit(t *testing.T) {
	casos := []struct {
		nome       string
		cardName   string
		wantAceito bool
	}{
		{"32 bytes ASCII - aceito", strings.Repeat("A", 32), true},
		{"33 bytes ASCII - rejeitado", strings.Repeat("A", 33), false},
		{"16x'á' = 32 bytes - aceito", strings.Repeat("á", 16), true},
		{"32x'á' = 64 bytes - rejeitado", strings.Repeat("á", 32), false},
		{"16 'A' + 16 'á' = 48 bytes - rejeitado", strings.Repeat("A", 16) + strings.Repeat("á", 16), false},
	}

	for i, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			e, _ := newCardTestEmulator()
			r := gin.New()
			r.GET("/cgi-bin/recordUpdater.cgi", e.handleRecordUpdaterGet)

			w := inserirCartaoComNome(r, 1000+i, fmt.Sprintf("CARDLEN%d", i), c.cardName)

			if c.wantAceito {
				if w.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
				}
				if got := w.Body.String(); got != "RecNo=1" {
					t.Errorf("body = %q, want %q", got, "RecNo=1")
				}
			} else {
				if w.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
				}
				if got := w.Body.String(); got != "Error\nBad Request!" {
					t.Errorf("body = %q, want %q", got, "Error\nBad Request!")
				}
			}
		})
	}
}

// CardName não é sanitizado nem normalizado: acento, apóstrofo, parênteses,
// vírgula, ponto e hífen passam intactos, contanto que caibam no limite de
// 32 bytes. Medido byte-a-byte idêntico na bancada.
func TestHandleRecordUpdater_InsertCardNameRoundTripsAccentedText(t *testing.T) {
	nomes := []string{"Sonia Acao (teste)", "Sônia Ação", "José D'Ávila-Neto, Jr."}

	for _, nome := range nomes {
		t.Run(nome, func(t *testing.T) {
			e, store := newCardTestEmulator()
			r := gin.New()
			r.GET("/cgi-bin/recordUpdater.cgi", e.handleRecordUpdaterGet)

			if len(nome) > maxCardNameBytes {
				t.Fatalf("caso de teste invalido: %q tem %d bytes, acima do limite", nome, len(nome))
			}

			w := inserirCartaoComNome(r, 42, "RTRIP1", nome)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
			}

			if len(store.cards) != 1 || store.cards[0].CardName != nome {
				t.Errorf("CardName persistido = %q, want %q (sem sanitizacao)", store.cards[0].CardName, nome)
			}
		})
	}
}

// B1: insert com sucesso -> 200 "RecNo=<n>".
func TestHandleRecordUpdater_InsertSuccess(t *testing.T) {
	e, _ := newCardTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/recordUpdater.cgi", e.handleRecordUpdaterGet)

	w := inserirCartao(r, 1, "AAA111")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "RecNo=1" {
		t.Errorf("body = %q, want %q", got, "RecNo=1")
	}
}

// B2: um segundo insert para um UserID que já tem cartão -> 400 JSON
// {"code":286064929,"message":""}, mesmo com CardNo diferente.
func TestHandleRecordUpdater_InsertDuplicateUserIDRejected(t *testing.T) {
	e, _ := newCardTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/recordUpdater.cgi", e.handleRecordUpdaterGet)

	if w := inserirCartao(r, 1, "AAA111"); w.Code != http.StatusOK {
		t.Fatalf("insert inicial falhou: %d %s", w.Code, w.Body.String())
	}

	w := inserirCartao(r, 1, "ZZZ999")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	want := `{"code":286064929,"message":""}`
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// B3: inserir um CardNo que já pertence a outro UserID -> 400 JSON
// {"code":286064930,"message":""}.
func TestHandleRecordUpdater_InsertDuplicateCardNoRejected(t *testing.T) {
	e, _ := newCardTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/recordUpdater.cgi", e.handleRecordUpdaterGet)

	if w := inserirCartao(r, 1, "AAA111"); w.Code != http.StatusOK {
		t.Fatalf("insert inicial falhou: %d %s", w.Code, w.Body.String())
	}

	w := inserirCartao(r, 2, "AAA111")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	want := `{"code":286064930,"message":""}`
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// B4: remove é IDEMPOTENTE - RecNo inexistente devolve 200 OK, não erro e
// não 501.
func TestHandleRecordUpdater_RemoveIsIdempotent(t *testing.T) {
	e, _ := newCardTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/recordUpdater.cgi", e.handleRecordUpdaterGet)

	req := httptest.NewRequest(http.MethodGet, "/cgi-bin/recordUpdater.cgi?action=remove&recno=99999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "OK" {
		t.Errorf("body = %q, want %q", got, "OK")
	}
}

// ---------------------------------------------------------------------------
// C. Faces
// ---------------------------------------------------------------------------

func newFaceRouterEmulator() (*Emulator, *fakeStore) {
	e, store := newCardTestEmulator()
	return e, store
}

// fotoFake gera n bytes de payload "JPEG" falso (o emulador não valida o
// formato, só o tamanho) e devolve em base64.
func fotoFake(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 256)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// C1: POST AccessFace.cgi?action=insertMulti com sucesso -> 200 "OK".
func TestHandleAccessFace_InsertMultiSuccess(t *testing.T) {
	e, store := newFaceRouterEmulator()
	r := gin.New()
	r.POST("/cgi-bin/AccessFace.cgi", e.handleAccessFacePost)

	body := fmt.Sprintf(`{"FaceList":[{"UserID":"42","PhotoData":["%s"]}]}`, fotoFake(200*1024))
	req := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=insertMulti", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "OK" {
		t.Errorf("body = %q, want %q", got, "OK")
	}
	if _, ok := store.faces[42]; !ok {
		t.Error("face do UserID=42 nao foi persistida")
	}
}

// C2: reinserir face de um UserID que já tem uma -> 400 com FailCodes
// [286064926].
func TestHandleAccessFace_InsertMultiDuplicateRejected(t *testing.T) {
	e, _ := newFaceRouterEmulator()
	r := gin.New()
	r.POST("/cgi-bin/AccessFace.cgi", e.handleAccessFacePost)

	body := fmt.Sprintf(`{"FaceList":[{"UserID":"42","PhotoData":["%s"]}]}`, fotoFake(200*1024))

	// Primeira vai com sucesso.
	req := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=insertMulti", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("primeiro insert falhou: %d %s", w.Code, w.Body.String())
	}

	// Segunda para o mesmo UserID é rejeitada.
	req2 := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=insertMulti", strings.NewReader(body))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w2.Code, w2.Body.String())
	}
	want := `{"code":268632336,"detail":{"FailCodes":[286064926],"FailCount":1},"message":"Batch Process Error"}`
	if got := w2.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// C3: foto obviamente sem rosto (flat-gray de 5.425 bytes medido na
// bancada) -> 400 com FailCodes [288686087]. Este é o único caso que
// faceMinBytesAproximado ainda pega corretamente - ver o comentário da
// constante em accessface.go para a lista completa de medições e a
// divergência conhecida (43.530 bytes).
func TestHandleAccessFace_InsertMultiNoFaceDetected(t *testing.T) {
	e, _ := newFaceRouterEmulator()
	r := gin.New()
	r.POST("/cgi-bin/AccessFace.cgi", e.handleAccessFacePost)

	body := fmt.Sprintf(`{"FaceList":[{"UserID":"7","PhotoData":["%s"]}]}`, fotoFake(5425))
	req := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=insertMulti", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	want := `{"code":268632336,"detail":{"FailCodes":[288686087],"FailCount":1},"message":"Batch Process Error"}`
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// C3 (fixture real do harness): face_UT-001.jpg tem 46.352 bytes e o device
// real ACEITA essa foto (é um retrato de verdade). Com faceMinBytesAproximado
// em 64 KiB o emulador rejeitava esse fixture por engano; em 16 KiB ele é
// aceito, como deve ser.
func TestHandleAccessFace_InsertMultiAcceptsHarnessFixtureSize(t *testing.T) {
	e, store := newFaceRouterEmulator()
	r := gin.New()
	r.POST("/cgi-bin/AccessFace.cgi", e.handleAccessFacePost)

	body := fmt.Sprintf(`{"FaceList":[{"UserID":"55","PhotoData":["%s"]}]}`, fotoFake(46352))
	req := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=insertMulti", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "OK" {
		t.Errorf("body = %q, want %q", got, "OK")
	}
	if _, ok := store.faces[55]; !ok {
		t.Error("face do UserID=55 nao foi persistida")
	}
}

// Divergência conhecida e documentada (ver comentário de
// faceMinBytesAproximado): o caso de 43.530 bytes (rosto pequeno num canvas
// grande) é rejeitado pelo device real mas ACEITO pelo emulador, porque o
// emulador só olha tamanho, não conteúdo. Este teste fixa esse
// comportamento conhecido - se ele começar a falhar porque alguém "corrigiu"
// o limiar, a correção provavelmente reintroduziu a rejeição de retratos
// válidos (foi exatamente isso que o fixture de 46.352 bytes provou).
func TestHandleAccessFace_InsertMultiSmallFaceInBigCanvasIsAcceptedDivergenceFromRealDevice(t *testing.T) {
	e, store := newFaceRouterEmulator()
	r := gin.New()
	r.POST("/cgi-bin/AccessFace.cgi", e.handleAccessFacePost)

	body := fmt.Sprintf(`{"FaceList":[{"UserID":"56","PhotoData":["%s"]}]}`, fotoFake(43530))
	req := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=insertMulti", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (divergencia conhecida do device real); body=%s", w.Code, w.Body.String())
	}
	if _, ok := store.faces[56]; !ok {
		t.Error("face do UserID=56 nao foi persistida")
	}
}

// Sequência real do driver (harness): insertMulti tem sucesso, um segundo
// insertMulti para o mesmo UserID é rejeitado com 286064926, e o driver
// reenvia como updateMulti - que o device real aceita com 200 OK.
func TestHandleAccessFace_UpdateMultiAfterDuplicateSucceeds(t *testing.T) {
	e, store := newFaceRouterEmulator()
	r := gin.New()
	r.POST("/cgi-bin/AccessFace.cgi", e.handleAccessFacePost)

	corpoOriginal := fmt.Sprintf(`{"FaceList":[{"UserID":"77","PhotoData":["%s"]}]}`, fotoFake(200*1024))

	req1 := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=insertMulti", strings.NewReader(corpoOriginal))
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("insertMulti inicial falhou: %d %s", w1.Code, w1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=insertMulti", strings.NewReader(corpoOriginal))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	wantDup := `{"code":268632336,"detail":{"FailCodes":[286064926],"FailCount":1},"message":"Batch Process Error"}`
	if w2.Code != http.StatusBadRequest || w2.Body.String() != wantDup {
		t.Fatalf("segundo insertMulti: status=%d body=%q, want 400 %q", w2.Code, w2.Body.String(), wantDup)
	}

	corpoAtualizado := fmt.Sprintf(`{"FaceList":[{"UserID":"77","PhotoData":["%s"]}]}`, fotoFake(210*1024))
	req3 := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=updateMulti", strings.NewReader(corpoAtualizado))
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("updateMulti: status = %d, want 200; body=%s", w3.Code, w3.Body.String())
	}
	if got := w3.Body.String(); got != "OK" {
		t.Errorf("updateMulti: body = %q, want %q", got, "OK")
	}
	if _, ok := store.faces[77]; !ok {
		t.Error("face do UserID=77 deveria continuar persistida apos updateMulti")
	}
}

// C4: foto grande demais -> 400 com FailCodes [286064923]. Medido na
// bancada: 384.515 bytes aceito, 423.728 rejeitado.
func TestHandleAccessFace_InsertMultiTooLargeRejected(t *testing.T) {
	e, _ := newFaceRouterEmulator()
	r := gin.New()
	r.POST("/cgi-bin/AccessFace.cgi", e.handleAccessFacePost)

	body := fmt.Sprintf(`{"FaceList":[{"UserID":"9","PhotoData":["%s"]}]}`, fotoFake(423728))
	req := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=insertMulti", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	want := `{"code":268632336,"detail":{"FailCodes":[286064923],"FailCount":1},"message":"Batch Process Error"}`
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// C4 (segundo limite): um payload MUITO grande (957.047 bytes medido na
// bancada) não devolve o envelope JSON - devolve 400 puro "Error\nBad
// Request!". Reproduzido aqui com faceHardBadRequestBytes+1.
func TestHandleAccessFace_InsertMultiHardLimitReturnsPlainBadRequest(t *testing.T) {
	e, _ := newFaceRouterEmulator()
	r := gin.New()
	r.POST("/cgi-bin/AccessFace.cgi", e.handleAccessFacePost)

	body := fmt.Sprintf(`{"FaceList":[{"UserID":"9","PhotoData":["%s"]}]}`, fotoFake(faceHardBadRequestBytes+1))
	req := httptest.NewRequest(http.MethodPost, "/cgi-bin/AccessFace.cgi?action=insertMulti", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "Error\nBad Request!" {
		t.Errorf("body = %q, want %q", got, "Error\nBad Request!")
	}
}

// C6: FaceInfoManager.cgi?action=remove é IDEMPOTENTE - 200 OK mesmo quando
// o UserID não tem face cadastrada.
func TestHandleFaceInfoManagerGet_RemoveIsIdempotent(t *testing.T) {
	e, _ := newFaceRouterEmulator()
	r := gin.New()
	r.GET("/cgi-bin/FaceInfoManager.cgi", e.handleFaceInfoManagerGet)

	req := httptest.NewRequest(http.MethodGet, "/cgi-bin/FaceInfoManager.cgi?action=remove&UserID=12345", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "OK" {
		t.Errorf("body = %q, want %q", got, "OK")
	}
}

// startFindTotal extrai o campo Total do corpo JSON de
// FaceInfoManager.cgi?action=startFind.
func startFindTotal(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var resp struct{ Total int }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta de startFind nao e JSON valido: %v; body=%s", err, w.Body.String())
	}
	return resp.Total
}

// startFind&Condition.UserID=<id> deve devolver Total = 0 ou 1 PARA AQUELE
// UserID, nunca a contagem global de faces do device. Medido na bancada: o
// emulador antes devolvia uma contagem global crescente (Total subindo
// mesmo depois de um remove); o device real devolve Total=0 para um UserID
// sem face e Total=1 para um UserID com face. O gateway usa esse Total para
// decidir HasFace (spec do fabricante, seção 12.3.4).
func TestHandleFaceInfoManagerGet_StartFindTotalIsPerUser(t *testing.T) {
	e, store := newFaceRouterEmulator()
	r := gin.New()
	r.GET("/cgi-bin/FaceInfoManager.cgi", e.handleFaceInfoManagerGet)

	// Ruído: outro UserID com face cadastrada, para provar que Total não é
	// a contagem global.
	if err := e.repo.AddFace(999, "ruido"); err != nil {
		t.Fatalf("setup: AddFace(999) falhou: %v", err)
	}

	const userID = "9000077"

	// UserID sem face -> Total=0.
	req1 := httptest.NewRequest(http.MethodGet,
		"/cgi-bin/FaceInfoManager.cgi?action=startFind&Condition.UserID="+userID, nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("startFind (ausente): status = %d, want 200; body=%s", w1.Code, w1.Body.String())
	}
	if total := startFindTotal(t, w1); total != 0 {
		t.Errorf("startFind (ausente): Total = %d, want 0", total)
	}

	// Cadastra a face desse UserID -> Total=1.
	if err := e.repo.AddFace(9000077, "hash"); err != nil {
		t.Fatalf("setup: AddFace(9000077) falhou: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet,
		"/cgi-bin/FaceInfoManager.cgi?action=startFind&Condition.UserID="+userID, nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if total := startFindTotal(t, w2); total != 1 {
		t.Errorf("startFind (com face): Total = %d, want 1", total)
	}

	// Remove a face desse UserID -> Total volta a 0 (e a face de 999
	// continua intacta, provando que o remove/consulta não vazou entre
	// usuários).
	reqRemove := httptest.NewRequest(http.MethodGet,
		"/cgi-bin/FaceInfoManager.cgi?action=remove&UserID="+userID, nil)
	wRemove := httptest.NewRecorder()
	r.ServeHTTP(wRemove, reqRemove)
	if wRemove.Code != http.StatusOK || wRemove.Body.String() != "OK" {
		t.Fatalf("remove: status=%d body=%q, want 200 OK", wRemove.Code, wRemove.Body.String())
	}

	req3 := httptest.NewRequest(http.MethodGet,
		"/cgi-bin/FaceInfoManager.cgi?action=startFind&Condition.UserID="+userID, nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if total := startFindTotal(t, w3); total != 0 {
		t.Errorf("startFind (apos remove): Total = %d, want 0", total)
	}
	if _, ok := store.faces[999]; !ok {
		t.Error("face de outro UserID (999) nao deveria ter sido afetada")
	}
}

// C7: as cinco combinações abaixo devolvem 501 no device real.
func TestNotImplementedEndpoints(t *testing.T) {
	e, _ := newFaceRouterEmulator()
	r := gin.New()
	e.SetupRoutes(r)

	casos := []struct {
		method, url string
	}{
		{http.MethodGet, "/cgi-bin/AccessFace.cgi?action=get"},
		{http.MethodGet, "/cgi-bin/FaceInfoManager.cgi?action=list"},
		{http.MethodGet, "/cgi-bin/FaceInfoManager.cgi?action=find"},
		{http.MethodGet, "/cgi-bin/FaceInfoManager.cgi?action=getCollect"},
		{http.MethodGet, "/cgi-bin/AccessUser.cgi?action=remove"},
	}
	for _, c := range casos {
		req := httptest.NewRequest(c.method, c.url, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotImplemented {
			t.Errorf("%s: status = %d, want 501; body=%s", c.url, w.Code, w.Body.String())
		}
		if got := w.Body.String(); got != "Error\nNot Implemented!" {
			t.Errorf("%s: body = %q, want %q", c.url, got, "Error\nNot Implemented!")
		}
	}
}

// C8: AccessFace.cgi?action=list -> 400 "Error\nBad Request!" tanto via GET
// (query params) quanto via POST (qualquer corpo testado na bancada).
func TestHandleAccessFace_ListAlwaysBadRequest(t *testing.T) {
	e, _ := newFaceRouterEmulator()
	r := gin.New()
	e.SetupRoutes(r)

	casos := []struct {
		method, url, body string
	}{
		{http.MethodGet, "/cgi-bin/AccessFace.cgi?action=list&count=10&offset=0", ""},
		{http.MethodPost, "/cgi-bin/AccessFace.cgi?action=list", `{"UserIDList":[1,2,3]}`},
		{http.MethodPost, "/cgi-bin/AccessFace.cgi?action=list", `{}`},
	}
	for _, c := range casos {
		var req *http.Request
		if c.body != "" {
			req = httptest.NewRequest(c.method, c.url, strings.NewReader(c.body))
		} else {
			req = httptest.NewRequest(c.method, c.url, nil)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("%s %s: status = %d, want 400; body=%s", c.method, c.url, w.Code, w.Body.String())
		}
		if got := w.Body.String(); got != "Error\nBad Request!" {
			t.Errorf("%s %s: body = %q, want %q", c.method, c.url, got, "Error\nBad Request!")
		}
	}
}

// Confere que as rotas novas (AccessFace.cgi GET/POST, AccessUser.cgi GET)
// realmente foram registradas - sem isso, tudo cai no NoRoute e vira 404.
func TestSetupRoutes_RegistersAccessFaceAndAccessUser(t *testing.T) {
	e, _ := newFaceRouterEmulator()
	r := gin.New()
	e.SetupRoutes(r)

	esperadas := map[string]bool{
		"GET /cgi-bin/AccessFace.cgi":  false,
		"POST /cgi-bin/AccessFace.cgi": false,
		"GET /cgi-bin/AccessUser.cgi":  false,
	}
	for _, ri := range r.Routes() {
		chave := ri.Method + " " + ri.Path
		if _, ok := esperadas[chave]; ok {
			esperadas[chave] = true
		}
	}
	for rota, achou := range esperadas {
		if !achou {
			t.Errorf("rota nao registrada: %s", rota)
		}
	}
}
