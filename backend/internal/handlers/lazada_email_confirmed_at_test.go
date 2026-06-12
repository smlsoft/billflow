package handlers

import (
	"testing"
)

func TestExtractLazadaConfirmedAtFullThaiDate(t *testing.T) {
	body := "เราได้รับหมายเลขคำสั่งซื้อ 123456789 เมื่อ 11 มิถุนายน 2569 เวลา 16:45"
	confirmedAt, groupKey := ExtractLazadaConfirmedAt(body, "", "ACC001")
	if confirmedAt != "2026-06-11T16:45:00+07:00" {
		t.Errorf("confirmedAt = %q, want 2026-06-11T16:45:00+07:00", confirmedAt)
	}
	if groupKey != "lazada_card_ACC001_20260611_1645" {
		t.Errorf("groupKey = %q, want lazada_card_ACC001_20260611_1645", groupKey)
	}
}

// All 8 orders arrive with envelope seconds 17:45:59 / 17:46:00 / 17:46:01
// but body says "เวลา 16:45" → all must produce the same groupKey.
func TestExtractLazadaConfirmedAtSameMinuteAcrossEnvelopeSeconds(t *testing.T) {
	bodies := []string{
		"เราได้รับหมายเลขคำสั่งซื้อ AAA เมื่อ 11 มิถุนายน 2569 เวลา 16:45",
		"เราได้รับหมายเลขคำสั่งซื้อ BBB เมื่อ 11 มิถุนายน 2569 เวลา 16:45",
		"เราได้รับหมายเลขคำสั่งซื้อ CCC เมื่อ 11 มิถุนายน 2569 เวลา 16:45",
	}
	expected := "lazada_card_ACC001_20260611_1645"
	for _, body := range bodies {
		_, gk := ExtractLazadaConfirmedAt(body, "", "ACC001")
		if gk != expected {
			t.Errorf("groupKey = %q, want %q (body: %q)", gk, expected, body)
		}
	}
}

func TestExtractLazadaConfirmedAtNoMatch(t *testing.T) {
	confirmedAt, groupKey := ExtractLazadaConfirmedAt("some unrelated text", "<html>no match</html>", "ACC001")
	if confirmedAt != "" || groupKey != "" {
		t.Errorf("expected empty strings, got confirmedAt=%q groupKey=%q", confirmedAt, groupKey)
	}
}

func TestExtractLazadaConfirmedAtHTMLFallback(t *testing.T) {
	html := "<p>เราได้รับหมายเลขคำสั่งซื้อ 999 เมื่อ 5 มกราคม 2568 เวลา 09:30</p>"
	confirmedAt, groupKey := ExtractLazadaConfirmedAt("", html, "ACC002")
	if confirmedAt != "2025-01-05T09:30:00+07:00" {
		t.Errorf("confirmedAt = %q, want 2025-01-05T09:30:00+07:00", confirmedAt)
	}
	if groupKey != "lazada_card_ACC002_20250105_0930" {
		t.Errorf("groupKey = %q, want lazada_card_ACC002_20250105_0930", groupKey)
	}
}

func TestExtractLazadaConfirmedAtEmptyAccountID(t *testing.T) {
	body := "เราได้รับหมายเลขคำสั่งซื้อ X เมื่อ 1 ธันวาคม 2567 เวลา 23:59"
	_, groupKey := ExtractLazadaConfirmedAt(body, "", "")
	if groupKey != "lazada_card_default_20241201_2359" {
		t.Errorf("groupKey = %q, want lazada_card_default_20241201_2359", groupKey)
	}
}

func TestExtractLazadaConfirmedAtAbbreviatedMonth(t *testing.T) {
	body := "เราได้รับหมายเลขคำสั่งซื้อ Y เมื่อ 3 มิ.ย. 2569 เวลา 10:00"
	confirmedAt, groupKey := ExtractLazadaConfirmedAt(body, "", "ACC003")
	if confirmedAt != "2026-06-03T10:00:00+07:00" {
		t.Errorf("confirmedAt = %q, want 2026-06-03T10:00:00+07:00", confirmedAt)
	}
	if groupKey != "lazada_card_ACC003_20260603_1000" {
		t.Errorf("groupKey = %q, want lazada_card_ACC003_20260603_1000", groupKey)
	}
}
