//go:build !(linux || darwin || freebsd || android)

package adaptor

func ttyWidth() int {
	return 0
}
