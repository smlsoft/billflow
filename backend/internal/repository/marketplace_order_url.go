package repository

import (
	"html"
	"net/url"
	"regexp"
	"strings"

	nethtml "golang.org/x/net/html"
)

var (
	rawURLPattern                  = regexp.MustCompile(`https?://[^\s"'<>]+`)
	shopeePurchaseOrderPathPattern = regexp.MustCompile(`(?i)(?:user(?:%2[fF]|/)purchase(?:%2[fF]|/)order(?:%2[fF]|/))(\d+)`)
	lazadaTradeOrderPattern        = regexp.MustCompile(`(?i)tradeOrderId(?:%3[dD]|=)(\d+)`)
)

// ExtractMarketplaceOrderURL extracts a safe marketplace order URL from the
// source email. It is deterministic and intentionally refuses to guess when the
// email does not clearly map a URL to the requested order.
func ExtractMarketplaceOrderURL(source, bodyText, bodyHTML, orderID string) string {
	switch source {
	case "shopee_shipped":
		return ExtractShopeeMarketplaceOrderURL(bodyText, bodyHTML, orderID)
	case "lazada_email":
		return ExtractLazadaMarketplaceOrderURL(bodyText, bodyHTML, orderID)
	default:
		return ""
	}
}

func ExtractShopeeMarketplaceOrderURL(bodyText, bodyHTML, orderID string) string {
	orderID = strings.TrimSpace(strings.TrimLeft(orderID, "#"))
	if orderID == "" {
		return ""
	}
	if orderURL := extractShopeeOrderAnchorURL(bodyHTML, orderID); orderURL != "" {
		return orderURL
	}
	body := marketplaceURLBody(bodyText, bodyHTML)
	if body == "" {
		return ""
	}

	upperBody := strings.ToUpper(body)
	var best string
	bestDistance := len(body) + 1
	for _, variant := range orderIDVariants(orderID) {
		search := strings.ToUpper(variant)
		offset := 0
		for {
			idx := strings.Index(upperBody[offset:], search)
			if idx < 0 {
				break
			}
			idx += offset
			windowStart := idx - 14000
			if windowStart < 0 {
				windowStart = 0
			}
			windowEnd := idx + 6000
			if windowEnd > len(body) {
				windowEnd = len(body)
			}
			window := body[windowStart:windowEnd]
			for _, candidate := range marketplaceURLCandidates(window) {
				canonical, ok := CanonicalShopeeMarketplaceOrderURL(candidate)
				if !ok {
					continue
				}
				localIdx := strings.Index(window, candidate)
				if localIdx < 0 {
					localIdx = 0
				}
				absoluteIdx := windowStart + localIdx
				distance := absoluteIdx - idx
				if distance < 0 {
					distance = -distance
				}
				if best == "" || distance < bestDistance {
					best = canonical
					bestDistance = distance
				}
			}
			offset = idx + len(search)
		}
	}
	return best
}

// extractShopeeOrderAnchorURL handles Shopee payment emails whose order links
// are wrapped by SendGrid click tracking. The tracked URL is accepted only when
// it is the href of an anchor whose visible text exactly matches the requested
// order ID; this prevents a multi-order email from linking a bill to a nearby
// order or to one of the many unrelated tracked links in the template.
func extractShopeeOrderAnchorURL(bodyHTML, orderID string) string {
	bodyHTML = strings.TrimSpace(bodyHTML)
	orderID = normalizeShopeeOrderAnchorText(orderID)
	if bodyHTML == "" || orderID == "" {
		return ""
	}

	tokenizer := nethtml.NewTokenizer(strings.NewReader(bodyHTML))
	var href string
	var text strings.Builder
	inAnchor := false
	for {
		switch tokenizer.Next() {
		case nethtml.ErrorToken:
			return ""
		case nethtml.StartTagToken:
			token := tokenizer.Token()
			if strings.EqualFold(token.Data, "a") {
				inAnchor = true
				href = ""
				text.Reset()
				for _, attr := range token.Attr {
					if strings.EqualFold(attr.Key, "href") {
						href = strings.TrimSpace(attr.Val)
						break
					}
				}
			}
		case nethtml.TextToken:
			if inAnchor {
				text.Write(tokenizer.Text())
			}
		case nethtml.EndTagToken:
			token := tokenizer.Token()
			if !inAnchor || !strings.EqualFold(token.Data, "a") {
				continue
			}
			if normalizeShopeeOrderAnchorText(text.String()) == orderID {
				if canonical, ok := CanonicalShopeeMarketplaceOrderURL(href); ok {
					return canonical
				}
				if tracked, ok := safeShopeeSendGridURL(href); ok {
					return tracked
				}
			}
			inAnchor = false
			href = ""
			text.Reset()
		}
	}
}

func normalizeShopeeOrderAnchorText(value string) string {
	value = strings.Join(strings.Fields(value), "")
	return strings.ToUpper(strings.TrimLeft(value, "#"))
}

