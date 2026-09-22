package hikvision

import (
	"sync"
	"time"
)

// AcsEventRecord é uma linha de InfoList na resposta de
// POST /ISAPI/AccessControl/AcsEvent. Só carrega o que o gerenciador lê em
// read_events() — IoHikvisionCommunication.py:1160-1176.
type AcsEventRecord struct {
	Time         time.Time
	Major        int
	Minor        int
	CardNo       string
	Name         string
	EmployeeNo   string
	CardReaderNo int
	DoorNo       int
	SerialNo     int
}

// eventLog guarda os eventos que o dispositivo "tem em memória" para consulta
// offline. É um buffer circular por instância de emulador: reiniciar o
// emulador zera o histórico, como um device que perdeu energia.
type eventLog struct {
	mu   sync.Mutex
	cap  int
	recs []AcsEventRecord
}

func newEventLog(capacidade int) *eventLog {
	if capacidade <= 0 {
		capacidade = 500
	}
	return &eventLog{cap: capacidade, recs: make([]AcsEventRecord, 0, capacidade)}
}

// Append acrescenta um evento, descartando o mais antigo quando lota.
func (l *eventLog) Append(rec AcsEventRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.recs) >= l.cap {
		l.recs = append(l.recs[:0], l.recs[len(l.recs)-l.cap+1:]...)
	}
	l.recs = append(l.recs, rec)
}

// Page devolve uma fatia a partir de offset (do mais antigo para o mais novo,
// que é a ordem em que o gerenciador pagina) e o total armazenado.
func (l *eventLog) Page(offset, max int) ([]AcsEventRecord, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	total := len(l.recs)
	if offset < 0 || offset >= total || max <= 0 {
		return nil, total
	}
	fim := offset + max
	if fim > total {
		fim = total
	}
	out := make([]AcsEventRecord, fim-offset)
	copy(out, l.recs[offset:fim])
	return out, total
}

// PurgeBefore remove os eventos anteriores a t e devolve quantos saíram. É o
// que o gerenciador pede com StorageCfg {"mode":"time","checkTime":...} depois
// de coletar os eventos offline (IoHikvisionCommunication.py:1210).
func (l *eventLog) PurgeBefore(t time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	mantidos := l.recs[:0]
	removidos := 0
	for _, r := range l.recs {
		if r.Time.Before(t) {
			removidos++
			continue
		}
		mantidos = append(mantidos, r)
	}
	l.recs = mantidos
	return removidos
}

// Clear esvazia o log e devolve quantos eventos foram descartados.
func (l *eventLog) Clear() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(l.recs)
	l.recs = l.recs[:0]
	return n
}

// fusoDoDispositivo é o fuso em que o emulador carimba e interpreta horários,
// como um equipamento com timezone configurado. Precisa ser o MESMO usado para
// renderizar o campo `time` dos eventos e para ler o `checkTime` do StorageCfg:
// o container roda em UTC, então cair em time.Local deslocava a janela de purga
// em 3 horas e o StorageCfg nunca apagava nada.
func fusoDoDispositivo() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.Local
	}
	return loc
}
