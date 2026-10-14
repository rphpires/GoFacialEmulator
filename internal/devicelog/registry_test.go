package devicelog

import (
	"bytes"
	"os"
	"testing"
)

func TestForDevolveOMesmoLoggerEAtualizaInfo(t *testing.T) {
	r := NewRegistry(optsDeTeste(t), nil)
	a := r.For(Info{ID: 4, Name: "velho"})
	b := r.For(Info{ID: 4, Name: "novo"})
	if a != b {
		t.Fatal("For criou dois loggers para o mesmo id")
	}
	if b.info.Name != "novo" {
		t.Errorf("info não atualizada: %+v", b.info)
	}
}

func TestRemoveApagaMesmoSemLogger(t *testing.T) {
	o := optsDeTeste(t)
	if err := os.MkdirAll(o.Dir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(o.FilePath(5, 0), []byte("x"), 0644)
	_ = os.WriteFile(o.FilePath(5, 1), []byte("x"), 0644)
	r := NewRegistry(o, nil)
	if err := r.Remove(5); err != nil {
		t.Fatal(err)
	}
	if fs := o.Files(5); len(fs) != 0 {
		t.Errorf("sobrou: %v", fs)
	}
}

func TestRemoveNaoTocaEmIDComMesmoPrefixo(t *testing.T) {
	o := optsDeTeste(t)
	r := NewRegistry(o, nil)
	_ = r.SetEnabled(Info{ID: 1}, true)
	_ = r.SetEnabled(Info{ID: 12}, true)
	_ = r.Remove(1)
	if fs := o.Files(12); len(fs) != 1 {
		t.Errorf("arquivos do 12: %v", fs)
	}
	var buf bytes.Buffer
	if err := r.WriteZip(1, &buf); err != ErrNoLogs {
		t.Errorf("zip do 1 depois de remover: %v", err)
	}
}

func TestRegistryWriteZipSemLoggerLeDoDisco(t *testing.T) {
	o := optsDeTeste(t)
	_ = os.MkdirAll(o.Dir, 0755)
	_ = os.WriteFile(o.FilePath(8, 0), []byte("linha\n"), 0644)
	r := NewRegistry(o, nil)
	var buf bytes.Buffer
	if err := r.WriteZip(8, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Error("zip vazio")
	}
}

func TestRegistryNilNaoEntraEmPanico(t *testing.T) {
	var r *Registry
	if r.For(Info{ID: 1}) != nil {
		t.Error("For em nil deveria devolver nil")
	}
	_ = r.SetEnabled(Info{ID: 1}, true)
	_ = r.Remove(1)
}
