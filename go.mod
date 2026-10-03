module lunex

go 1.25.5

require (
	github.com/ebitengine/purego v0.11.1
	github.com/go-webgpu/goffi v0.6.4
	github.com/golang-jwt/jwt/v5 v5.2.0
	golang.org/x/crypto v0.31.0
	golang.org/x/sys v0.42.0
	modernc.org/sqlite v1.52.0
)

require (
	github.com/dustin/go-humanize v1.0.1
	github.com/google/uuid v1.6.0
	github.com/mattn/go-isatty v0.0.20
	github.com/ncruces/go-strftime v1.0.0
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec
	modernc.org/libc v1.72.3
	modernc.org/mathutil v1.7.1
	modernc.org/memory v1.11.0
)

replace github.com/ebitengine/purego => github.com/unxed/pureffi v0.1.16
