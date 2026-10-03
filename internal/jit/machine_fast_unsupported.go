//go:build !amd64 && !arm64

package jit

import "fmt"

func emitMachineFast(p *Program) ([]byte, error) {
	return nil, fmt.Errorf("fast native register backend is unavailable on this architecture")
}
