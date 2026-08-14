//go:build windows

package reachability

// O Windows não tem RLIMIT_NOFILE. Zero significa "não se aplica".
func limiteArquivosAbertos() uint64 { return 0 }
