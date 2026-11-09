package debug

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	lunexruntime "lunex/internal/runtime"
)

func TestStartLiveDiagnosticsServesLocalSnapshots(t *testing.T) {
	profiler := lunexruntime.NewProfiler()
	profile := profiler.GetOrCreate("sample")
	profile.Record(time.Microsecond.Nanoseconds())
	server, err := StartLiveDiagnostics("127.0.0.1:0", profiler)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	for _, endpoint := range []string{"/debug/lunex/functions", "/debug/lunex/runtime", "/debug/pprof/"} {
		response, requestErr := http.Get("http://" + server.Address() + endpoint)
		if requestErr != nil {
			t.Fatalf("GET %s: %v", endpoint, requestErr)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatalf("read %s: %v", endpoint, readErr)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s returned %d: %s", endpoint, response.StatusCode, body)
		}
		if len(body) == 0 {
			t.Fatalf("GET %s returned an empty response", endpoint)
		}
	}

	response, err := http.Get("http://" + server.Address() + "/debug/lunex/functions")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var profiles []lunexruntime.FunctionProfileSnapshot
	if err := json.NewDecoder(response.Body).Decode(&profiles); err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Name != "sample" {
		t.Fatalf("unexpected function profile response: %#v", profiles)
	}
}

func TestLiveDiagnosticsRejectsNonLoopbackAddress(t *testing.T) {
	if _, err := StartLiveDiagnostics("0.0.0.0:6060", lunexruntime.NewProfiler()); err == nil {
		t.Fatal("expected non-loopback bind address to be rejected")
	}
}

func TestWriteReportProducesMachineReadableJSON(t *testing.T) {
	path := t.TempDir() + "/report.json"
	report := RunReport{
		SchemaVersion: 1,
		Execution:     ExecutionSnapshot{Status: "ok", ExitCode: 0},
	}
	if err := WriteReport(path, report); err != nil {
		t.Fatal(err)
	}
	response, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RunReport
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != 1 || decoded.Execution.Status != "ok" {
		t.Fatalf("unexpected report data: %#v", decoded)
	}
}

func TestWriteRuntimeProfileCreatesPprofFile(t *testing.T) {
	path := t.TempDir() + "/goroutines.pprof"
	if err := WriteRuntimeProfile("goroutine", path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("expected a non-empty goroutine profile")
	}
}

func TestStartCaptureCleansUpCPUProfileWhenTraceSetupFails(t *testing.T) {
	directory := t.TempDir()
	cpuPath := directory + "/cpu.pprof"
	tracePath := directory + "/missing/trace.out"

	if capture, err := StartCapture(CaptureOptions{CPUProfilePath: cpuPath, TracePath: tracePath}); err == nil {
		if capture != nil {
			_ = capture.Close()
		}
		t.Fatal("expected trace creation to fail")
	}

	retryPath := directory + "/retry.pprof"
	capture, err := StartCapture(CaptureOptions{CPUProfilePath: retryPath})
	if err != nil {
		t.Fatalf("CPU profiler remained active after cleanup: %v", err)
	}
	if err := capture.Close(); err != nil {
		t.Fatalf("close CPU profile: %v", err)
	}
	info, err := os.Stat(retryPath)
	if err != nil {
		t.Fatalf("stat retry profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected a non-empty CPU profile")
	}
}
