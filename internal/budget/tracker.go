package budget

import (
	"sync"
	"time"
)

// SpendRecord tracks cumulative spending for a budget key (e.g. a developer
// identifier or API endpoint). Spend is automatically reset at the start of
// each calendar month.
type SpendRecord struct {
	Key          string    `json:"key"`
	TotalSpend   float64   `json:"total_spend"`
	TotalTokens  int64     `json:"total_tokens"`
	RequestCount int64     `json:"request_count"`
	MonthlyLimit float64   `json:"monthly_limit"`
	LastResetAt  time.Time `json:"last_reset_at"`
}

// Tracker tracks spending across developers and endpoints using in-memory
// counters. All operations are safe for concurrent use.
type Tracker struct {
	mu           sync.RWMutex
	records      map[string]*SpendRecord
	catalog      *PricingCatalog
	defaultLimit float64
}

// NewTracker creates a new spend tracker with the given pricing catalog and
// default monthly spending limit (in US dollars). A defaultMonthlyLimit of 0
// means no limit.
func NewTracker(catalog *PricingCatalog, defaultMonthlyLimit float64) *Tracker {
	return &Tracker{
		records:      make(map[string]*SpendRecord),
		catalog:      catalog,
		defaultLimit: defaultMonthlyLimit,
	}
}

// RecordUsage records token usage and cost for a budget key. It returns the
// updated spend record after applying the usage.
func (t *Tracker) RecordUsage(key string, usage TokenUsage, cost float64) *SpendRecord {
	t.mu.Lock()
	defer t.mu.Unlock()

	record := t.getOrCreateLocked(key)
	t.resetIfNeeded(record)

	record.TotalSpend += cost
	record.TotalTokens += int64(usage.TotalTokens)
	record.RequestCount++

	return record
}

// CheckBudget checks whether the budget key has remaining budget. It returns
// the remaining dollar amount and whether the next request should be allowed.
// If no limit is configured (limit ≤ 0), the request is always allowed and
// remaining is reported as positive infinity.
func (t *Tracker) CheckBudget(key string) (remaining float64, allowed bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	record := t.getOrCreateLocked(key)
	t.resetIfNeeded(record)

	if record.MonthlyLimit <= 0 {
		// No limit configured — always allowed.
		return record.MonthlyLimit, true
	}

	remaining = record.MonthlyLimit - record.TotalSpend
	return remaining, record.TotalSpend < record.MonthlyLimit
}

// SetLimit sets a custom monthly spending limit for a specific key, overriding
// the default.
func (t *Tracker) SetLimit(key string, limit float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	record := t.getOrCreateLocked(key)
	record.MonthlyLimit = limit
}

// GetRecord returns a copy of the current spend record for a key, or nil if
// no record exists.
func (t *Tracker) GetRecord(key string) *SpendRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	record, ok := t.records[key]
	if !ok {
		return nil
	}

	// Return a copy to avoid data races.
	cp := *record
	return &cp
}

// AllRecords returns a snapshot of all spend records. The returned slice
// contains copies that are safe to read without holding the tracker's lock.
func (t *Tracker) AllRecords() []*SpendRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*SpendRecord, 0, len(t.records))
	for _, r := range t.records {
		cp := *r
		result = append(result, &cp)
	}
	return result
}

// getOrCreateLocked returns the record for key, creating one with default
// values if it does not exist. The caller must hold t.mu.
func (t *Tracker) getOrCreateLocked(key string) *SpendRecord {
	record, ok := t.records[key]
	if !ok {
		record = &SpendRecord{
			Key:          key,
			MonthlyLimit: t.defaultLimit,
			LastResetAt:  time.Now(),
		}
		t.records[key] = record
	}
	return record
}

// resetIfNeeded checks whether the calendar month has changed since the last
// reset and, if so, zeroes out the spend and token counters.
func (t *Tracker) resetIfNeeded(record *SpendRecord) {
	now := time.Now()
	if record.LastResetAt.Year() != now.Year() || record.LastResetAt.Month() != now.Month() {
		record.TotalSpend = 0
		record.TotalTokens = 0
		record.RequestCount = 0
		record.LastResetAt = now
	}
}
