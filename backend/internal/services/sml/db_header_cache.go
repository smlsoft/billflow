package sml

import (
	"sync"
	"time"
)

const dbCacheTTL = 30 * time.Second

type dbCacheEntry struct {
	cfg      *SMLDBHeaderConfig
	loadedAt time.Time
}

var (
	dbCache   dbCacheEntry
	dbCacheMu sync.Mutex
)

// GetCachedDBConfig returns the cached SMLDBHeaderConfig, calling loader() to
// refresh when the cache is empty or older than dbCacheTTL (30 s).
// Returns nil when the loader returns an error or an incomplete config.
// The loader is called with the mutex held — keep it fast (a single DB read).
func GetCachedDBConfig(loader func() (*SMLDBHeaderConfig, error)) *SMLDBHeaderConfig {
	dbCacheMu.Lock()
	defer dbCacheMu.Unlock()

	if !dbCache.loadedAt.IsZero() && time.Since(dbCache.loadedAt) < dbCacheTTL {
		return dbCache.cfg
	}

	cfg, err := loader()
	if err != nil || cfg == nil || !cfg.IsSet() {
		dbCache = dbCacheEntry{cfg: nil, loadedAt: time.Now()}
		return nil
	}

	dbCache = dbCacheEntry{cfg: cfg, loadedAt: time.Now()}
	return cfg
}

// InvalidateDBHeaderCache drops the cached entry so that the next request
// picks up fresh values from the database. Call this after admin saves
// /settings/instance to ensure the new DB config takes effect immediately.
func InvalidateDBHeaderCache() {
	dbCacheMu.Lock()
	dbCache = dbCacheEntry{}
	dbCacheMu.Unlock()
}
