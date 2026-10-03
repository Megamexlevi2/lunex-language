package adaptor

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

type Platform uint8

const (
	PlatformLinux Platform = iota
	PlatformMacOS
	PlatformWindows
	PlatformFreeBSD
	PlatformAndroid
	PlatformTermux
	PlatformWASM
	PlatformUnknown
)

var Current = detectPlatform()

func detectPlatform() Platform {
	switch runtime.GOOS {
	case "linux":

		if os.Getenv("PREFIX") != "" {
			return PlatformTermux
		}

		if os.Getenv("ANDROID_ROOT") != "" || os.Getenv("ANDROID_DATA") != "" {
			return PlatformAndroid
		}
		return PlatformLinux
	case "android":
		if os.Getenv("PREFIX") != "" {
			return PlatformTermux
		}
		return PlatformAndroid
	case "darwin":
		return PlatformMacOS
	case "windows":
		return PlatformWindows
	case "freebsd":
		return PlatformFreeBSD
	case "js":
		return PlatformWASM
	default:
		return PlatformUnknown
	}
}

func (p Platform) String() string {
	switch p {
	case PlatformLinux:
		return "linux"
	case PlatformMacOS:
		return "macos"
	case PlatformWindows:
		return "windows"
	case PlatformFreeBSD:
		return "freebsd"
	case PlatformAndroid:
		return "android"
	case PlatformTermux:
		return "android/termux"
	case PlatformWASM:
		return "wasm"
	default:
		return "unknown"
	}
}

func IsAndroidLike() bool {
	return Current == PlatformAndroid || Current == PlatformTermux
}

func IsUnix() bool {
	switch Current {
	case PlatformLinux, PlatformMacOS, PlatformFreeBSD,
		PlatformAndroid, PlatformTermux:
		return true
	}
	return false
}

const cwdCacheDir = "lunex-cache"

func DataDirCandidates() []string {
	if override := os.Getenv("LUNEX_DATA_DIR"); override != "" {
		return []string{override}
	}

	var candidates []string

	if prefix := os.Getenv("PREFIX"); prefix != "" {
		candidates = append(candidates,
			filepath.Join(prefix, "var", "cache", "lunex"),
		)
	}

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".local", "share", "lunex"),
			filepath.Join(home, ".lx"),
		)
	}

	if cacheDir, err := os.UserCacheDir(); err == nil {
		candidates = append(candidates, filepath.Join(cacheDir, "lunex"))
	}

	if Current == PlatformWindows {
		if appData := os.Getenv("APPDATA"); appData != "" {
			candidates = append(candidates, filepath.Join(appData, "lunex"))
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates, filepath.Join(localAppData, "lunex"))
		}
	}

	if Current == PlatformMacOS {
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates,
				filepath.Join(home, "Library", "Application Support", "lunex"),
			)
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		cwdCache := filepath.Join(cwd, cwdCacheDir)
		if os.Getenv("LUNEX_USE_CWD_CACHE") == "1" {

			candidates = append([]string{cwdCache}, candidates...)
		} else {

			candidates = append(candidates, cwdCache)
		}
	}

	return candidates
}

func CWDCacheDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(cwd, cwdCacheDir)
}

func UseCWDCache() bool {
	return os.Getenv("LUNEX_USE_CWD_CACHE") == "1"
}

func CacheDir() string {
	return subDir("cache")
}

func NativeCacheDir() string {
	return subDir(filepath.Join("ncache", runtime.GOOS+"_"+runtime.GOARCH))
}

func ModuleDir(name, version string) string {
	if version == "" {
		version = "main"
	}
	safe := strings.ReplaceAll(name, "/", "__")
	return subDir(filepath.Join("modules", safe+"@"+version))
}

func RuntimeDir(hash string) string {
	return subDirExec("rt-" + hash)
}

func MarkerPath() (string, bool) {
	base := subDir("")
	if base == "" {
		return "", false
	}
	return filepath.Join(base, ".initialized"), true
}

func subDir(sub string) string {
	for _, base := range DataDirCandidates() {
		dir := base
		if sub != "" {
			dir = filepath.Join(base, sub)
		}
		if EnsureWritable(dir) {
			return dir
		}
	}

	return ""
}

func subDirExec(sub string) string {
	for _, base := range DataDirCandidates() {
		dir := filepath.Join(base, sub)
		if EnsureWritable(dir) && FSSupportsExec(dir) {
			return dir
		}
	}

	return ""
}

