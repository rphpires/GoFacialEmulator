//go:build !windows

package reachability

import "syscall"

func limiteArquivosAbertos() uint64 {
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		return 0
	}
	return uint64(lim.Cur)
}
