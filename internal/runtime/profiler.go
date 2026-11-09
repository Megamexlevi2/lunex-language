package runtime

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type FnProfile struct {
	Name             string
	calls            atomic.Int64
	totalNs          atomic.Int64
	samples          atomic.Int64
	sampleTick       atomic.Uint64
	lastRecordedCall atomic.Int64
}

func NewFnProfile(name string) *FnProfile {
	return &FnProfile{Name: name}
}

func (p *FnProfile) CountCall() {
	p.calls.Add(1)
}

func (p *FnProfile) RecordSample(durationNs int64) {
	if durationNs < 0 {
		durationNs = 0
	}
	currentCalls := p.calls.Load()
	previousCalls := p.lastRecordedCall.Swap(currentCalls)
	weight := currentCalls - previousCalls
	if weight < 1 {
		weight = 1
	}
	if weight > 32 {
		weight = 32
	}
	weightedDuration := durationNs
	if durationNs > 0 && durationNs > int64(^uint64(0)>>1)/weight {
		weightedDuration = int64(^uint64(0) >> 1)
	} else {
		weightedDuration *= weight
	}
	p.samples.Add(1)
	for {
		current := p.totalNs.Load()
		next := current + weightedDuration
		if weightedDuration > 0 && next < current {
			next = int64(^uint64(0) >> 1)
		}
		if p.totalNs.CompareAndSwap(current, next) {
			break
		}
	}
}

func (p *FnProfile) Record(durationNs int64) {
	if durationNs < 0 {
		durationNs = 0
	}
	call := p.calls.Add(1)
	p.lastRecordedCall.Store(call)
	p.samples.Add(1)
	for {
		current := p.totalNs.Load()
		next := current + durationNs
		if durationNs > 0 && next < current {
			next = int64(^uint64(0) >> 1)
		}
		if p.totalNs.CompareAndSwap(current, next) {
			break
		}
	}
}

func (p *FnProfile) ShouldSample() bool {
	tick := p.sampleTick.Add(1)
	if tick <= 8 {
		return true
	}
	return tick&31 == 1
}

func (p *FnProfile) TotalCalls() int64 {
	return p.calls.Load()
}

func (p *FnProfile) SampleCount() int64 {
	return p.samples.Load()
}

func (p *FnProfile) TotalNs() int64 {
	return p.totalNs.Load()
}

func (p *FnProfile) AvgNs() float64 {
	calls := p.calls.Load()
	if calls == 0 {
		return 0
	}
	return float64(p.totalNs.Load()) / float64(calls)
}

type FunctionProfileSnapshot struct {
	Name               string `json:"name"`
	Calls              int64  `json:"calls"`
	Samples            int64  `json:"samples"`
	EstimatedTotalNs   int64  `json:"estimated_total_ns"`
	EstimatedAverageNs int64  `json:"estimated_average_ns"`
}

type Profiler struct {
	profiles sync.Map
	start    time.Time
}

func NewProfiler() *Profiler {
	return &Profiler{start: time.Now()}
}

func (p *Profiler) GetOrCreate(name string) *FnProfile {
	if value, ok := p.profiles.Load(name); ok {
		return value.(*FnProfile)
	}
	profile := NewFnProfile(name)
	actual, _ := p.profiles.LoadOrStore(name, profile)
	return actual.(*FnProfile)
}

func (p *Profiler) Report() string {
	var builder strings.Builder
	elapsed := time.Since(p.start)
	builder.WriteString(fmt.Sprintf("\n\x1b[1m  Lunex Profile\x1b[0m  \x1b[90m(runtime: %.2fs)\x1b[0m\n", elapsed.Seconds()))
	builder.WriteString(fmt.Sprintf("  %-28s %-8s %-12s\n", "Symbol", "Calls", "Avg µs"))
	builder.WriteString(strings.Repeat("\x1b[90m─\x1b[0m", 52) + "\n")
	profiles := p.snapshot()
	for _, profile := range profiles {
		calls := profile.TotalCalls()
		if calls == 0 {
			continue
		}
		builder.WriteString(fmt.Sprintf("  %-28s %-8d %-12.2f\n", trunc(profile.Name, 27), calls, profile.AvgNs()/1e3))
	}
	return builder.String()
}

func (p *Profiler) snapshot() []*FnProfile {
	profiles := make([]*FnProfile, 0)
	p.profiles.Range(func(_, value any) bool {
		profiles = append(profiles, value.(*FnProfile))
		return true
	})
	sort.Slice(profiles, func(i, j int) bool {
		left := profiles[i].TotalNs()
		right := profiles[j].TotalNs()
		if left == right {
			return profiles[i].Name < profiles[j].Name
		}
		return left > right
	})
	return profiles
}

