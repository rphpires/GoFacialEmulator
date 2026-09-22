package dahua

import (
	"strings"
	"testing"
	"time"
)

func TestDahuaEventLog_BetweenFiltersByTime(t *testing.T) {
	l := newDahuaEventLog(10)
	base := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		l.Append(CardRecEntry{CreateTime: base.Add(time.Duration(i) * time.Hour), CardNo: "AA", UserID: i})
	}

	got := l.Between(base.Add(time.Hour), base.Add(3*time.Hour))
	if len(got) != 3 {
		t.Fatalf("faixa inclusiva 1h..3h: got %d, want 3", len(got))
	}
	if got[0].UserID != 1 || got[2].UserID != 3 {
		t.Errorf("janela errada: %+v", got)
	}
}

func TestDahuaEventLog_RingDropsOldest(t *testing.T) {
	l := newDahuaEventLog(3)
	base := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		l.Append(CardRecEntry{CreateTime: base.Add(time.Duration(i) * time.Minute), UserID: i})
	}
	if n := l.Count(); n != 3 {
		t.Fatalf("capacidade nao respeitada: got %d, want 3", n)
	}
	todos := l.Between(base, base.Add(time.Hour))
	if todos[0].UserID != 2 {
		t.Errorf("mais antigo retido: got UserID=%d, want 2", todos[0].UserID)
	}
}

func TestDahuaEventLog_RenderMatchesDeviceFormat(t *testing.T) {
	l := newDahuaEventLog(10)
	quando := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	recs := []CardRecEntry{{
		CreateTime: quando, CardNo: "0A0B0C0D", CardName: "Fulano",
		UserID: 7, ReaderID: 1, Method: 15, ErrorCode: 0, Status: 1,
	}}

	saida := l.Render(recs)
	if !strings.HasPrefix(saida, "found=1") {
		t.Errorf("resposta deve comecar com found=N:\n%s", saida)
	}
	for _, obrigatorio := range []string{
		"records[0].CardNo=0A0B0C0D",
		"records[0].UserID=7",
		"records[0].ReaderID=1",
		"records[0].Method=15",
		"records[0].ErrorCode=0",
	} {
		if !strings.Contains(saida, obrigatorio) {
			t.Errorf("falta %q:\n%s", obrigatorio, saida)
		}
	}
	// CreateTime é epoch inteiro: datetime.utcfromtimestamp(int(l[1])).
	esperado := "records[0].CreateTime=" + itoaTest(quando.Unix())
	if !strings.Contains(saida, esperado) {
		t.Errorf("CreateTime deve ser epoch (%s):\n%s", esperado, saida)
	}
}

func TestDahuaEventLog_RenderEmpty(t *testing.T) {
	l := newDahuaEventLog(10)
	if got := l.Render(nil); got != "found=0" {
		t.Errorf("sem registros deve devolver found=0; got %q", got)
	}
}

func itoaTest(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
