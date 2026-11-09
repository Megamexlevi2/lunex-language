package debug

import (
	"encoding/json"
	"errors"
	"fmt"
	"lunex/internal/errfmt"
	lunexruntime "lunex/internal/runtime"
	"net"
	"net/http"
	httppprof "net/http/pprof"
	"os"
	goruntime "runtime"
	"runtime/pprof"
	"runtime/trace"
	"sort"
	"strings"
	"time"
)

type ResourceSnapshot struct {
	Timestamp         time.Time `json:"timestamp"`
	Goroutines        int       `json:"goroutines"`
	GOMAXPROCS        int       `json:"gomaxprocs"`
	CPUCount          int       `json:"cpu_count"`
	HeapAllocBytes    uint64    `json:"heap_alloc_bytes"`
	HeapInuseBytes    uint64    `json:"heap_inuse_bytes"`
	HeapSysBytes      uint64    `json:"heap_sys_bytes"`
	HeapReleasedBytes uint64    `json:"heap_released_bytes"`
	HeapObjects       uint64    `json:"heap_objects"`
	StackInuseBytes   uint64    `json:"stack_inuse_bytes"`
	TotalAllocBytes   uint64    `json:"total_alloc_bytes"`
	Mallocs           uint64    `json:"mallocs"`
	Frees             uint64    `json:"frees"`
	NumGC             uint32    `json:"num_gc"`
	PauseTotalNs      uint64    `json:"pause_total_ns"`
	LastGCPauseNs     uint64    `json:"last_gc_pause_ns"`
	LastGCTime        time.Time `json:"last_gc_time,omitempty"`
	GCCPUFraction     float64   `json:"gc_cpu_fraction"`
}

type ToolSnapshot struct {
	Version    string `json:"version"`
	Go         string `json:"go"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	PID        int    `json:"pid"`
	GOMAXPROCS int    `json:"gomaxprocs"`
}

type TargetSnapshot struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
	Lines     int    `json:"lines"`
	SHA256    string `json:"sha256"`
}

type ExecutionSnapshot struct {
	Status          string `json:"status"`
	FailedPhase     string `json:"failed_phase,omitempty"`
	ExitCode        int    `json:"exit_code"`
	CompileTimeNs   int64  `json:"compile_time_ns"`
	ExecutionTimeNs int64  `json:"execution_time_ns"`
	TotalTimeNs     int64  `json:"total_time_ns"`
}

type RuntimeErrorSnapshot struct {
	Kind    string `json:"kind,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}

type RunReport struct {
	SchemaVersion int                                    `json:"schema_version"`
	StartedAt     time.Time                              `json:"started_at"`
	Tool          ToolSnapshot                           `json:"tool"`
	Target        TargetSnapshot                         `json:"target"`
	Execution     ExecutionSnapshot                      `json:"execution"`
	Resources     ResourceSnapshot                       `json:"resources"`
	Functions     []lunexruntime.FunctionProfileSnapshot `json:"functions,omitempty"`
	Error         *RuntimeErrorSnapshot                  `json:"error,omitempty"`
	Diagnostics   []RuntimeErrorSnapshot                 `json:"diagnostics,omitempty"`
	LunexStack    string                                 `json:"lunex_stack,omitempty"`
	GoStack       string                                 `json:"go_stack,omitempty"`
}

type CaptureOptions struct {
	CPUProfilePath string
	TracePath      string
}

type Capture struct {
	cpuFile   *os.File
	traceFile *os.File
	closed    bool
}

type LiveServer struct {
	server   *http.Server
	listener net.Listener
}

func SnapshotResources() ResourceSnapshot {
	var stats goruntime.MemStats
	goruntime.ReadMemStats(&stats)
	result := ResourceSnapshot{
		Timestamp:         time.Now().UTC(),
		Goroutines:        goruntime.NumGoroutine(),
		GOMAXPROCS:        goruntime.GOMAXPROCS(0),
		CPUCount:          goruntime.NumCPU(),
		HeapAllocBytes:    stats.HeapAlloc,
		HeapInuseBytes:    stats.HeapInuse,
		HeapSysBytes:      stats.HeapSys,
		HeapReleasedBytes: stats.HeapReleased,
		HeapObjects:       stats.HeapObjects,
		StackInuseBytes:   stats.StackInuse,
		TotalAllocBytes:   stats.TotalAlloc,
		Mallocs:           stats.Mallocs,
		Frees:             stats.Frees,
		NumGC:             stats.NumGC,
		PauseTotalNs:      stats.PauseTotalNs,
		GCCPUFraction:     stats.GCCPUFraction,
	}
	if stats.NumGC > 0 {
		result.LastGCPauseNs = stats.PauseNs[(stats.NumGC+255)%256]
		result.LastGCTime = time.Unix(0, int64(stats.LastGC)).UTC()
	}
	return result
}

func PrintResources(stats ResourceSnapshot) {
	fmt.Fprintf(os.Stderr, "\n%sRuntime resources%s\n", cBold, cReset)
	fmt.Fprintf(os.Stderr, "  goroutines    %d\n", stats.Goroutines)
	fmt.Fprintf(os.Stderr, "  GOMAXPROCS    %d  (CPUs: %d)\n", stats.GOMAXPROCS, stats.CPUCount)
	fmt.Fprintf(os.Stderr, "  heap alloc    %s\n", formatBytes(stats.HeapAllocBytes))
	fmt.Fprintf(os.Stderr, "  heap in-use   %s\n", formatBytes(stats.HeapInuseBytes))
	fmt.Fprintf(os.Stderr, "  heap reserved %s\n", formatBytes(stats.HeapSysBytes))
	fmt.Fprintf(os.Stderr, "  stack memory  %s\n", formatBytes(stats.StackInuseBytes))
	fmt.Fprintf(os.Stderr, "  heap objects  %d\n", stats.HeapObjects)
	fmt.Fprintf(os.Stderr, "  GC cycles     %d  (total pause: %s)\n", stats.NumGC, time.Duration(stats.PauseTotalNs).Round(time.Microsecond))
	if stats.NumGC > 0 {
		fmt.Fprintf(os.Stderr, "  last GC       %s ago\n", time.Since(stats.LastGCTime).Round(time.Millisecond))
	}
}

func formatBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, exponent := uint64(unit), 0
	for next := divisor * unit; value >= next && exponent < 5; next = divisor * unit {
		divisor = next
		exponent++
	}
	units := [...]string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	return fmt.Sprintf("%.2f %s", float64(value)/float64(divisor), units[exponent])
}

