package bytecode

import (
	"fmt"
	"lunex/internal/adaptor"
)

func CacheDir() string {
	return adaptor.CacheDir()
}

func CacheKey(absPath string) (string, error) {
	key, err := adaptor.MemCacheKey(absPath)
	if err != nil {
		return "", fmt.Errorf("cache key error: %w", err)
	}
	return key, nil
}

func CacheLookup(absPath string) ([]byte, bool) {
	return adaptor.CacheLookup(absPath)
}

func CacheStore(absPath string, objectData []byte) error {
	adaptor.CacheStore(absPath, objectData)
	return nil
}

func CacheInvalidate(absPath string) {
	adaptor.CacheInvalidate(absPath)
}

var overrideCacheDir string

func SetCacheDir(dir string) error {
	overrideCacheDir = dir
	return nil
}

func UnpackNAX(data []byte, outDir string) (int, error) {
	return unpackNAXData(data, outDir)
}

func RecoverNAX(data []byte, outDir string) (int, error) {
	return recoverNAXData(data, outDir)
}
