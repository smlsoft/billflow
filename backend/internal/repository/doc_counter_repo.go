package repository

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DocCounterRepo serves atomic counters per (prefix, period) for SML doc_no
// generation. Each call to NextSeq increments and returns the value to use,
// so two retries racing the same channel don't collide on doc_no.
type DocCounterRepo struct {
	db *sql.DB
}

func NewDocCounterRepo(db *sql.DB) *DocCounterRepo {
	return &DocCounterRepo{db: db}
}

// NextSeq atomically increments the counter for (prefix, period) and returns
// the seq value to use right now. First call for a (prefix, period) returns 1.
func (r *DocCounterRepo) NextSeq(prefix, period string) (int, error) {
	var seq int
	err := r.db.QueryRow(
		`INSERT INTO doc_counters (prefix, period, last_used_seq, updated_at)
		 VALUES ($1, $2, 1, NOW())
		 ON CONFLICT (prefix, period) DO UPDATE SET
		   last_used_seq = doc_counters.last_used_seq + 1,
		   updated_at    = NOW()
		 RETURNING last_used_seq`,
		prefix, period,
	).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("doc counter increment: %w", err)
	}
	return seq, nil
}

// GenerateDocNo renders prefix + format with date tokens substituted and the
// sequence counter atomically incremented.
//
// Format tokens (case-sensitive):
//
//	YYYY  → 4-digit year       (e.g. 2026)
//	YY    → 2-digit year       (26)
//	MM    → 2-digit month      (04)
//	DD    → 2-digit day        (28)
//	#...  → zero-padded counter; the count of #s is the padding width
//	        (so "####" = 4 digits, "#####" = 5 digits)
//
// Period for counter reset is derived from which date tokens the format uses:
//
//	contains DD → resets daily        (period = YYYYMMDD)
//	contains MM → resets monthly      (period = YYYYMM, default for YYMM####)
//	contains YY → resets yearly       (period = YYYY)
//	none of the above → never resets  (period = "_")
//
// Both prefix and format default to safe values when blank: "BF" and "YYMM####".
func (r *DocCounterRepo) GenerateDocNo(prefix, format string, now time.Time) (string, error) {
	if prefix == "" {
		prefix = "BF"
	}
	if format == "" {
		format = "YYMM####"
	}

	yyyy := fmt.Sprintf("%04d", now.Year())
	yy := fmt.Sprintf("%02d", now.Year()%100)
	mm := fmt.Sprintf("%02d", int(now.Month()))
	dd := fmt.Sprintf("%02d", now.Day())

	var period string
	switch {
	case strings.Contains(format, "DD"):
		period = yyyy + mm + dd
	case strings.Contains(format, "MM"):
		period = yyyy + mm
	case strings.Contains(format, "YYYY") || strings.Contains(format, "YY"):
		period = yyyy
	default:
		period = "_"
	}

	seq, err := r.NextSeq(prefix, period)
	if err != nil {
		return "", err
	}

	// Count contiguous # block to determine pad width. Default to 4 if absent.
	width := 4
	if hashRe.MatchString(format) {
		width = len(hashRe.FindString(format))
	}
	seqStr := fmt.Sprintf("%0*d", width, seq)

	out := format
	// YYYY before YY so the longer match consumes the substring first.
	out = strings.ReplaceAll(out, "YYYY", yyyy)
	out = strings.ReplaceAll(out, "YY", yy)
	out = strings.ReplaceAll(out, "MM", mm)
	out = strings.ReplaceAll(out, "DD", dd)
	out = hashRe.ReplaceAllString(out, seqStr)

	return prefix + out, nil
}

var hashRe = regexp.MustCompile(`#+`)
