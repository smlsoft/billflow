package repository

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"billflow/internal/models"
	"golang.org/x/net/html"
)

var (
	lazadaQtyLabel  = regexp.MustCompile(`จำนวน\s*:`)
	lazadaCardQty   = regexp.MustCompile(`จำนวน\s*:\s*(-?[0-9]+(?:\.[0-9]+)?)`)
	lazadaCardPrice = regexp.MustCompile(`(?i)(?:THB|฿)\s*(-?[0-9][0-9,]*(?:\.[0-9]+)?)`)
	lazadaCardSKU   = regexp.MustCompile(`(?i)-s([0-9]+)\.html`)
)

// ParseLazadaEmailItems preserves source card occurrences, including identical
// titles/SKUs. A parsed set is usable only if every quantity block was read and
// the unmodified unit prices reconcile to the email's goods subtotal.
// No quantity blocks means an unsupported template; callers may retain AI rows
// for review but must not manufacture prices from the order subtotal.
func ParseLazadaEmailItems(bodyHTML string, summary LazadaAmountSummary) ([]models.BillItem, error) {
	if len(bodyHTML) > 5*1024*1024 {
		return nil, fmt.Errorf("Lazada HTML exceeds 5 MiB source parsing limit")
	}
	root, err := html.Parse(strings.NewReader(bodyHTML))
	if err != nil {
		return nil, err
	}
	texts := make(map[*html.Node]string)
	cachedBytes := 0
	var contentErr error
	var content func(*html.Node) string
	content = func(n *html.Node) string {
		if contentErr != nil {
			return ""
		}
		if n.Type == html.ElementNode && (n.Data == "style" || n.Data == "script") {
			return ""
		}
		if n.Type == html.TextNode {
			return n.Data
		}
		var parts []string
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			parts = append(parts, content(c))
		}
		text := strings.Join(parts, "\n")
		cachedBytes += len(text)
		if cachedBytes > 32*1024*1024 {
			contentErr = fmt.Errorf("Lazada HTML nesting exceeds source parsing text budget")
			return ""
		}
		texts[n] = text
		return text
	}
	allText := content(root)
	if contentErr != nil {
		return nil, contentErr
	}
	expected := len(lazadaQtyLabel.FindAllStringIndex(allText, -1))
	if expected == 0 {
		return nil, nil
	}
	var items []models.BillItem
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		text := texts[n]
		if n.Type == html.ElementNode && len(lazadaQtyLabel.FindAllStringIndex(text, -1)) == 1 {
			if item, ok := lazadaSourceCard(n, text); ok {
				item.SourceLineNo = len(items) + 1
				items = append(items, item)
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	if len(items) != expected {
		return nil, fmt.Errorf("Lazada source items incomplete: parsed=%d quantity_blocks=%d", len(items), expected)
	}
	if !summary.HasGoodsTotalAmount {
		return nil, fmt.Errorf("Lazada source items have no verified goods subtotal")
	}
	gross := 0.0
	for _, item := range items {
		gross += item.Qty * *item.Price
	}
	if math.Abs(roundMoney(gross)-summary.GoodsTotalAmount) > 0.01 {
		return nil, fmt.Errorf("Lazada source item total %.2f differs from goods subtotal %.2f", gross, summary.GoodsTotalAmount)
	}
	return items, nil
}

func lazadaSourceCard(node *html.Node, text string) (models.BillItem, bool) {
	var item models.BillItem
	qty := lazadaCardQty.FindAllStringSubmatch(text, -1)
	prices := lazadaCardPrice.FindAllStringSubmatch(text, -1)
	if len(qty) != 1 || len(prices) != 1 {
		return item, false
	}
	var qtyErr error
	item.Qty, qtyErr = strconv.ParseFloat(qty[0][1], 64)
	price, err := strconv.ParseFloat(strings.ReplaceAll(prices[0][1], ",", ""), 64)
	if qtyErr != nil || err != nil || item.Qty <= 0 || price < 0 {
		return item, false
	}
	item.Price = &price
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "img" && item.SourceImageURL == "" {
				for _, attr := range n.Attr {
					if attr.Key == "src" && lazadaSourceImage(attr.Val) {
						item.SourceImageURL = attr.Val
					}
				}
			}
			if n.Data == "a" {
				name := lazadaNodeText(n)
				if len([]rune(name)) >= 4 && !lazadaCardPrice.MatchString(name) && !lazadaQtyLabel.MatchString(name) && len([]rune(name)) > len([]rune(item.RawName)) {
					item.RawName = name
				}
				for _, attr := range n.Attr {
					if attr.Key != "href" {
						continue
					}
					value := attr.Val
					for i := 0; i < 3; i++ {
						if m := lazadaCardSKU.FindStringSubmatch(value); len(m) > 1 {
							item.SourceSKU = m[1]
							break
						}
						decoded, err := url.QueryUnescape(value)
						if err != nil || decoded == value {
							break
						}
						value = decoded
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(node)
	if item.RawName == "" {
		// Some templates use an unlinked title. Only consider text before
		// the price, within this card, never seller or order summary blocks.
		prefix := text[:lazadaCardPrice.FindStringIndex(text)[0]]
		for _, line := range strings.Split(prefix, "\n") {
			line = strings.Join(strings.Fields(line), " ")
			if len([]rune(line)) > len([]rune(item.RawName)) {
				item.RawName = line
			}
		}
	}
	return item, item.SourceImageURL != "" && len([]rune(item.RawName)) >= 4
}

func lazadaNodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(lazadaNodeText(c))
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func lazadaSourceImage(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	if strings.EqualFold(u.Host, "filebroker-cdn.lazada.co.th") {
		// Some Lazada confirmations use their filebroker CDN for product images.
		return u.Scheme == "https" && u.User == nil &&
			strings.HasPrefix(u.Path, "/kf/") && len(u.Path) > len("/kf/")
	}
	return strings.HasSuffix(strings.ToLower(u.Hostname()), ".slatic.net") && strings.Contains(u.Path, "/p/")
}