func StartLiveDiagnostics(address string, profiler *lunexruntime.Profiler) (*LiveServer, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid pprof address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if host == "localhost" {
		_, port, _ := net.SplitHostPort(address)
		address = net.JoinHostPort("127.0.0.1", port)
	} else if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("pprof can only bind to localhost or a loopback IP address")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", httppprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", httppprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", httppprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", httppprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", httppprof.Trace)
	for _, name := range []string{"allocs", "block", "goroutine", "heap", "mutex", "threadcreate"} {
		mux.Handle("/debug/pprof/"+name, httppprof.Handler(name))
	}
	mux.HandleFunc("/debug/lunex/functions", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if profiler == nil {
			_, _ = writer.Write([]byte("[]\n"))
			return
		}
		if err := json.NewEncoder(writer).Encode(profiler.Snapshot()); err != nil {
			http.Error(writer, "cannot encode function profile", http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/debug/lunex/runtime", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(SnapshotResources()); err != nil {
			http.Error(writer, "cannot encode runtime snapshot", http.StatusInternalServerError)
		}
	})
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("start local pprof listener: %w", err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	live := &LiveServer{server: server, listener: listener}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			Log("live diagnostics server stopped: %v", serveErr)
		}
	}()
	return live, nil
}

func (live *LiveServer) Address() string {
	if live == nil || live.listener == nil {
		return ""
	}
	return live.listener.Addr().String()
}

func (live *LiveServer) Close() error {
	if live == nil || live.server == nil {
		return nil
	}
	return live.server.Close()
}

func StartCapture(options CaptureOptions) (*Capture, error) {
	capture := &Capture{}
	cleanup := func(err error) (*Capture, error) {
		_ = capture.Close()
		return nil, err
	}
	if options.CPUProfilePath != "" {
		file, err := os.OpenFile(options.CPUProfilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return nil, fmt.Errorf("create CPU profile: %w", err)
		}
		if err := file.Chmod(0600); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("secure CPU profile: %w", err)
		}
		if err := pprof.StartCPUProfile(file); err != nil {
			_ = file.Close()
			return cleanup(fmt.Errorf("start CPU profile: %w", err))
		}
		capture.cpuFile = file
	}
	if options.TracePath != "" {
		file, err := os.OpenFile(options.TracePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return cleanup(fmt.Errorf("create execution trace: %w", err))
		}
		if err := file.Chmod(0600); err != nil {
			_ = file.Close()
			return cleanup(fmt.Errorf("secure execution trace: %w", err))
		}
		if err := trace.Start(file); err != nil {
			_ = file.Close()
			return cleanup(fmt.Errorf("start execution trace: %w", err))
		}
		capture.traceFile = file
	}
	return capture, nil
}

