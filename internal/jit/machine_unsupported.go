//go:build !amd64 && !arm64

package jit

import "fmt"

func emitMachine(p *Program) ([]byte, error) {
	return nil, fmt.Errorf("native execution is unavailable on %s", platform())
}

func platform() string { return "unsupported architecture" }