func safeShopeeSendGridURL(raw string) (string, bool) {
	target := strings.TrimSpace(html.UnescapeString(raw))
	if target == "" || len(target) > 8192 {
		return "", false
	}
	parsed, err := url.Parse(target)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil || parsed.Port() != "" || parsed.Fragment != "" {
		return "", false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "ct.sendgrid.net" || !strings.HasSuffix(host, ".ct.sendgrid.net") || parsed.EscapedPath() != "/ls/click" {
		return "", false
	}
	if !strings.HasPrefix(strings.TrimSpace(parsed.Query().Get("upn")), "u001.") {
		return "", false
	}
	return target, true
}

func ExtractLazadaMarketplaceOrderURL(bodyText, bodyHTML, orderID string) string {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return ""
	}
	body := marketplaceURLBody(bodyText, bodyHTML)
	if body == "" {
		return ""
	}
	for _, candidate := range marketplaceURLCandidates(body) {
		canonical, ok := CanonicalLazadaMarketplaceOrderURL(candidate, orderID)
		if ok {
			return canonical
		}
	}
	return ""
}

func CanonicalShopeeMarketplaceOrderURL(raw string) (string, bool) {
	target := strings.TrimSpace(html.UnescapeString(raw))
	if target == "" {
		return "", false
	}
	if canonical, ok := canonicalShopeeFromAllowedURL(target); ok {
		return canonical, true
	}
	decoded := repeatedQueryUnescape(target)
	if decoded != target {
		return canonicalShopeeFromAllowedURL(decoded)
	}
	return "", false
}

func CanonicalLazadaMarketplaceOrderURL(raw, expectedOrderID string) (string, bool) {
	expectedOrderID = strings.TrimSpace(expectedOrderID)
	target := strings.TrimSpace(html.UnescapeString(raw))
	if target == "" {
		return "", false
	}
	if canonical, ok := canonicalLazadaFromAllowedURL(target, expectedOrderID); ok {
		return canonical, true
	}
	decoded := repeatedQueryUnescape(target)
	if decoded != target {
		return canonicalLazadaFromAllowedURL(decoded, expectedOrderID)
	}
	return "", false
}

func canonicalShopeeFromAllowedURL(target string) (string, bool) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Hostname() == "" || !isAllowedShopeeHost(parsed.Hostname()) {
		return "", false
	}
	if strings.EqualFold(parsed.Hostname(), "th.shp.ee") {
		if redir := strings.TrimSpace(parsed.Query().Get("redir")); redir != "" {
			return CanonicalShopeeMarketplaceOrderURL(redir)
		}
		return "", false
	}
	if !strings.EqualFold(parsed.Hostname(), "shopee.co.th") {
		return "", false
	}
	if match := shopeePurchaseOrderPathPattern.FindStringSubmatch(target); len(match) >= 2 {
		return "https://shopee.co.th/user/purchase/order/" + match[1] + "?type=6", true
	}
	return "", false
}

func canonicalLazadaFromAllowedURL(target, expectedOrderID string) (string, bool) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Hostname() == "" || !isAllowedLazadaHost(parsed.Hostname()) {
		return "", false
	}
	if strings.EqualFold(parsed.Hostname(), "c.lazada.co.th") {
		if redir := strings.TrimSpace(parsed.Query().Get("url")); redir != "" {
			return CanonicalLazadaMarketplaceOrderURL(redir, expectedOrderID)
		}
		return "", false
	}
	if tradeOrderID := strings.TrimSpace(parsed.Query().Get("tradeOrderId")); tradeOrderID != "" {
		return canonicalLazadaOrderDetailURL(tradeOrderID, expectedOrderID)
	}
	if match := lazadaTradeOrderPattern.FindStringSubmatch(target); len(match) >= 2 {
		return canonicalLazadaOrderDetailURL(match[1], expectedOrderID)
	}
	return "", false
}

func canonicalLazadaOrderDetailURL(orderID, expectedOrderID string) (string, bool) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return "", false
	}
	if expectedOrderID != "" && orderID != expectedOrderID {
		return "", false
	}
	return "https://my.lazada.co.th/customer/order/view/?tradeOrderId=" + url.QueryEscape(orderID), true
}

func marketplaceURLBody(bodyText, bodyHTML string) string {
	if strings.TrimSpace(bodyHTML) != "" {
		return html.UnescapeString(bodyHTML)
	}
	return html.UnescapeString(bodyText)
}

func marketplaceURLCandidates(body string) []string {
	matches := rawURLPattern.FindAllString(body, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		clean := strings.TrimRight(strings.TrimSpace(html.UnescapeString(match)), ".,);]")
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func repeatedQueryUnescape(value string) string {
	value = html.UnescapeString(strings.TrimSpace(value))
	for i := 0; i < 4; i++ {
		decoded, err := url.QueryUnescape(value)
		if err != nil || decoded == value {
			break
		}
		value = decoded
	}
	return value
}

func orderIDVariants(orderID string) []string {
	clean := strings.TrimSpace(strings.TrimLeft(orderID, "#"))
	if clean == "" {
		return nil
	}
	return []string{"#" + clean, clean}
}

func isAllowedShopeeHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "shopee.co.th" || host == "th.shp.ee"
}

func isAllowedLazadaHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "my.lazada.co.th" || host == "www.lazada.co.th" || host == "c.lazada.co.th"
}
