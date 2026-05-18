package catalog

import "testing"

func TestParseProductV4ResponseBillFlowNativeMeta(t *testing.T) {
	body := []byte(`{
		"success": true,
		"data": [
			{
				"code": "BF00002",
				"name_1": "สินค้า",
				"name_2": "",
				"unit_standard": "ชิ้น",
				"group_code": "G1",
				"balance_qty": 3,
				"price": 12.5
			}
		],
		"meta": {"total": 201, "page": 2, "size": 100}
	}`)

	items, page, maxPage, err := parseProductV4Response(body)
	if err != nil {
		t.Fatal(err)
	}
	if page != 2 || maxPage != 3 {
		t.Fatalf("page=%d maxPage=%d, want 2/3", page, maxPage)
	}
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
	if items[0].GroupCodeV1 != "G1" || items[0].Price != 12.5 {
		t.Fatalf("item = %+v", items[0])
	}
}