func (p *Profiler) Snapshot() []FunctionProfileSnapshot {
	if p == nil {
		return nil
	}
	profiles := p.snapshot()
	result := make([]FunctionProfileSnapshot, 0, len(profiles))
	for _, profile := range profiles {
		calls := profile.TotalCalls()
		if calls == 0 {
			continue
		}
		average := int64(0)
		if calls > 0 {
			average = profile.TotalNs() / calls
		}
		result = append(result, FunctionProfileSnapshot{
			Name:               profile.Name,
			Calls:              calls,
			Samples:            profile.SampleCount(),
			EstimatedTotalNs:   profile.TotalNs(),
			EstimatedAverageNs: average,
		})
	}
	return result
}

func (p *Profiler) WritePprof(writer io.Writer) error {
	if writer == nil {
		return fmt.Errorf("pprof output writer is nil")
	}
	profiles := p.snapshot()
	stringsTable := []string{"", "calls", "count", "wall", "nanoseconds"}
	stringIDs := map[string]uint64{"": 0, "calls": 1, "count": 2, "wall": 3, "nanoseconds": 4}
	for _, profile := range profiles {
		if _, exists := stringIDs[profile.Name]; !exists {
			stringIDs[profile.Name] = uint64(len(stringsTable))
			stringsTable = append(stringsTable, profile.Name)
		}
	}
	var result []byte
	result = appendBytesField(result, 1, encodeValueType(stringIDs["calls"], stringIDs["count"]))
	result = appendBytesField(result, 1, encodeValueType(stringIDs["wall"], stringIDs["nanoseconds"]))
	functionID := uint64(1)
	for _, profile := range profiles {
		calls := profile.TotalCalls()
		if calls <= 0 {
			continue
		}
		nameID := stringIDs[profile.Name]
		function := appendVarintField(nil, 1, functionID)
		function = appendVarintField(function, 2, nameID)
		function = appendVarintField(function, 3, nameID)
		function = appendVarintField(function, 4, 0)
		result = appendBytesField(result, 5, function)
		line := appendVarintField(nil, 1, functionID)
		location := appendVarintField(nil, 1, functionID)
		location = appendBytesField(location, 4, line)
		result = appendBytesField(result, 4, location)
		var packedLocations []byte
		packedLocations = appendVarint(packedLocations, functionID)
		var packedValues []byte
		packedValues = appendVarint(packedValues, uint64(calls))
		elapsed := profile.TotalNs()
		if elapsed < 0 {
			elapsed = 0
		}
		packedValues = appendVarint(packedValues, uint64(elapsed))
		sample := appendBytesField(nil, 1, packedLocations)
		sample = appendBytesField(sample, 2, packedValues)
		result = appendBytesField(result, 2, sample)
		functionID++
	}
	for _, value := range stringsTable {
		result = appendBytesField(result, 6, []byte(value))
	}
	result = appendVarintField(result, 9, uint64(p.start.UnixNano()))
	duration := time.Since(p.start).Nanoseconds()
	if duration < 0 {
		duration = 0
	}
	result = appendVarintField(result, 10, uint64(duration))
	compressed := gzip.NewWriter(writer)
	if err := writeAll(compressed, result); err != nil {
		_ = compressed.Close()
		return err
	}
	return compressed.Close()
}

func (p *Profiler) PprofBytes() ([]byte, error) {
	var buffer bytes.Buffer
	if err := p.WritePprof(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (interp *Interpreter) GetProfiler() *Profiler {
	if interp == nil {
		return nil
	}
	return interp.profiler
}

func appendVarint(data []byte, value uint64) []byte {
	var encoded [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(encoded[:], value)
	return append(data, encoded[:n]...)
}

func appendVarintField(data []byte, field int, value uint64) []byte {
	data = appendVarint(data, uint64(field<<3))
	return appendVarint(data, value)
}

func appendBytesField(data []byte, field int, value []byte) []byte {
	data = appendVarint(data, uint64(field<<3|2))
	data = appendVarint(data, uint64(len(value)))
	return append(data, value...)
}

func encodeValueType(typeID, unitID uint64) []byte {
	value := appendVarintField(nil, 1, typeID)
	return appendVarintField(value, 2, unitID)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func trunc(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max-1] + "…"
}
