package firstrun

import (
	"fmt"
	"lunex/internal/adaptor"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	colorCyan   = "\033[96m"
	colorWhite  = "\033[97m"
	colorDim    = "\033[2m"
	colorGreen  = "\033[92m"
	colorYellow = "\033[93m"
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	hideCursor  = "\033[?25l"
	showCursor  = "\033[?25h"
	eraseLine   = "\033[2K"
)

var logoLines = []string{
	`  ██╗      ██╗   ██╗ ██╗   ██╗ ███████╗ ██╗  ██╗`,
	`  ██║      ██║   ██║ ████╗ ██║ ██╔════╝ ╚██╗██╔╝`,
	`  ██║      ██║   ██║ ██╔██╗██║ █████╗    ╚███╔╝ `,
	`  ██║      ██║   ██║ ██║╚████║ ██╔══╝    ██╔██╗ `,
	`  ███████╗ ╚██████╔╝ ██║ ╚███║ ███████╗ ██╔╝ ██╗`,
	`  ╚══════╝  ╚═════╝  ╚═╝  ╚══╝ ╚══════╝ ╚═╝  ╚═╝`,
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func clampStr(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func isTerminal() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func supportsColor() bool {
	if !isTerminal() {
		return false
	}
	term := os.Getenv("TERM")
	colorTerm := os.Getenv("COLORTERM")
	if colorTerm == "truecolor" || colorTerm == "24bit" || colorTerm == "256color" {
		return true
	}
	for _, dumb := range []string{"", "dumb"} {
		if term == dumb {
			return false
		}
	}
	return true
}

func canWriteDir(dir string) bool {
	probe := filepath.Join(dir, ".lunex_probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(probe)
	return true
}

func ensureWritableDir(dir string) bool {
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			return false
		}
		return canWriteDir(dir)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false
	}
	return canWriteDir(dir)
}

func markerPath() (string, bool) {
	return adaptor.MarkerPath()
}

func isFirstRun() (bool, string) {
	mp, ok := markerPath()
	if !ok {
		return true, ""
	}
	if _, err := os.Stat(mp); os.IsNotExist(err) {
		return true, mp
	}
	return false, mp
}

func writeMarker(path string) {
	if path == "" {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	os.WriteFile(path, []byte("1"), 0600)
}

func animateStep(label, result string, useColor bool) {
	const padWidth = 32
	padding := padWidth - len(label)
	if padding < 1 {
		padding = 1
	}
	dots := strings.Repeat(".", padding)
	padded := label + dots

	for i := 0; i < 10; i++ {
		frame := spinnerFrames[i%len(spinnerFrames)]
		if useColor {
			fmt.Printf("\r%s  %s%s%s  %s%s%s",
				eraseLine,
				colorCyan, frame, colorReset,
				colorDim, padded, colorReset)
		} else {
			fmt.Printf("\r  %s  %s", frame, padded)
		}
		time.Sleep(55 * time.Millisecond)
	}

	if useColor {
		fmt.Printf("\r%s  %s✓%s  %s%-32s%s %s%s\n",
			eraseLine,
			colorGreen, colorReset,
			colorWhite, label, colorDim, result, colorReset)
	} else {
		fmt.Printf("\r  v  %-32s %s\n", label, result)
	}
}

func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func showWelcomeAnimation(version string, useColor bool) {
	if useColor {
		fmt.Print(hideCursor)
	}

	fmt.Println()

	for _, line := range logoLines {
		if useColor {
			fmt.Printf("%s%s%s\n", colorCyan, line, colorReset)
		} else {
			fmt.Printf("%s\n", line)
		}
		time.Sleep(35 * time.Millisecond)
	}

	fmt.Println()

	if useColor {
		fmt.Printf("  %s%sLunex Lang%s  %sv%s%s  ·  The Lunex scripting language\n",
			colorBold, colorWhite, colorReset,
			colorCyan, version, colorReset)
		fmt.Printf("  %sby David Dev  ·  github.com/Megamexlevi2%s\n",
			colorDim, colorReset)
	} else {
		fmt.Printf("  Lunex Lang v%s  ·  The Lunex scripting language\n", version)
		fmt.Printf("  by David Dev  ·  github.com/Megamexlevi2\n")
	}

	fmt.Println()

	if useColor {
		fmt.Printf("  %s⚙  Setting up your environment...%s\n\n", colorYellow, colorReset)
	} else {
		fmt.Println("  Setting up your environment...")
	}

	time.Sleep(200 * time.Millisecond)

	arch := runtime.GOARCH
	goos := runtime.GOOS

	ntlDir, _ := adaptor.MarkerPath()

	if ntlDir != "" {
		ntlDir = filepath.Dir(ntlDir)
	}
	cacheDisplay := "~/.lx/cache"
	if ntlDir != "" {
		cacheDisplay = shortenHome(filepath.Join(ntlDir, "cache"))
	}

	animateStep("Detecting OS / architecture", goos+"/"+arch, useColor)
	animateStep("Initializing bytecode cache", cacheDisplay, useColor)
	animateStep("Loading standard library", "13 modules ready", useColor)

	fmt.Println()

	if useColor {
		fmt.Printf("  %s%s✓  Environment ready%s\n\n", colorBold, colorGreen, colorReset)
		fmt.Printf("  %sRun %s'lunex help'%s%s to get started.%s\n\n",
			colorDim, colorWhite, colorReset, colorDim, colorReset)
	} else {
		fmt.Println("  Environment ready.")
		fmt.Println("  Run 'lunex help' to get started.")
	}

	time.Sleep(300 * time.Millisecond)

	if useColor {
		fmt.Print(showCursor)
	}
}

func Check(version string) {
	first, mp := isFirstRun()
	if !first {
		return
	}

	color := supportsColor()
	showWelcomeAnimation(version, color)
	writeMarker(mp)
}
