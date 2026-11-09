//go:build wasip1

package std

func fsEnsureHydrated() {}

func fsPersistFile(key string, data []byte) {}

func fsPersistDir(key string) {}

func fsPersistDelete(key string) {}

func fsPersistDeletePrefix(key string) {}

// OnFsHydrated runs cb immediately: wasip1 has no persisted backend to wait for.
func OnFsHydrated(cb func()) {
	cb()
}
