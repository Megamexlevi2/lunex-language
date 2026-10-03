//go:build linux || darwin || freebsd || android

package adaptor

import (
	"os"

	"golang.org/x/sys/unix"
)

func ttyWidth() int {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return 0
	}
	defer tty.Close()

	size, err := unix.IoctlGetWinsize(int(tty.Fd()), unix.TIOCGWINSZ)
	if err != nil || size == nil || size.Col == 0 {
		return 0
	}
	return int(size.Col)
}
