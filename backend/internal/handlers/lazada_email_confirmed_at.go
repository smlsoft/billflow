package handlers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// thaiMonthFull maps Thai full month names → month number.
var thaiMonthFull = map[string]int{
	"มกราคม":  1,
	"กุมภาพันธ์": 2,
	"มีนาคม":  3,
	"เมษายน":  4,
	"พฤษภาคม": 5,
	"มิถุนายน": 6,
	"กรกฎาคม": 7,
	"สิงหาคม": 8,
	"กันยายน": 9,
	"ตุลาคม":  10,
	"พฤศจิกายน": 11,
	"ธันวาคม": 12,
}

// thaiMonthAbbr maps Thai abbreviated month names → month number.
var thaiMonthAbbr = map[string]int{
	"ม.ค.":  1,
	"ก.พ.":  2,
	"มี.ค.": 3,
	"เม.ย.": 4,
	"พ.ค.":  5,
	"มิ.ย.": 6,
	"ก.ค.":  7,
	"ส.ค.":  8,
	"ก.ย.":  9,
	"ต.ค.":  10,
	"พ.ย.":  11,
	"ธ.ค.":  12,
}

// Lazada TH confirmation email pattern:
// "เราได้รับหมายเลขคำสั่งซื้อ ... เมื่อ <day> <month> <year> เวลา <HH:MM>"
// The pattern uses a broad [^\n]* to skip any order number between keyword and เมื่อ.
var lazadaConfirmedAtRe = regexp.MustCompile(
	`(?i)เราได้รับหมายเลขคำสั่งซื้อ[^\n]*เมื่อ\s+` +
		`(\d{1,2})\s+(\S+)\s+(\d{4})\s+เวลา\s+(\d{1,2}:\d{2})`,
)

// lazadaThaiLoc is UTC+7 used for all Lazada TH timestamps.
var lazadaThaiLoc = time.FixedZone("Asia/Bangkok", 7*60*60)

// ExtractLazadaConfirmedAt parses the Lazada purchase confirmation text to find
// the minute-level confirmation time stated in the email body. It searches
// plainText first, then bodyHTML.
//
// accountID is included in the group key so bills from different IMAP accounts
// never collide (a safety measure; in practice all Lazada emails for one shop
// arrive via one account).
//
// Returns (confirmedAt, groupKey) where:
//   - confirmedAt is RFC3339 to minute precision with +07:00 offset, e.g. "2026-06-11T16:45:00+07:00"
//   - groupKey is "lazada_card_<accountID>_YYYYMMDD_HHMM", safe to use as a DB lookup key
//
// Both strings are empty when the pattern is not found (graceful fallback to email_date grouping).
func ExtractLazadaConfirmedAt(plainText, bodyHTML, accountID string) (confirmedAt, groupKey string) {
	t, ok := extractLazadaConfirmedAtFromText(plainText)
	if !ok {
		t, ok = extractLazadaConfirmedAtFromText(bodyHTML)
	}
	if !ok {
		return "", ""
	}
	accountPart := strings.ReplaceAll(strings.TrimSpace(accountID), "_", "-")
	if accountPart == "" {
		accountPart = "default"
	}
	confirmedAt = t.Format(time.RFC3339)
	groupKey = fmt.Sprintf("lazada_card_%s_%s", accountPart, t.Format("20060102_1504"))
	return confirmedAt, groupKey
}

func extractLazadaConfirmedAtFromText(text string) (time.Time, bool) {
	m := lazadaConfirmedAtRe.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}, false
	}
	day, err := strconv.Atoi(m[1])
	if err != nil || day < 1 || day > 31 {
		return time.Time{}, false
	}
	month, ok := parseThaiMonth(m[2])
	if !ok {
		return time.Time{}, false
	}
	yearRaw, err := strconv.Atoi(m[3])
	if err != nil {
		return time.Time{}, false
	}
	year := yearRaw
	if year > 2400 {
		year -= 543
	}
	if year < 2000 || year > 2100 {
		return time.Time{}, false
	}
	parts := strings.SplitN(m[4], ":", 2)
	if len(parts) != 2 {
		return time.Time{}, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return time.Time{}, false
	}
	t := time.Date(year, time.Month(month), day, hour, minute, 0, 0, lazadaThaiLoc)
	return t, true
}

func parseThaiMonth(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if n, ok := thaiMonthFull[s]; ok {
		return n, true
	}
	if n, ok := thaiMonthAbbr[s]; ok {
		return n, true
	}
	return 0, false
}
