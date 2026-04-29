package sml

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// PartyCache is an in-memory snapshot of SML 248 customers + suppliers.
//
// On boot it fetches both lists, then refreshes every 6 h. Callers can also
// trigger an on-demand RefreshNow (e.g. after admin creates a row in SML).
// Search runs in O(N) over ~1500 records — sub-millisecond, so no need for
// a trie/prefix tree.
type PartyCache struct {
	client *PartyClient
	log    *zap.Logger

	mu        sync.RWMutex
	customers []Party
	suppliers []Party
	lastSync  time.Time

	stopCh chan struct{}
}

func NewPartyCache(client *PartyClient, log *zap.Logger) *PartyCache {
	return &PartyCache{
		client: client,
		log:    log,
		stopCh: make(chan struct{}),
	}
}

// Start runs the initial fetch then loops on a 6 h ticker. Failures are
// logged but never block startup — handlers fall back to empty results,
// admin can click refresh in the UI to retry.
func (pc *PartyCache) Start(ctx context.Context) {
	if !pc.client.IsConfigured() {
		pc.log.Warn("party_cache_skipped_unconfigured")
		return
	}
	go func() {
		if err := pc.RefreshNow(ctx); err != nil {
			pc.log.Error("party_cache_initial_fetch_failed", zap.Error(err))
		}
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-pc.stopCh:
				return
			case <-ticker.C:
				if err := pc.RefreshNow(ctx); err != nil {
					pc.log.Error("party_cache_refresh_failed", zap.Error(err))
				}
			}
		}
	}()
}

func (pc *PartyCache) Stop() {
	select {
	case <-pc.stopCh:
	default:
		close(pc.stopCh)
	}
}

// RefreshNow fetches both customer and supplier lists and atomically swaps
// the cache. Returns the first error encountered (cache is unchanged on
// failure).
func (pc *PartyCache) RefreshNow(ctx context.Context) error {
	start := time.Now()
	customers, err := pc.client.FetchAllCustomers(ctx)
	if err != nil {
		return err
	}
	suppliers, err := pc.client.FetchAllSuppliers(ctx)
	if err != nil {
		return err
	}
	pc.mu.Lock()
	pc.customers = customers
	pc.suppliers = suppliers
	pc.lastSync = time.Now()
	pc.mu.Unlock()
	pc.log.Info("party_cache_refreshed",
		zap.Int("customers", len(customers)),
		zap.Int("suppliers", len(suppliers)),
		zap.Duration("dur", time.Since(start)),
	)
	return nil
}

// LastSync returns when the cache was last filled. Zero value means never.
func (pc *PartyCache) LastSync() time.Time {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.lastSync
}

// Counts returns (customerCount, supplierCount).
func (pc *PartyCache) Counts() (int, int) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return len(pc.customers), len(pc.suppliers)
}

func (pc *PartyCache) listForBillType(billType string) []Party {
	if billType == "purchase" {
		return pc.suppliers
	}
	return pc.customers
}

// GetByCode returns the cached party with the given code, or nil if missing.
func (pc *PartyCache) GetByCode(billType, code string) *Party {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	for i, p := range pc.listForBillType(billType) {
		if p.Code == code {
			return &pc.listForBillType(billType)[i]
		}
	}
	return nil
}

// FindByExactName returns the first party whose Name matches exactly.
// Used by Quick-setup to find the AR00001-04 placeholders.
func (pc *PartyCache) FindByExactName(billType, name string) *Party {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	for i, p := range pc.listForBillType(billType) {
		if p.Name == name {
			return &pc.listForBillType(billType)[i]
		}
	}
	return nil
}

// Search returns up to `limit` parties matching `query`, ranked by relevance.
//
// Matching rules:
//   - empty query → top N alphabetical by code
//   - non-empty   → score each row, descending by score
//
// Scoring (highest wins):
//   100  exact code match
//    90  code starts with query
//    70  code contains query
//    60  name starts with query
//    40  name contains query
//    20  tax_id contains query
//
// Strings are compared case-insensitive for ASCII; Thai is left as-is
// (Thai has no case so strings.Contains works directly).
func (pc *PartyCache) Search(billType, query string, limit int) []Party {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	src := pc.listForBillType(billType)
	if limit <= 0 {
		limit = 20
	}
	if query == "" {
		out := make([]Party, 0, limit)
		for i := 0; i < len(src) && i < limit; i++ {
			out = append(out, src[i])
		}
		return out
	}

	q := strings.ToLower(strings.TrimSpace(query))
	type scored struct {
		party Party
		score int
	}
	var hits []scored
	for _, p := range src {
		code := strings.ToLower(p.Code)
		name := strings.ToLower(p.Name)
		taxID := strings.ToLower(p.TaxID)

		score := 0
		switch {
		case code == q:
			score = 100
		case strings.HasPrefix(code, q):
			score = 90
		case strings.Contains(code, q):
			score = 70
		}
		// also check name (best of code-score vs name-score)
		switch {
		case strings.HasPrefix(name, q):
			if 60 > score {
				score = 60
			}
		case strings.Contains(name, q):
			if 40 > score {
				score = 40
			}
		}
		// Thai-friendly: original-case substring
		if score == 0 && strings.Contains(p.Name, query) {
			score = 40
		}
		if score == 0 && taxID != "" && strings.Contains(taxID, q) {
			score = 20
		}
		if score > 0 {
			hits = append(hits, scored{party: p, score: score})
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].party.Code < hits[j].party.Code
	})

	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Party, len(hits))
	for i, h := range hits {
		out[i] = h.party
	}
	return out
}
