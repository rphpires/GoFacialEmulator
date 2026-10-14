// Package devicelog grava o log de um emulador específico: conexões,
// requisições recebidas e eventos enviados ao gerenciador. Liga e desliga
// pela flag log_enabled do dispositivo; desligar apaga os arquivos.
//
// Todo método aceita receptor nil e vira no-op, para os emuladores não
// precisarem checar se receberam um logger.
package devicelog

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// ErrNoLogs indica que o dispositivo não tem nenhum arquivo de log no disco.
var ErrNoLogs = errors.New("devicelog: nenhum arquivo de log")

// Options define onde e quanto guardar.
type Options struct {
	Dir      string // pasta dos arquivos
	MaxSize  int64  // tamanho de um arquivo antes de rotacionar
	MaxFiles int    // ativo + rotacionados
}

// DefaultOptions: logs/devices, 8 arquivos de 3MB.
func DefaultOptions() Options {
	return Options{Dir: filepath.Join("logs", "devices"), MaxSize: 3 << 20, MaxFiles: 8}
}

// FilePath devolve o n-ésimo arquivo do dispositivo: 0 é o ativo, 1 o
// rotacionado mais novo.
func (o Options) FilePath(id, n int) string {
	if n == 0 {
		return filepath.Join(o.Dir, fmt.Sprintf("device-%d.log", id))
	}
	return filepath.Join(o.Dir, fmt.Sprintf("device-%d.%d.log", id, n))
}

// Files devolve os arquivos existentes do dispositivo, do mais novo ao mais
// velho.
func (o Options) Files(id int) []string {
	var out []string
	for n := 0; n < o.MaxFiles; n++ {
		p := o.FilePath(id, n)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func (o Options) removeAll(id int) error {
	var primeiro error
	for n := 0; n < o.MaxFiles; n++ {
		err := os.Remove(o.FilePath(id, n))
		if err != nil && !errors.Is(err, os.ErrNotExist) && primeiro == nil {
			primeiro = err
		}
	}
	return primeiro
}

// Info identifica o dispositivo no cabeçalho de cada arquivo.
type Info struct {
	ID   int
	Name string
	Type string
	Port int
}

// Logger é o log de um dispositivo.
type Logger struct {
	opts    Options
	onError func(error)
	now     func() time.Time
	enabled atomic.Bool

	mu                sync.Mutex
	info              Info
	file              *os.File
	size              int64
	failed            bool
	rotateErrReported bool
	conns             map[string]*connInfo
	streams           map[string]*streamInfo
}

// New cria o logger desligado. onError recebe falhas de disco (pode ser nil).
func New(opts Options, info Info, onError func(error)) *Logger {
	if opts.MaxFiles < 2 {
		opts.MaxFiles = 2
	}
	return &Logger{
		opts:    opts,
		info:    info,
		onError: onError,
		now:     time.Now,
		conns:   map[string]*connInfo{},
		streams: map[string]*streamInfo{},
	}
}

// Enabled diz se o log está ligado.
func (l *Logger) Enabled() bool {
	return l != nil && l.enabled.Load()
}

// SetInfo atualiza nome/porta para o próximo cabeçalho.
func (l *Logger) SetInfo(info Info) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.info = info
	l.mu.Unlock()
}

// SetEnabled liga ou desliga. Ligar de novo depois de uma falha de disco
// tenta outra vez. Desligar fecha e apaga todos os arquivos.
func (l *Logger) SetEnabled(on bool) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if on {
		if l.enabled.Load() && !l.failed {
			return nil
		}
		l.failed = false
		l.enabled.Store(true)
		l.writeLocked("LOG", "ligado")
		return nil
	}

	l.enabled.Store(false)
	l.closeLocked()
	l.failed = false
	l.conns = map[string]*connInfo{}
	l.streams = map[string]*streamInfo{}
	return l.opts.removeAll(l.info.ID)
}

