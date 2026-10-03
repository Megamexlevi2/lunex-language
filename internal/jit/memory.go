package jit

import "fmt"

var allocateExecutable = func(size int) ([]byte, error) {
	return nil, fmt.Errorf("executable memory is unavailable on this platform")
}

var protectExecutable = func(code []byte) error {
	return fmt.Errorf("executable memory is unavailable on this platform")
}

var releaseExecutable = func(code []byte) error {
	return fmt.Errorf("executable memory is unavailable on this platform")
}