func (capture *Capture) Close() error {
	if capture == nil || capture.closed {
		return nil
	}
	capture.closed = true
	var failures []error
	if capture.traceFile != nil {
		trace.Stop()
		if err := capture.traceFile.Close(); err != nil {
			failures = append(failures, err)
		}
	}
	if capture.cpuFile != nil {
		pprof.StopCPUProfile()
		if err := capture.cpuFile.Close(); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func WriteRuntimeProfile(name, path string) error {
	profile := pprof.Lookup(name)
	if profile == nil {
		return fmt.Errorf("runtime profile %q is unavailable", name)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	if err := profile.WriteTo(file, 0); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func WriteHeapProfile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	if err := pprof.WriteHeapProfile(file); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func WriteGoroutineDump(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	buffer := make([]byte, 1<<20)
	for {
		n := goruntime.Stack(buffer, true)
		if n < len(buffer) {
			_, err := file.Write(buffer[:n])
			return err
		}
		if len(buffer) >= 64<<20 {
			_, err := file.Write(buffer[:n])
			if err == nil {
				_, err = file.WriteString("\n[goroutine dump truncated at 64 MiB]\n")
			}
			return err
		}
		buffer = make([]byte, len(buffer)*2)
	}
}

func WriteReport(path string, report RunReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func PrintFunctionProfile(profiles []lunexruntime.FunctionProfileSnapshot, limit int) {
	if len(profiles) == 0 {
		fmt.Fprintln(os.Stderr, "\nFunction profile: no user-function samples were recorded")
		return
	}
	profiles = append([]lunexruntime.FunctionProfileSnapshot(nil), profiles...)
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].EstimatedTotalNs == profiles[j].EstimatedTotalNs {
			return profiles[i].Calls > profiles[j].Calls
		}
		return profiles[i].EstimatedTotalNs > profiles[j].EstimatedTotalNs
	})
	if limit > 0 && len(profiles) > limit {
		profiles = profiles[:limit]
	}
	fmt.Fprintf(os.Stderr, "\n%sFunction profile%s\n", cBold, cReset)
	fmt.Fprintf(os.Stderr, "  %-28s %10s %10s %12s %12s\n", "Function", "Calls", "Samples", "Total est.", "Average est.")
	fmt.Fprintf(os.Stderr, "  %s\n", strings.Repeat("-", 78))
	for _, profile := range profiles {
		fmt.Fprintf(os.Stderr, "  %-28s %10d %10d %12s %12s\n",
			truncate(profile.Name, 28), profile.Calls, profile.Samples,
			time.Duration(profile.EstimatedTotalNs).Round(time.Microsecond),
			time.Duration(profile.EstimatedAverageNs).Round(time.Microsecond))
	}
	fmt.Fprintln(os.Stderr, "  Estimated times are sampled and weighted; they are not exact per-call timings.")
}

func PanicReport(value any, stack []byte) {
	PanicReportWithLunexStack(value, stack, "")
}

func PanicReportWithLunexStack(value any, goStack []byte, lunexStack string) {
	fmt.Fprintf(os.Stderr, "\n%sUnhandled runtime panic%s: %v\n", cBold+cYellow, cReset, value)
	if lunexStack != "" {
		fmt.Fprintln(os.Stderr, "Lunex call stack:")
		fmt.Fprint(os.Stderr, lunexStack)
	}
	if len(goStack) > 0 {
		fmt.Fprintln(os.Stderr, "Go stack:")
		_, _ = os.Stderr.Write(goStack)
	}
}

func ErrorSnapshot(err error) (*RuntimeErrorSnapshot, string) {
	if err == nil {
		return nil, ""
	}
	snapshot := &RuntimeErrorSnapshot{Message: err.Error()}
	var lunexErr *errfmt.LunexError
	if errors.As(err, &lunexErr) {
		return snapshotFromLunexError(lunexErr)
	}
	var diagnostic interface{ DiagnosticError() *errfmt.LunexError }
	if errors.As(err, &diagnostic) {
		if diagnosticError := diagnostic.DiagnosticError(); diagnosticError != nil {
			return snapshotFromLunexError(diagnosticError)
		}
	}
	return snapshot, ""
}

func snapshotFromLunexError(lunexErr *errfmt.LunexError) (*RuntimeErrorSnapshot, string) {
	if lunexErr == nil {
		return nil, ""
	}
	return &RuntimeErrorSnapshot{
		Kind:    string(lunexErr.Kind),
		Code:    lunexErr.Code,
		Message: lunexErr.Message,
		File:    lunexErr.File,
		Line:    lunexErr.Line,
		Column:  lunexErr.Col,
	}, FormatStack(lunexErr.Stack)
}

func FormatStack(frames []errfmt.StackFrame) string {
	if len(frames) == 0 {
		return ""
	}
	var builder strings.Builder
	for index := len(frames) - 1; index >= 0; index-- {
		frame := frames[index]
		fmt.Fprintf(&builder, "#%d %s", len(frames)-1-index, frame.FnName)
		if frame.File != "" {
			fmt.Fprintf(&builder, " (%s", frame.File)
			if frame.Line > 0 {
				fmt.Fprintf(&builder, ":%d", frame.Line)
				if frame.Col > 0 {
					fmt.Fprintf(&builder, ":%d", frame.Col)
				}
			}
			builder.WriteByte(')')
		}
		builder.WriteByte('\n')
	}
	return builder.String()
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
