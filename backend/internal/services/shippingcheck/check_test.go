package shippingcheck

import (
	"strings"
	"testing"
)

func testProfile() Profile {
	return Profile{ID: "office", Name: "สำนักงานใหญ่", House: "368", Moo: "10", District: "คลองมะเดื่อ", DistrictAliases: []string{"Khlong Ma Duea"}, Amphoe: "กระทุ่มแบน", AmphoeAliases: []string{"Krathum Baen"}, Province: "สมุทรสาคร", ProvinceAliases: []string{"Samut Sakhon"}, Postcode: "74110"}
}
func TestShippingChecks(t *testing.T) {
	p := testProfile()
	address := "368 หมู่ 10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110"
	shopee := func(name, addr string) string {
		return `<html><body><p>เรียน คุณ account</p><h3>รายละเอียดการส่งสินค้า</h3><table><tr><td>ชื่อผู้รับ:</td><td>` + name + `</td></tr><tr><td>หมายเลขโทรศัพท์:</td><td>0000000000</td></tr><tr><td>ที่อยู่ในการส่งสินค้า:</td><td>` + addr + `</td></tr></table><h3>รายละเอียดการออกใบกำกับภาษี</h3><p>บริษัทอื่น 999</p></body></html>`
	}
	for _, tt := range []struct{ name, html, want string }{
		{"match", shopee("บริษัท ทดสอบ จำกัด 05", address), "match"},
		{"unapproved receiver", shopee("PD01", address), "mismatch"},
		{"house suffix", shopee("บริษัท ทดสอบ จำกัด 05", strings.Replace(address, "368", "368/1", 1)), "mismatch"},
		{"moo changed", shopee("บริษัท ทดสอบ จำกัด 05", strings.Replace(address, "หมู่ 10", "หมู่ 11", 1)), "mismatch"},
		{"province changed", shopee("บริษัท ทดสอบ จำกัด 05", strings.Replace(address, "สมุทรสาคร", "กรุงเทพมหานคร", 1)), "mismatch"},
		{"postcode changed", shopee("บริษัท ทดสอบ จำกัด 05", strings.Replace(address, "74110", "74111", 1)), "mismatch"},
		{"missing fields", shopee("บริษัท ทดสอบ จำกัด 05", "368 74110"), "unverifiable"},
		{"no section", "<p>บริษัท ทดสอบ จำกัด 05 " + address + "</p>", "unverifiable"},
		{"ambiguous sections", shopee("บริษัท ทดสอบ จำกัด 05", address) + shopee("บริษัท ทดสอบ จำกัด 05", address), "unverifiable"},
		{"lazada bilingual", `<h3>สินค้าของคุณจะได้รับการจัดส่งไปยัง</h3><table><tr><td>ชื่อ-นามสกุล: บริษัท ทดสอบ จำกัด 05</td></tr><tr><td>ที่อยู่: เลขผู้เสียภาษี 0100000000000 บ้านเลขที่ 368 หมู่10 · Khlong Ma Duea / Krathum Baen / Samut Sakhon 74110</td></tr><tr><td>เบอร์โทร: 0000000000</td></tr></table>`, "match"},
		{"duplicated areas", shopee("บริษัท ทดสอบ จำกัด 05", address+" ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน"), "match"},
		{"conflicting district", shopee("บริษัท ทดสอบ จำกัด 05", address+" ตำบลอื่น"), "mismatch"},
		{"extra soi", shopee("บริษัท ทดสอบ จำกัด 05", strings.Replace(address, "หมู่ 10", "หมู่ 10 ซอยมะเดื่อ 10 ซอยวัดแคราย", 1)), "match"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := Check(tt.html, p, []string{"บริษัท ทดสอบ จำกัด 05"})
			if got.Status != tt.want {
				t.Fatalf("got %s want %s: %+v", got.Status, tt.want, got)
			}
		})
	}
}
func TestValidateConfig(t *testing.T) {
	c := Config{Profiles: []Profile{testProfile()}, Policies: []Policy{{AccountID: "mail", Enabled: true, ProfileID: "office", Recipients: []string{"PD01"}}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Policies[0].Recipients = nil
	if c.Validate() == nil {
		t.Fatal("enabled policy without receiver accepted")
	}
}

func TestConflictingEnglishAreaCannotPass(t *testing.T) {
	html := `<h3>สินค้าของคุณจะได้รับการจัดส่งไปยัง</h3><p>ชื่อ-นามสกุล: PD01</p><p>ที่อยู่: 368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร / Bangkok 74110</p><p>เบอร์โทร: 000</p>`
	if r := Check(html, testProfile(), []string{"PD01"}); r.Status == "match" {
		t.Fatal("conflicting English place passed")
	}
}
func TestMultipleShippingNamesInOneSectionCannotPass(t *testing.T) {
	html := `<h3>รายละเอียดการส่งสินค้า</h3><p>ชื่อผู้รับ: PD01</p><p>ที่อยู่: 368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110</p><p>เบอร์โทร: 000</p><p>ชื่อผู้รับ: Someone</p><p>ที่อยู่: 999 หมู่1 จังหวัดอื่น</p>`
	if r := Check(html, testProfile(), []string{"PD01"}); r.Status == "match" {
		t.Fatal("ambiguous shipping identities passed")
	}
}

func TestConflictingRepeatedAddressComponents(t *testing.T) {
	for _, tail := range []string{" บ้านเลขที่ 369", " หมู่11", " 74111"} {
		body := `<h3>รายละเอียดการส่งสินค้า</h3><p>ชื่อผู้รับ: PD01</p><p>ที่อยู่: 368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110` + tail + `</p><p>เบอร์โทร: 000</p>`
		if r := Check(body, testProfile(), []string{"PD01"}); r.Status == "match" {
			t.Fatalf("conflict passed: %s", tail)
		}
	}
}

func TestHouseAndMooSuffixesAreNotIgnored(t *testing.T) {
	for _, address := range []string{"บ้านเลขที่ 368-1 หมู่10", "บ้านเลขที่ 368.1 หมู่10", "บ้านเลขที่ 368 หมู่10-1"} {
		body := `<h3>รายละเอียดการส่งสินค้า</h3><p>ชื่อผู้รับ: PD01</p><p>ที่อยู่: ` + address + ` ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110</p><p>เบอร์โทร: 000</p>`
		if r := Check(body, testProfile(), []string{"PD01"}); r.Status == "match" {
			t.Fatalf("suffix passed: %s", address)
		}
	}
}

func TestMultiOrderEmailOnlyUsesTrailingSharedShippingBlock(t *testing.T) {
	shipping := `<h3>รายละเอียดการส่งสินค้า</h3><p>ชื่อผู้รับ: PD01</p><p>ที่อยู่: 368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110</p><p>เบอร์โทร: 000</p>`
	first := `<p>หมายเลขคำสั่งซื้อ: 261116ABCDEFGH</p>`
	second := `<p>หมายเลขคำสั่งซื้อ: 261116ABCDEFGJ</p>`
	if r := Check(first+shipping+second, testProfile(), []string{"PD01"}); r.Status != "unverifiable" {
		t.Fatalf("per-order shipping applied to all orders: %+v", r)
	}
	if r := Check(first+second+shipping, testProfile(), []string{"PD01"}); r.Status != "match" {
		t.Fatalf("shared shipping: %+v", r)
	}
}

func TestExtractShippingDetailsWithoutPolicy(t *testing.T) {
	html := `<h3>รายละเอียดการส่งสินค้า</h3><p>ชื่อผู้รับ: PD01</p><p>ที่อยู่: 368 หมู่10 ตำบลคลองมะเดื่อ</p><p>เบอร์โทร: 000</p>`
	r := Extract(html)
	if r.Recipient != "PD01" || r.Address != "368 หมู่10 ตำบลคลองมะเดื่อ" {
		t.Fatalf("source fields: %+v", r)
	}
	if r.Status == "match" || len(r.Findings) != 0 {
		t.Fatal("reading source is not a successful policy check")
	}
}

func TestHouseNumberAfterRecipientCompanyPrefix(t *testing.T) {
	name := "บริษัท ไทยซันสปอร์ต จำกัด (สำนักงานใหญ่)"
	for _, tt := range []struct{ name, address, want string }{
		{"customer screenshot", "บริษัท ไทยซันสปอร์ต จำกัด(สำนักงานใหญ่) 368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110", "match"},
		{"changed number", "บริษัท ไทยซันสปอร์ต จำกัด(สำนักงานใหญ่) 369 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110", "mismatch"},
		{"slash suffix", "บริษัท ไทยซันสปอร์ต จำกัด(สำนักงานใหญ่) 368/1 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110", "mismatch"},
		{"unknown company prefix", "บริษัท อื่น จำกัด 368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110", "unverifiable"},
		{"number is not a house", "บริษัท ไทยซันสปอร์ต จำกัด(สำนักงานใหญ่) ซอย 368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110", "unverifiable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			html := `<h3>รายละเอียดการส่งสินค้า</h3><p>ชื่อผู้รับ: ` + name + `</p><p>ที่อยู่ในการส่งสินค้า: ` + tt.address + `</p><p>เบอร์โทร: 000</p>`
			got := Check(html, testProfile(), []string{name})
			if got.Status != tt.want {
				t.Fatalf("got=%s want=%s findings=%+v", got.Status, tt.want, got.Findings)
			}
		})
	}
}

func TestCompanyPrefixUsesLongestApprovedNameWithoutNumericGuessing(t *testing.T) {
	name := "บริษัท ไทยซันสปอร์ต จำกัด (สำนักงานใหญ่)"
	render := func(recipient, address string) string {
		return `<h3>รายละเอียดการส่งสินค้า</h3><p>ชื่อผู้รับ: ` + recipient + `</p><p>ที่อยู่: ` + address + `</p><p>เบอร์โทร: 000</p>`
	}
	address := name + " 0105559145296 368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110"
	got := Check(render(name, address), testProfile(), []string{"บริษัท ไทยซันสปอร์ต จำกัด", name, name + " 0105559145296"})
	if got.Status != "match" {
		t.Fatalf("tax id must not become the house number: %+v", got)
	}
	numberedName := "บริษัท ไทยซันสปอร์ต จำกัด 05"
	got = Check(render(numberedName, "บริษัท ไทยซันสปอร์ต จำกัด 05368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110"), testProfile(), []string{numberedName})
	if got.Status == "match" {
		t.Fatalf("ambiguous company digits must not pass: %+v", got)
	}
	foundHouse := false
	for _, field := range got.Findings {
		if field.Field == "บ้านเลขที่" {
			foundHouse = true
			if field.Actual != "" || field.Status != "unverifiable" {
				t.Fatalf("recipient suffix must not swallow house number digits: %+v", field)
			}
		}
	}
	if !foundHouse {
		t.Fatal("missing house number finding")
	}
}