// Printf grava uma linha "data hora TIPO mensagem".
func (l *Logger) Printf(kind, format string, args ...interface{}) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeLocked(kind, fmt.Sprintf(format, args...))
}

// writeLocked exige l.mu. Re-checa enabled porque um SetEnabled(false) pode
// ter rodado entre o Enabled() do chamador e o lock.
func (l *Logger) writeLocked(kind, msg string) {
	if !l.enabled.Load() || l.failed {
		return
	}
	line := l.now().Format("2006-01-02 15:04:05.000") + " " + kind + " " + msg + "\n"

	if l.file == nil {
		if err := l.openLocked(); err != nil {
			l.fail(err)
			return
		}
	}
	if l.size > 0 && l.size+int64(len(line)) > l.opts.MaxSize {
		if err := l.rotateLocked(); err != nil {
			if l.file == nil {
				l.fail(err)
				return
			}
			// Continua no arquivo atual; tenta de novo na próxima linha.
			if !l.rotateErrReported {
				l.rotateErrReported = true
				l.report(fmt.Errorf("rotação: %w", err))
			}
		} else {
			l.rotateErrReported = false
		}
	}

	n, err := l.file.WriteString(line)
	l.size += int64(n)
	if err != nil {
		l.fail(err)
	}
}

func (l *Logger) openLocked() error {
	if err := os.MkdirAll(l.opts.Dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.opts.FilePath(l.info.ID, 0), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.file, l.size = f, st.Size()
	if l.size == 0 {
		n, err := fmt.Fprintf(f, "# device %d %q %s porta %d — aberto %s\n",
			l.info.ID, l.info.Name, l.info.Type, l.info.Port,
			l.now().Format("2006-01-02 15:04:05"))
		l.size += int64(n)
		if err != nil {
			return err
		}
	}
	return nil
}

// rotateLocked apaga o mais velho, desloca .n → .n+1 e abre um ativo novo.
func (l *Logger) rotateLocked() error {
	l.closeLocked()
	id, ultimo := l.info.ID, l.opts.MaxFiles-1

	if err := os.Remove(l.opts.FilePath(id, ultimo)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return l.reopenAfter(err)
	}
	for n := ultimo - 1; n >= 0; n-- {
		err := os.Rename(l.opts.FilePath(id, n), l.opts.FilePath(id, n+1))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return l.reopenAfter(err)
		}
	}
	return l.openLocked()
}

// reopenAfter reabre o ativo depois de uma rotação que falhou, para continuar
// gravando nele.
func (l *Logger) reopenAfter(cause error) error {
	if err := l.openLocked(); err != nil {
		return err
	}
	return cause
}

func (l *Logger) closeLocked() {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	l.size = 0
}

// fail desiste até o próximo SetEnabled(true): uma falha de disco não pode
// virar uma linha de erro no trace a cada requisição.
func (l *Logger) fail(err error) {
	l.failed = true
	l.closeLocked()
	l.report(err)
}

func (l *Logger) report(err error) {
	if l.onError != nil {
		l.onError(fmt.Errorf("log do dispositivo %d: %w", l.info.ID, err))
	}
}

// WriteZip escreve um zip com todos os arquivos do dispositivo. Segura o
// lock para não pegar uma rotação no meio.
func (l *Logger) WriteZip(w io.Writer) error {
	if l == nil {
		return ErrNoLogs
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Sync()
	}
	return writeZip(w, l.opts.Files(l.info.ID))
}

func writeZip(w io.Writer, files []string) error {
	if len(files) == 0 {
		return ErrNoLogs
	}
	zw := zip.NewWriter(w)
	for _, p := range files {
		if err := addToZip(zw, p); err != nil {
			return err
		}
	}
	return zw.Close()
}

func addToZip(zw *zip.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := zip.FileInfoHeader(st)
	if err != nil {
		return err
	}
	hdr.Name = filepath.Base(path)
	hdr.Method = zip.Deflate
	dst, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, f)
	return err
}