func RuntimeCandidateDirs(hash string) []string {
	sub := filepath.Join("lunex", "rt-"+hash)
	var dirs []string

	if override := os.Getenv("LUNEX_RT_DIR"); override != "" {
		dirs = append(dirs, filepath.Join(override, "rt-"+hash))
	}

	if prefix := os.Getenv("PREFIX"); prefix != "" {
		dirs = append(dirs,
			filepath.Join(prefix, "var", "cache", sub),
		)
	}

	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".local", "share", sub),
			filepath.Join(home, ".lx", "rt-"+hash),
		)
	}

	if cacheDir, err := os.UserCacheDir(); err == nil {
		dirs = append(dirs, filepath.Join(cacheDir, sub))
	}

	if Current == PlatformWindows {
		if appData := os.Getenv("LOCALAPPDATA"); appData != "" {
			dirs = append(dirs, filepath.Join(appData, sub))
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(cwd, cwdCacheDir, "rt-"+hash))
	}

	return dirs
}

func EnsureWritable(dir string) bool {
	if dir == "" {
		return false
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false
	}
	probe := filepath.Join(dir, ".lunex_probe_"+fmt.Sprintf("%d", os.Getpid()))
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(probe)
	return true
}

func FSSupportsExec(dir string) bool {
	if Current == PlatformWindows {
		return true
	}
	if Current == PlatformWASM {
		return false
	}
	if dir == "" {
		return false
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return false
	}

	probe := filepath.Join(dir, ".lunex_exec_probe")

	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(probe, []byte(script), 0755); err != nil {
		return false
	}
	defer os.Remove(probe)

	if err := os.Chmod(probe, 0755); err != nil {
		return false
	}

	cmd := exec.Command(probe)
	err := cmd.Run()
	if err == nil {
		return true
	}
	if _, ok := err.(*exec.ExitError); ok {
		return true
	}
	return false
}

var memCache sync.Map

func MemCacheKey(absPath string) (string, error) {
	info, err := os.Stat(absPath)
	if err != nil {

		h := sha256.Sum256([]byte(absPath))
		return hex.EncodeToString(h[:16]), nil
	}
	h := sha256.New()
	h.Write([]byte(absPath))
	fmt.Fprintf(h, "%d", info.ModTime().UnixNano())
	fmt.Fprintf(h, "%d", info.Size())
	return hex.EncodeToString(h.Sum(nil)[:16]), nil
}

func MemCacheGet(key string) ([]byte, bool) {
	v, ok := memCache.Load(key)
	if !ok {
		return nil, false
	}
	return v.([]byte), true
}

func MemCacheSet(key string, data []byte) {
	if len(data) == 0 {
		return
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	memCache.Store(key, cp)
}

func MemCacheDelete(key string) {
	memCache.Delete(key)
}

func MemCacheStats() (count int, totalBytes int64) {
	memCache.Range(func(_, v any) bool {
		count++
		totalBytes += int64(len(v.([]byte)))
		return true
	})
	return
}

func MemCacheClear() {
	memCache.Range(func(k, _ any) bool {
		memCache.Delete(k)
		return true
	})
}

func CacheLookup(absPath string) ([]byte, bool) {
	key, err := MemCacheKey(absPath)
	if err != nil {
		return nil, false
	}

	if preferMemCache {
		if data, ok := MemCacheGet(key); ok {
			return data, true
		}
		if dir := CacheDir(); dir != "" {
			diskPath := filepath.Join(dir, key+".nax")
			if data, err := os.ReadFile(diskPath); err == nil && len(data) > 0 {
				MemCacheSet(key, data)
				return data, true
			}
		}
		return nil, false
	}

	if dir := CacheDir(); dir != "" {
		diskPath := filepath.Join(dir, key+".nax")
		if data, err := os.ReadFile(diskPath); err == nil && len(data) > 0 {
			return data, true
		}
	}

	return MemCacheGet(key)
}

func CacheStore(absPath string, objectData []byte) {
	key, err := MemCacheKey(absPath)
	if err != nil || len(objectData) == 0 {
		return
	}

	MemCacheSet(key, objectData)

	dir := CacheDir()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	diskPath := filepath.Join(dir, key+".nax")
	_ = os.WriteFile(diskPath, objectData, 0600)
}

func CacheInvalidate(absPath string) {
	key, err := MemCacheKey(absPath)
	if err != nil {
		return
	}
	MemCacheDelete(key)
	if dir := CacheDir(); dir != "" {
		_ = os.Remove(filepath.Join(dir, key+".nax"))
	}
}

func CanExecBinary(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if Current != PlatformWindows && info.Mode()&0111 == 0 {
		return false
	}
	cmd := exec.Command(path, "--probe")
	if err := cmd.Run(); err == nil {
		return true
	} else if _, ok := err.(*exec.ExitError); ok {
		return true
	}
	return false
}

func SetExecBit(path string) error {
	if Current == PlatformWindows {
		return nil
	}
	return os.Chmod(path, 0755)
}

func BinaryName() string {
	if Current == PlatformWindows {
		return "lunex-rt.exe"
	}
	return "lunex-rt"
}

func ShortenHome(path string) string {
	if path == "" {
		return "(memory only)"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func CleanStale(currentDir, currentHash string) {
	if currentDir == "" {
		return
	}
	parent := filepath.Dir(currentDir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "rt-") {
			continue
		}
		if name == "rt-"+currentHash {
			continue
		}
		_ = os.RemoveAll(filepath.Join(parent, name))
	}
}

func userToken() string {
	if u := os.Getenv("USER"); u != "" {
		return sanitize(u, 16)
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return sanitize(u, 16)
	}
	return fmt.Sprintf("pid%d", os.Getpid())
}

func sanitize(s string, maxLen int) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
		if b.Len() >= maxLen {
			break
		}
	}
	if b.Len() == 0 {
		return "user"
	}
	return b.String()
}

