package runtime

import (
	"fmt"
	"os"
	goruntime "runtime"
	"sync"
)

var nativeCompilationOnce sync.Once
var nativeCompilationQueue chan func()

func submitNativeCompilation(task func()) bool {
	if task == nil {
		return false
	}
	nativeCompilationOnce.Do(startNativeCompilationWorkers)
	select {
	case nativeCompilationQueue <- task:
		return true
	default:
		return false
	}
}

func startNativeCompilationWorkers() {
	workers := goruntime.GOMAXPROCS(0)
	limit := 4
	if goruntime.GOOS == "android" {
		limit = 2
	}
	if workers < 1 {
		workers = 1
	}
	if workers > limit {
		workers = limit
	}
	nativeCompilationQueue = make(chan func(), 64)
	for i := 0; i < workers; i++ {
		go func() {
			for task := range nativeCompilationQueue {
				runNativeCompilationTask(task)
			}
		}()
	}
}

func runNativeCompilationTask(task func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintf(os.Stderr, "lunex native: background compilation failed unexpectedly: %v\n", recovered)
		}
	}()
	task()
}
