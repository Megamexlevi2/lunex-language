//go:build js

package std

import "sync"

var fsHydrateOnce sync.Once
var fsHydrateMu sync.Mutex
var fsHydrateDone bool
var fsHydrateWaiters []func()

func fsEnsureHydrated() {
	fsHydrateOnce.Do(func() {
		fsBackend.init(func() {
			fsBackend.loadAll(func(key string, value []byte) {
				isDir := len(value) == 1 && value[0] == 0
				globalVFS.loadFromStore(key, value, isDir)
			}, fsMarkHydrated)
		})
	})
}

func fsMarkHydrated() {
	fsHydrateMu.Lock()
	fsHydrateDone = true
	waiters := fsHydrateWaiters
	fsHydrateWaiters = nil
	fsHydrateMu.Unlock()
	for _, w := range waiters {
		w()
	}
}

func fsOnHydrated(cb func()) {
	fsHydrateMu.Lock()
	if fsHydrateDone {
		fsHydrateMu.Unlock()
		cb()
		return
	}
	fsHydrateWaiters = append(fsHydrateWaiters, cb)
	fsHydrateMu.Unlock()
}

// OnFsHydrated registers a callback that runs once the virtual filesystem
// has finished loading its persisted state (or immediately, if there is
// nothing to load). cmd/lunexwasm uses this to expose Lunex.ready().
func OnFsHydrated(cb func()) {
	fsEnsureHydrated()
	fsOnHydrated(cb)
}

func fsPersistFile(key string, data []byte) {
	if fsBackend.isUsable() {
		fsBackend.putAsync(key, data)
	}
}

func fsPersistDir(key string) {
	if fsBackend.isUsable() {
		fsBackend.putAsync(key, []byte{0})
	}
}

func fsPersistDelete(key string) {
	if fsBackend.isUsable() {
		fsBackend.deleteAsync(key)
	}
}

func fsPersistDeletePrefix(key string) {
	if fsBackend.isUsable() {
		fsBackend.deletePrefixAsync(key)
	}
}
