package std

import "sync/atomic"

var ffiEnabled atomic.Bool

func SetFFIEnabled(enabled bool) {
	ffiEnabled.Store(enabled)
}

func IsFFIEnabled() bool {
	return ffiEnabled.Load()
}
