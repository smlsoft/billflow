package repository

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"billflow/internal/models"
)

var (
	shopeeOrderDatePattern = regexp.MustCompile(`วันที่สั่งซื้อ\s*[:：]\s*([^\r\n<]+)`)
	shopeeSellerPattern    = regexp.MustCompile(`ผู้ขาย\s*[:：]\s*([^\r\n<]+)`)
	htmlTagPattern         = regexp.MustCompile(`<[^>]+>`)
	spacePattern           = regexp.MustCompile(`[ \t]+`)
)

func enrichShopeeBillRawData(b *models.Bill, itemCount int, stripBody bool) {
	if b == nil || b.RawData == nil || b.Source != "shopee_shipped" {
		return
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(b.RawData, &raw); err != nil {
		return
	}

	orderID := strings.TrimSpace(stringField(raw, "order_id"))
	if orderID == "" {
		orderID = strings.TrimSpace(stringField(raw, "shopee_order_id"))
	}
	body := stringField(raw, "body_text")
	if body == "" {
		body = htmlToSummaryText(stringField(raw, "body_html"))
	}
	block := shopeeOrderBlock(body, orderID)
	if block == "" {
		block = body
	}

	setIfEmpty(raw, "order_datetime", firstSubmatch(shopeeOrderDatePattern, block))
	setIfEmpty(raw, "seller_name", firstSubmatch(shopeeSellerPattern, block))
	setMoneyIfMissing(raw, "goods_total_amount", "ยอดรวมค่าสินค้า", block)
	setMoneyIfMissing(raw, "shipping_amount", "ค่าจัดส่งสินค้า", block)
	setMoneyIfMissing(raw, "paid_total_amount", "ยอดที่ต้องชำระทั้งหมด", block)
	if itemCount > 0 {
		raw["item_count"] = itemCount
	}

	if stripBody {
		delete(raw, "body_text")
		delete(raw, "body_html")
	}

	if out, err := json.Marshal(raw); err == nil {
		b.RawData = out
	}
}

func shopeeOrderBlock(body, orderID string) string {
	if body == "" || orderID == "" {
		return ""
	}
	idx := -1
	searchFrom := 0
	for {
		found := strings.Index(body[searchFrom:], orderID)
		if found < 0 {
			break
		}
		candidate := searchFrom + found
		if strings.LastIndex(body[:candidate], "หมายเลขคำสั่งซื้อ") >= 0 {
			idx = candidate
			break
		}
		searchFrom = candidate + len(orderID)
	}
	if idx < 0 {
		idx = strings.Index(body, orderID)
	}
	if idx < 0 {
		return ""
	}
	start := strings.LastIndex(body[:idx], "หมายเลขคำสั่งซื้อ")
	if start < 0 {
		start = idx
	}
	after := body[idx+len(orderID):]
	endRel := strings.Index(after, "หมายเลขคำสั่งซื้อ")
	if endRel < 0 {
		return body[start:]
	}
	return body[start : idx+len(orderID)+endRel]
}

func stringField(raw map[string]interface{}, key string) string {
	v, ok := raw[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func setIfEmpty(raw map[string]interface{}, key, value string) {
	if value == "" || stringField(raw, key) != "" {
		return
	}
	raw[key] = strings.TrimSpace(value)
}

func firstSubmatch(re *regexp.Regexp, text string) string {
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func setMoneyIfMissing(raw map[string]interface{}, key, label, text string) {
	if _, ok := raw[key]; ok {
		return
	}
	re := regexp.MustCompile(regexp.QuoteMeta(label) + `\s*[:：]\s*฿\s*([\d,]+(?:\.\d+)?)`)
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return
	}
	clean := strings.ReplaceAll(m[1], ",", "")
	if v, err := strconv.ParseFloat(clean, 64); err == nil {
		raw[key] = v
	}
}

func htmlToSummaryText(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "</tr>", "\n")
	s = strings.ReplaceAll(s, "</td>", " ")
	s = strings.ReplaceAll(s, "</div>", "\n")
	s = htmlTagPattern.ReplaceAllString(s, "")
	replacer := strings.NewReplacer(
		"&nbsp;", " ",
		"&#160;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
	)
	s = replacer.Replace(s)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(spacePattern.ReplaceAllString(line, " "))
	}
	return strings.Join(lines, "\n")
}
