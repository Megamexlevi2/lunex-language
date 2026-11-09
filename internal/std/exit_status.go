package std

import "sync/atomic"

var requestedExitCode atomic.Int32

func RequestExitCode(code int) {
	if code == 0 {
		return
	}
	if code < 0 {
		code = 1
	}
	if code > 255 {
		code = 255
	}
	requestedExitCode.CompareAndSwap(0, int32(code))
}

func ResetExitCode() {
	requestedExitCode.Store(0)
}

func ConsumeExitCode() int {
	return int(requestedExitCode.Swap(0))
}
