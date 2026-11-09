package runtime

import (
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

var adaptiveGCTunerOnce sync.Once

func StartAdaptiveGCController(defaultMemoryLimit int64) {
	if os.Getenv("GOGC") != "" {
		return
	}
	if raw := strings.TrimSpace(strings.ToLower(os.Getenv("LUNEX_GC_ADAPTIVE"))); raw == "0" || raw == "false" || raw == "off" || raw == "no" {
		return
	}
	if defaultMemoryLimit <= 0 {
		defaultMemoryLimit = 200 << 20
	}
	if configured := parseMemoryLimit(os.Getenv("GOMEMLIMIT")); configured != 0 {
		defaultMemoryLimit = configured
	}
	adaptiveGCTunerOnce.Do(func() {
		go func(limit int64) {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			lastPercent := -1
			for range ticker.C {
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				ratio := float64(stats.HeapAlloc) / float64(limit)
				percent := 70
				switch {
				case ratio >= 0.92:
					percent = 10
				case ratio >= 0.84:
					percent = 18
				case ratio >= 0.74:
					percent = 28
				case ratio >= 0.60:
					percent = 40
				case ratio >= 0.45:
					percent = 55
				}
				if percent != lastPercent {
					debug.SetGCPercent(percent)
					lastPercent = percent
				}
			}
		}(defaultMemoryLimit)
	})
}

func parseMemoryLimit(raw string) int64 {
	value := strings.TrimSpace(raw)
	if value == "" || strings.EqualFold(value, "off") || strings.EqualFold(value, "-1") {
		return 0
	}
	units := []struct {
		suffix string
		factor int64
	}{{"gib", 1 << 30}, {"gb", 1_000_000_000}, {"mib", 1 << 20}, {"mb", 1_000_000}, {"kib", 1 << 10}, {"kb", 1_000}, {"b", 1}}
	lower := strings.ToLower(value)
	for _, unit := range units {
		if strings.HasSuffix(lower, unit.suffix) {
			number := strings.TrimSpace(value[:len(value)-len(unit.suffix)])
			n, err := strconv.ParseInt(number, 10, 64)
			if err != nil || n <= 0 || n > (1<<63-1)/unit.factor {
				return 0
			}
			return n * unit.factor
		}
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
