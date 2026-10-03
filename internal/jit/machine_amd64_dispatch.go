//go:build amd64

package jit

func emitMachine(p *Program) ([]byte, error) {
	if p.Tier >= 2 {
		return emitMachineTier2(p)
	}
	return emitMachineLegacy(p)
}
