package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseShopeeExcelAprilExportWithoutSKU(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Order.all.20260401_20260430.xlsx")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("real Shopee sample file is not present")
		}
		t.Fatalf("open sample: %v", err)
	}
	defer f.Close()

	orders, warnings, skipped, err := parseShopeeExcel(f)
	if err != nil {
		t.Fatalf("parse sample: %v", err)
	}
	if got, want := len(orders), 53; got != want {
		t.Fatalf("orders = %d, want %d; warnings=%v", got, want, warnings)
	}
	itemCount := 0
	noSKUItems := 0
	multiLineOrders := 0
	for _, order := range orders {
		itemCount += len(order.Items)
		if order.HasNoSKU {
			noSKUItems += order.NoSKUItemCount
		}
		if order.MultiLine {
			multiLineOrders++
		}
		for _, item := range order.Items {
			if item.RawName == "" {
				t.Fatalf("order %s has item without raw_name", order.OrderID)
			}
		}
	}
	if got, want := itemCount, 58; got != want {
		t.Fatalf("items = %d, want %d", got, want)
	}
	if got, want := noSKUItems, 58; got != want {
		t.Fatalf("no sku items = %d, want %d", got, want)
	}
	if got, want := multiLineOrders, 5; got != want {
		t.Fatalf("multi-line orders = %d, want %d", got, want)
	}
	if got, want := skipped, 6; got != want {
		t.Fatalf("skipped rows = %d, want %d", got, want)
	}
}