func Info() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "platform       : %s (%s/%s)\n", Current, runtime.GOOS, runtime.GOARCH)

	cwdDir := CWDCacheDir()
	if cwdDir != "" {
		active := ""
		if UseCWDCache() {
			active = " (active — LUNEX_USE_CWD_CACHE=1)"
		}
		fmt.Fprintf(&sb, "cwd cache      : %s%s\n", cwdDir, active)
	}

	cacheDir := CacheDir()
	if cacheDir == "" {
		fmt.Fprintf(&sb, "cache dir      : (unavailable — memory only)\n")
	} else {
		fmt.Fprintf(&sb, "cache dir      : %s\n", ShortenHome(cacheDir))
	}

	nativeDir := NativeCacheDir()
	if nativeDir == "" {
		fmt.Fprintf(&sb, "native cache   : (unavailable — memory only)\n")
	} else {
		fmt.Fprintf(&sb, "native cache   : %s\n", ShortenHome(nativeDir))
	}

	count, total := MemCacheStats()
	fmt.Fprintf(&sb, "mem cache      : %d entries, %d bytes\n", count, total)

	if mp, ok := MarkerPath(); ok {
		fmt.Fprintf(&sb, "marker path    : %s\n", ShortenHome(mp))
	} else {
		fmt.Fprintf(&sb, "marker path    : (unavailable)\n")
	}

	return sb.String()
}

func availableMemoryBytes() uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}

func littleCoreCount() int {
	nCPU := runtime.NumCPU()
	if nCPU <= 2 {
		return 0
	}

	freqs := make([]int, 0, nCPU)
	for i := 0; i < nCPU; i++ {
		path := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cpufreq/cpuinfo_max_freq", i)
		data, err := os.ReadFile(path)
		if err != nil {
			return 0
		}
		freq, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return 0
		}
		freqs = append(freqs, freq)
	}

	minFreq, maxFreq := freqs[0], freqs[0]
	for _, f := range freqs {
		if f < minFreq {
			minFreq = f
		}
		if f > maxFreq {
			maxFreq = f
		}
	}

	if maxFreq == minFreq {
		return 0
	}

	mid := (minFreq + maxFreq) / 2
	little := 0
	for _, f := range freqs {
		if f < mid {
			little++
		}
	}
	return little
}

func tunedGOMAXPROCS() int {
	total := runtime.NumCPU()
	little := littleCoreCount()

	big := total - little
	if big < 2 {
		big = total
	}
	if big < 1 {
		big = 1
	}
	return big
}

func applyAndroidTuning() {
	if !IsAndroidLike() {
		return
	}

	if os.Getenv("GOMAXPROCS") == "" {
		if procs := tunedGOMAXPROCS(); procs > 0 {
			runtime.GOMAXPROCS(procs)
		}
	}

	if os.Getenv("GOGC") == "" {
		mem := availableMemoryBytes()
		switch {
		case mem > 0 && mem < 3<<30:
			debug.SetGCPercent(30)
		case mem > 0 && mem < 6<<30:
			debug.SetGCPercent(40)
		default:
			debug.SetGCPercent(50)
		}
	}

	if os.Getenv("GOMEMLIMIT") == "" {
		mem := availableMemoryBytes()
		if mem > 0 {

			limit := mem / 4
			const floor = 64 << 20
			const ceiling = 256 << 20
			if limit < floor {
				limit = floor
			}
			if limit > ceiling {
				limit = ceiling
			}
			debug.SetMemoryLimit(int64(limit))
		}
	}

	preferMemCache = true

	androidTuningApplied = true
}

var androidTuningApplied = false

func AndroidTuningApplied() bool {
	return androidTuningApplied
}

var preferMemCache = false

func init() {
	applyAndroidTuning()
}

const defaultTerminalWidth = 80

const minTerminalWidth = 32

func TerminalWidth() (width int) {

	defer func() {
		if recover() != nil {
			width = defaultTerminalWidth
		}
	}()

	if cols := os.Getenv("COLUMNS"); cols != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(cols)); err == nil && n > 0 {
			return clampWidth(n)
		}
	}

	if w := ttyWidth(); w > 0 {
		return clampWidth(w)
	}

	return defaultTerminalWidth
}

func clampWidth(n int) int {
	if n < minTerminalWidth {
		return minTerminalWidth
	}
	return n
}
