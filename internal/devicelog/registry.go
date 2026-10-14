package devicelog

import (
	"io"
	"sync"
)

// Registry guarda um Logger por dispositivo. Vive no Manager e sobrevive a
// stop/start do emulador: a flag é do dispositivo, não da instância.
type Registry struct {
	opts    Options
	onError func(error)

	mu      sync.Mutex
	loggers map[int]*Logger
}

func NewRegistry(opts Options, onError func(error)) *Registry {
	return &Registry{opts: opts, onError: onError, loggers: map[int]*Logger{}}
}

// For devolve o logger do dispositivo, criando se preciso, com info
// atualizada.
func (r *Registry) For(info Info) *Logger {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.loggers[info.ID]; ok {
		l.SetInfo(info)
		return l
	}
	l := New(r.opts, info, r.onError)
	r.loggers[info.ID] = l
	return l
}

// SetEnabled liga ou desliga o log do dispositivo (desligar apaga).
func (r *Registry) SetEnabled(info Info, on bool) error {
	if r == nil {
		return nil
	}
	return r.For(info).SetEnabled(on)
}

// Remove desliga, apaga os arquivos e esquece o dispositivo.
func (r *Registry) Remove(id int) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	l := r.loggers[id]
	delete(r.loggers, id)
	r.mu.Unlock()
	if l != nil {
		return l.SetEnabled(false)
	}
	return r.opts.removeAll(id)
}

// ActivePath é o arquivo mais recente do dispositivo.
func (r *Registry) ActivePath(id int) string {
	return r.opts.FilePath(id, 0)
}

// WriteZip escreve todos os arquivos do dispositivo num zip.
func (r *Registry) WriteZip(id int, w io.Writer) error {
	if r == nil {
		return ErrNoLogs
	}
	r.mu.Lock()
	l := r.loggers[id]
	r.mu.Unlock()
	if l != nil {
		return l.WriteZip(w)
	}
	return writeZip(w, r.opts.Files(id))
}
