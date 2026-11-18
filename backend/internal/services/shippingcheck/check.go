// Package shippingcheck checks only explicit shipping sections, without AI or network access.
package shippingcheck

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const Version = "shipping-v3"

type Profile struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	House           string   `json:"house"`
	Moo             string   `json:"moo"`
	District        string   `json:"district"`
	Amphoe          string   `json:"amphoe"`
	Province        string   `json:"province"`
	Postcode        string   `json:"postcode"`
	DistrictAliases []string `json:"district_aliases"`
	AmphoeAliases   []string `json:"amphoe_aliases"`
	ProvinceAliases []string `json:"province_aliases"`
}
type Policy struct {
	AccountID  string   `json:"account_id"`
	ProfileID  string   `json:"profile_id"`
	Recipients []string `json:"recipients"`
	Enabled    bool     `json:"enabled"`
	EnabledAt  string   `json:"enabled_at,omitempty"`
}
type Config struct {
	Profiles []Profile `json:"profiles"`
	Policies []Policy  `json:"policies"`
}
type Finding struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Status   string `json:"status"`
}
type Result struct {
	Trigger   string    `json:"trigger,omitempty"`
	CheckedBy string    `json:"checked_by,omitempty"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	Recipient string    `json:"recipient"`
	Address   string    `json:"address"`
	Findings  []Finding `json:"findings"`
	Version   string    `json:"version"`
}

func (c Config) Validate() error {
	if len(c.Profiles) > 50 || len(c.Policies) > 200 {
		return fmt.Errorf("จำนวนโปรไฟล์หรือกล่องอีเมลเกินกำหนด")
	}
	ids := map[string]bool{}
	for _, p := range c.Profiles {
		if p.ID == "" || ids[p.ID] || p.Name == "" {
			return fmt.Errorf("รหัสโปรไฟล์ต้องไม่ว่างหรือซ้ำ และต้องระบุชื่อ")
		}
		ids[p.ID] = true
		for _, v := range []string{p.ID, p.Name, p.House, p.Moo, p.District, p.Amphoe, p.Province, p.Postcode} {
			if len(v) > 500 {
				return fmt.Errorf("ข้อมูลโปรไฟล์ยาวเกินกำหนด")
			}
		}
		if !regexp.MustCompile(`^\d+(?:/\d+)?$`).MatchString(p.House) || !regexp.MustCompile(`^\d+$`).MatchString(p.Moo) || p.District == "" || p.Amphoe == "" || p.Province == "" || !regexp.MustCompile(`^\d{5}$`).MatchString(p.Postcode) {
			return fmt.Errorf("กรอกบ้านเลขที่ หมู่ ตำบล อำเภอ จังหวัด และรหัสไปรษณีย์ 5 หลักให้ครบ")
		}
		for _, aliases := range [][]string{p.DistrictAliases, p.AmphoeAliases, p.ProvinceAliases} {
			if err := validateNames(aliases); err != nil {
				return err
			}
		}
	}
	accounts := map[string]bool{}
	for _, p := range c.Policies {
		if p.AccountID == "" || accounts[p.AccountID] {
			return fmt.Errorf("กล่องอีเมลต้องไม่ว่างหรือซ้ำ")
		}
		accounts[p.AccountID] = true
		if err := validateNames(p.Recipients); err != nil {
			return err
		}
		if p.Enabled && (!ids[p.ProfileID] || len(p.Recipients) == 0) {
			return fmt.Errorf("เลือกโปรไฟล์และกรอกชื่อผู้รับก่อนเปิดตรวจ")
		}
	}
	return nil
}
func validateNames(names []string) error {
	if len(names) > 50 {
		return fmt.Errorf("ชื่อที่อนุญาตเกินกำหนด")
	}
	for _, n := range names {
		if norm(n) == "" || len(n) > 500 {
			return fmt.Errorf("ชื่อหรือชื่อพื้นที่ไม่ถูกต้อง")
		}
	}
	return nil
}
func norm(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

// textLines retains table cell and block boundaries. Scripts/styles never become evidence.
func textLines(src string) string {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "head") {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "td", "tr", "p", "div", "br", "h1", "h2", "h3", "h4", "li":
				b.WriteByte('\n')
			}
		}
	}
	walk(doc)
	return b.String()
}

var sectionRE = regexp.MustCompile(`รายละเอียดการส่งสินค้า|สินค้าของคุณจะได้รับการจัดส่งไปยัง`)
var nameRE = regexp.MustCompile(`(?s)(?:ชื่อผู้รับ|ชื่อ\s*-\s*นามสกุล)\s*[:：]\s*(.*?)(?:หมายเลขโทรศัพท์|เบอร์โทร|ที่อยู่ในการส่งสินค้า|ที่อยู่\s*[:：])`)
var addressRE = regexp.MustCompile(`(?s)(?:ที่อยู่ในการส่งสินค้า|ที่อยู่)\s*[:：]\s*(.*?)(?:เบอร์โทร|หมายเลขโทรศัพท์|อีเมล|รายละเอียดการออกใบกำกับภาษี|รายละเอียดการชำระเงิน|$)`)

// Extract reads shipping fields without applying any policy or declaring a match.
func Extract(src string) Result {
	r := Result{Status: "unverifiable", Version: Version, Findings: []Finding{}}
	if len(src) > 4*1024*1024 {
		r.Reason = "อีเมลใหญ่เกินขนาดที่ตรวจได้"
		return r
	}
	text := textLines(src)
	locs := sectionRE.FindAllStringIndex(text, -1)
	if len(locs) != 1 {
		r.Reason = "ไม่พบส่วนจัดส่งที่ชัดเจนเพียงส่วนเดียว"
		return r
	}
	// A multi-order email is shared only when its sole shipping block trails
	// all identifiable order blocks. A recipient inside one order is ambiguous.
	orderRE := regexp.MustCompile(`\b(?:[0-9]{6}[A-Z0-9]{8}|[0-9]{16})\b`)
	orderIDs := map[string]bool{}
	for _, id := range orderRE.FindAllString(strings.ToUpper(text), -1) {
		orderIDs[id] = true
	}
	if len(orderIDs) > 1 && orderRE.MatchString(strings.ToUpper(text[locs[0][1]:])) {
		r.Reason = "อีเมลมีหลายคำสั่งซื้อและไม่ยืนยันว่าข้อมูลจัดส่งใช้ร่วมกัน"
		return r
	}
	section := text[locs[0][1]:]
	// Stop at an invoice or payment block; never borrow its identity/address.
	for _, end := range []string{"รายละเอียดการออกใบกำกับภาษี", "รายละเอียดการชำระเงิน"} {
		if i := strings.Index(section, end); i >= 0 {
			section = section[:i]
		}
	}
	nameCount := len(nameRE.FindAllStringSubmatch(section, -1))
	addressCount := len(addressRE.FindAllStringSubmatch(section, -1))
	if nameCount > 1 || addressCount > 1 {
		r.Reason = "ข้อมูลผู้รับหรือที่อยู่จัดส่งมีหลายชุด ไม่สามารถจับคู่ได้"
		return r
	}
	n := nameRE.FindStringSubmatch(section)
	a := addressRE.FindStringSubmatch(section)
	if len(n) > 1 {
		r.Recipient = strings.Join(strings.Fields(n[1]), " ")
	}
	if len(a) > 1 {
		r.Address = strings.Join(strings.Fields(a[1]), " ")
	}
	if r.Recipient == "" || r.Address == "" {
		r.Reason = "ไม่พบชื่อผู้รับหรือที่อยู่จัดส่งครบถ้วน"
		return r
	}
	return r
}
func Check(src string, p Profile, recipients []string) Result {
	r := Extract(src)
	if r.Reason != "" {
		return r
	}
	add := func(field, expected, actual string, matched bool) {
		status := "mismatch"
		if actual == "" {
			status = "unverifiable"
		} else if matched {
			status = "match"
		}
		r.Findings = append(r.Findings, Finding{field, expected, actual, status})
	}
	ok := false
	for _, v := range recipients {
		if norm(v) == norm(r.Recipient) {
			ok = true
		}
	}
	add("ชื่อผู้รับ", strings.Join(recipients, " / "), r.Recipient, ok)
	house := extractHouse(r.Address, append([]string{r.Recipient}, recipients...)...)
	add("บ้านเลขที่", p.House, house, house == p.House)
	moo := regexp.MustCompile(`หมู่(?:ที่)?\s*(\d+(?:[/.\-]\d+)*)`).FindAllStringSubmatch(r.Address, -1)
	mv := uniqueCaptures(moo)
	add("หมู่", p.Moo, mv, mv == p.Moo)
	for _, v := range []struct {
		field, label, expected string
		aliases                []string
	}{
		{"ตำบล", `(?:ตำบล|ต\.)`, p.District, p.DistrictAliases},
		{"อำเภอ", `(?:อำเภอ|อ\.)`, p.Amphoe, p.AmphoeAliases},
		{"จังหวัด", `(?:จังหวัด|จ\.)`, p.Province, p.ProvinceAliases},
	} {
		actual, matched := area(r.Address, v.label, append([]string{v.expected}, v.aliases...))
		add(v.field, v.expected, actual, matched)
	}
	codes := regexp.MustCompile(`[0-9]+`).FindAllString(r.Address, -1)
	cv := ""
	valid := true
	for _, code := range codes {
		if len(code) != 5 {
			continue
		}
		if cv != "" {
			cv += " / "
		}
		cv += code
		if code != p.Postcode {
			valid = false
		}
	}
	add("รหัสไปรษณีย์", p.Postcode, cv, valid)
	// Unexpected Latin place text must not be silently ignored in bilingual addresses.
	allowedEnglish := append(append(append([]string{}, p.DistrictAliases...), p.AmphoeAliases...), p.ProvinceAliases...)
	allowedEnglish = append(allowedEnglish, p.District, p.Amphoe, p.Province)
	for _, phrase := range regexp.MustCompile(`[A-Za-z]+(?:\s+[A-Za-z]+)*`).FindAllString(r.Address, -1) {
		known := false
		for _, alias := range allowedEnglish {
			if norm(phrase) == norm(alias) {
				known = true
				break
			}
		}
		if !known {
			r.Findings = append(r.Findings, Finding{Field: "ข้อความที่อยู่เพิ่มเติม", Expected: "ชื่อพื้นที่ตามโปรไฟล์", Actual: phrase, Status: "unverifiable"})
		}
	}
	r.Status = "match"
	for _, f := range r.Findings {
		if f.Status == "mismatch" {
			r.Status = "mismatch"
			break
		}
		if f.Status == "unverifiable" {
			r.Status = "unverifiable"
		}
	}
	return r
}
func uniqueCaptures(matches [][]string) string {
	values := []string{}
	seen := map[string]bool{}
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			values = append(values, m[1])
		}
	}
	return strings.Join(values, " / ")
}
func extractHouse(s string, recipients ...string) string {
	matches := [][]string{}
	for _, pattern := range []string{`(?:บ้านเลขที่|เลขที่)\s*(\d+(?:[/.\-]\d+)*[A-Za-z]?)`, `^\s*(\d+(?:[/.\-]\d+)*[A-Za-z]?)\b`} {
		matches = append(matches, regexp.MustCompile(pattern).FindAllStringSubmatch(s, -1)...)
	}
	// Lazada can put area/postcode first, then a separate "368 หมู่10"
	// address segment. Require a segment boundary AND the following moo label;
	// do not search for the expected number or reuse a soi/postcode as a house.
	matches = append(matches, regexp.MustCompile(`(?:^|[,;·\n])\s*(\d+(?:[/.\-]\d+)*[A-Za-z]?)\s*หมู่(?:ที่)?\s*\d`).FindAllStringSubmatch(s, -1)...)
	// Some marketplace emails repeat the recipient before an unlabelled house
	// number. Strip only a known name prefix, never scan arbitrary address numbers.
	if rest, ok := afterRecipientPrefix(s, recipients); ok {
		matches = append(matches, regexp.MustCompile(`^\s*(\d+(?:[/.\-]\d+)*[A-Za-z]?)\b`).FindAllStringSubmatch(rest, -1)...)
	}
	return uniqueCaptures(matches)
}
func afterRecipientPrefix(address string, recipients []string) (string, bool) {
	longest := 0
	for _, recipient := range recipients {
		wanted := []rune(norm(recipient))
		if len(wanted) == 0 {
			continue
		}
		position := 0
		for index, r := range address {
			if unicode.IsSpace(r) || unicode.IsPunct(r) {
				continue
			}
			if unicode.ToLower(r) != wanted[position] {
				break
			}
			position++
			if position == len(wanted) {
				end := index + utf8.RuneLen(r)
				// A name ending in digits must not consume the prefix of a larger number.
				if end < len(address) {
					next, _ := utf8.DecodeRuneInString(address[end:])
					if unicode.IsLetter(next) || unicode.IsDigit(next) {
						break
					}
				}
				if end > longest {
					longest = end
				}
				break
			}
		}
	}
	if longest == 0 {
		return "", false
	}
	return strings.TrimLeft(address[longest:], " \t\r\n),，.:：]"), true
}
func area(s, label string, allowed []string) (string, bool) {
	// Insert boundaries before Thai administrative labels even when joined together.
	s = regexp.MustCompile(`(ตำบล|อำเภอ|จังหวัด)`).ReplaceAllString(s, " $1")
	matches := regexp.MustCompile(label+`\s*([^\s,/·]+)`).FindAllStringSubmatch(s, -1)
	if len(matches) > 0 {
		values := []string{}
		ok := true
		for _, m := range matches {
			values = append(values, m[1])
			found := false
			for _, a := range allowed {
				if norm(a) == norm(m[1]) {
					found = true
				}
			}
			if !found {
				ok = false
			}
		}
		return strings.Join(values, " / "), ok
	}
	// Unlabelled bilingual forms must contain a complete configured place name.
	for _, a := range allowed {
		if strings.TrimSpace(a) == "" {
			continue
		}
		re := regexp.MustCompile(`(?i)(?:^|[\s,/·:;\-])` + regexp.QuoteMeta(a) + `(?:$|[\s,/·:;\-])`)
		if re.MatchString(s) {
			return a, true
		}
	}
	return "", false
}
