package dahua

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// CardRecEntry é uma linha da tabela AccessControlCardRec — o histórico de
// acessos que o gerenciador varre em read_events() quando o device esteve
// offline (IoDahuaCommunication.py:852).
type CardRecEntry struct {
	CreateTime time.Time
	CardNo     string
	CardName   string
	UserID     int
	ReaderID   int
	Method     int
	ErrorCode  int
	Status     int
}

// dahuaEventLog é o histórico em memória, por instância de emulador.
// Reiniciar o emulador zera o histórico, como um device que perdeu energia.
type dahuaEventLog struct {
	mu   sync.Mutex
	cap  int
	recs []CardRecEntry
}

func newDahuaEventLog(capacidade int) *dahuaEventLog {
	if capacidade <= 0 {
		capacidade = 500
	}
	return &dahuaEventLog{cap: capacidade, recs: make([]CardRecEntry, 0, capacidade)}
}

func (l *dahuaEventLog) Append(e CardRecEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.recs) >= l.cap {
		l.recs = append(l.recs[:0], l.recs[len(l.recs)-l.cap+1:]...)
	}
	l.recs = append(l.recs, e)
}

// Between devolve os registros com CreateTime na faixa [inicio, fim],
// inclusiva nas duas pontas — o gerenciador manda StartTime/EndTime em epoch.
func (l *dahuaEventLog) Between(inicio, fim time.Time) []CardRecEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	var out []CardRecEntry
	for _, r := range l.recs {
		if r.CreateTime.Before(inicio) || r.CreateTime.After(fim) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Count devolve quantos registros existem (usado por getQuerySize).
func (l *dahuaEventLog) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recs)
}

// Render serializa no formato de tabela do Dahua: uma linha por campo, com
// prefixo records[i]. O parser do gerenciador faz line.split('=') e corta tudo
// antes do primeiro ponto — IoDahuaCommunication.py:856-870.
func (l *dahuaEventLog) Render(recs []CardRecEntry) string {
	if len(recs) == 0 {
		return "found=0"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "found=%d\r\n", len(recs))
	for i, r := range recs {
		fmt.Fprintf(&b, "records[%d].CardName=%s\r\n", i, r.CardName)
		fmt.Fprintf(&b, "records[%d].CardNo=%s\r\n", i, r.CardNo)
		fmt.Fprintf(&b, "records[%d].CardType=0\r\n", i)
		fmt.Fprintf(&b, "records[%d].CreateTime=%d\r\n", i, r.CreateTime.Unix())
		fmt.Fprintf(&b, "records[%d].Door=0\r\n", i)
		fmt.Fprintf(&b, "records[%d].ErrorCode=%d\r\n", i, r.ErrorCode)
		fmt.Fprintf(&b, "records[%d].Method=%d\r\n", i, r.Method)
		fmt.Fprintf(&b, "records[%d].ReaderID=%d\r\n", i, r.ReaderID)
		fmt.Fprintf(&b, "records[%d].RecNo=%d\r\n", i, i+1)
		fmt.Fprintf(&b, "records[%d].Status=%d\r\n", i, r.Status)
		fmt.Fprintf(&b, "records[%d].UserID=%d\r\n", i, r.UserID)
		fmt.Fprintf(&b, "records[%d].UserType=0\r\n", i)
	}
	return b.String()
}
