package debug

import (
	"fmt"
	"lunex/internal/meta"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

var enabled atomic.Bool
var verbose atomic.Bool

func init() {
	enabled.Store(envEnabled("LUNEX_DEBUG") || envEnabled("NTL_DEBUG") || envEnabled("DEBUG"))
	verbose.Store(envEnabled("LUNEX_VERBOSE"))
}

func envEnabled(name string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "on", "debug":
		return true
	default:
		return false
	}
}

func Enabled() bool {
	return enabled.Load()
}

func Verbose() bool {
	return verbose.Load()
}

func Enable() {
	enabled.Store(true)
	_ = os.Setenv("LUNEX_DEBUG", "1")
}

func EnableVerbose() {
	verbose.Store(true)
	Enable()
	_ = os.Setenv("LUNEX_VERBOSE", "1")
}

var (
	cReset  string
	cBold   string
	cDim    string
	cCyan   string
	cGreen  string
	cYellow string
)

func SetColorEnabled(enabled bool) {
	if !enabled {
		cReset, cBold, cDim, cCyan, cGreen, cYellow = "", "", "", "", "", ""
		return
	}
	cReset = "\x1b[0m"
	cBold = "\x1b[1m"
	cDim = "\x1b[90m"
	cCyan = "\x1b[36m"
	cGreen = "\x1b[32m"
	cYellow = "\x1b[33m"
}

func init() {
	SetColorEnabled(os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb")
}

func stamp() string {
	return time.Now().Format("15:04:05.000")
}

func Log(format string, args ...any) {
	if !Enabled() {
		return
	}
	fmt.Fprintf(os.Stderr, "%s  [debug %s] %s%s\n", cDim, stamp(), cReset, fmt.Sprintf(format, args...))
}

func Section(title string) {
	if !Enabled() {
		return
	}
	fmt.Fprintf(os.Stderr, "\n%s== %s ==%s\n", cBold+cCyan, title, cReset)
}

func Header(file string) {
	if !Enabled() {
		return
	}
	version := meta.FullVersion()
	executable := "(unknown)"
	if path, err := os.Executable(); err == nil && path != "" {
		executable = filepath.Base(path)
	}
	fmt.Fprintf(os.Stderr, "\n%sLunex diagnostic session%s\n", cBold, cReset)
	fmt.Fprintf(os.Stderr, "  version       %s\n", version)
	fmt.Fprintf(os.Stderr, "  target        %s\n", file)
	fmt.Fprintf(os.Stderr, "  executable    %s\n", executable)
	fmt.Fprintf(os.Stderr, "  platform      %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(os.Stderr, "  Go runtime    %s\n", runtime.Version())
	fmt.Fprintf(os.Stderr, "  process       %d\n", os.Getpid())
}

func Footer(total time.Duration) {
	if !Enabled() {
		return
	}
	fmt.Fprintf(os.Stderr, "\n%sTotal elapsed: %s%s\n", cDim, total.Round(time.Microsecond), cReset)
	PrintResources(SnapshotResources())
}

func Step(label, detail string) {
	if !Enabled() {
		return
	}
	if detail == "" {
		fmt.Fprintf(os.Stderr, "  %s•%s %s\n", cCyan, cReset, label)
		return
	}
	fmt.Fprintf(os.Stderr, "  %s•%s %-24s %s\n", cCyan, cReset, label, detail)
}

func StepOK(tag, label, detail string) {
	if !Enabled() {
		return
	}
	if detail == "" {
		fmt.Fprintf(os.Stderr, "  %s%-5s%s %s\n", cGreen, tag, cReset, label)
		return
	}
	fmt.Fprintf(os.Stderr, "  %s%-5s%s %-24s %s\n", cGreen, tag, cReset, label, detail)
}

func StepWarn(label, detail string) {
	if !Enabled() {
		return
	}
	fmt.Fprintf(os.Stderr, "  %sWARN%s %-24s %s\n", cYellow, cReset, label, detail)
}

func BytecodeSection(totalBytes, ntzBytes int, hasNTZ bool) {
	if !Enabled() {
		return
	}
	fmt.Fprintf(os.Stderr, "  object        %d bytes\n", totalBytes)
	if hasNTZ && ntzBytes > 0 {
		fmt.Fprintf(os.Stderr, "  native data   %d bytes\n", ntzBytes)
	} else {
		fmt.Fprintln(os.Stderr, "  native data   unavailable")
	}
}

func MemStats() {
	if Enabled() {
		PrintResources(SnapshotResources())
	}
}

func VSection(title string) {
	if Verbose() {
		fmt.Fprintf(os.Stderr, "\n%s%s%s\n", cCyan, title, cReset)
	}
}

func VStep(label string, args ...any) {
	if !Verbose() {
		return
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "  > %s\n", label)
		return
	}
	parts := make([]string, 0, len(args))
	for _, value := range args {
		parts = append(parts, fmt.Sprint(value))
	}
	fmt.Fprintf(os.Stderr, "  > %s  %s\n", label, strings.Join(parts, "  "))
}

func VKV(key string, value any) {
	if Verbose() {
		fmt.Fprintf(os.Stderr, "    %-24s %v\n", key, value)
	}
}

func VHeader(file string) {
	if Verbose() {
		fmt.Fprintf(os.Stderr, "\n%sLunex verbose diagnostics%s\n  file: %s\n  pid:  %d\n\n", cBold, cReset, file, os.Getpid())
	}
}

func VFooter(total time.Duration) {
	if Verbose() {
		fmt.Fprintf(os.Stderr, "\n%sVerbose session completed in %s%s\n", cDim, total.Round(time.Microsecond), cReset)
	}
}
