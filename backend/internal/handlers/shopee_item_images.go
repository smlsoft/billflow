package handlers

import (
	htmlstd "html"
	"math"
	"regexp"
	"strconv"
	"strings"

	"billflow/internal/services/ai"
)

const (
	ShopeeItemImageReasonExisting       = "existing"
	ShopeeItemImageReasonBlock          = "block"
	ShopeeItemImageReasonNearest        = "nearest"
	ShopeeItemImageReasonSingleFallback = "single_fallback"
	ShopeeItemImageReasonNoMatch        = "no_match"
	ShopeeItemImageReasonAmbiguous      = "ambiguous"
)

type ShopeeItemImageDecision struct {
	ImageURL string
	Reason   string
}

type shopeeImageRef struct {
	url   string
	start int
	end   int
}

type shopeeProductImageBlock struct {
	refIndex       int
	normalizedText string
	qty            *float64
	price          *float64
}

var (
	shopeeHTMLTagPattern    = regexp.MustCompile(`(?s)<[^>]+>`)
	shopeeWhitespacePattern = regexp.MustCompile(`\s+`)
	shopeeQtyPattern        = regexp.MustCompile(`(?i)(?:^|\s)(?:จำนวน|qty|quantity)\s*[:：]?\s*([0-9]+(?:\.[0-9]+)?)`)
)

func MatchShopeeItemImages(items []ai.ExtractedItem, bodyHTML, orderID string) ([]ai.ExtractedItem, []ShopeeItemImageDecision) {
	out := make([]ai.ExtractedItem, len(items))
	copy(out, items)

	decisions := make([]ShopeeItemImageDecision, len(out))
	for i := range decisions {
		decisions[i].Reason = ShopeeItemImageReasonNoMatch
	}
	if len(out) == 0 {
		return out, decisions
	}
	for i := range out {
		if strings.TrimSpace(out[i].ImageURL) != "" {
			decisions[i] = ShopeeItemImageDecision{
				ImageURL: out[i].ImageURL,
				Reason:   ShopeeItemImageReasonExisting,
			}
		}
	}

	scopedHTML := scopeShopeeItemImageHTML(bodyHTML, orderID)
	if strings.TrimSpace(scopedHTML) == "" {
		return out, decisions
	}
	refs := shopeeProductImageRefs(scopedHTML)
	if len(refs) == 0 {
		return out, decisions
	}
	blocks := shopeeProductImageBlocks(scopedHTML, refs)
	nameCounts := shopeeNormalizedItemNameCounts(out)

	used := map[int]bool{}
	for _, item := range out {
		existingURL := strings.TrimSpace(item.ImageURL)
		if existingURL == "" {
			continue
		}
		for i, ref := range refs {
			if sameURL(existingURL, ref.url) {
				used[i] = true
				break
			}
		}
	}

	for i := range out {
		if strings.TrimSpace(out[i].ImageURL) != "" {
			continue
		}
		if blockIdx := matchShopeeProductImageBlockIndex(blocks, out[i], used); blockIdx >= 0 {
			refIdx := blocks[blockIdx].refIndex
			out[i].ImageURL = refs[refIdx].url
			decisions[i] = ShopeeItemImageDecision{
				ImageURL: refs[refIdx].url,
				Reason:   ShopeeItemImageReasonBlock,
			}
			used[refIdx] = true
			continue
		}
		if nameCounts[normalizeShopeeMatchText(out[i].RawName)] > 1 {
			decisions[i].Reason = ShopeeItemImageReasonAmbiguous
			continue
		}
		pos := findShopeeItemHTMLPosition(scopedHTML, out[i].RawName)
		if pos < 0 {
			if len(out) == 1 && len(refs) == 1 && !used[0] {
				out[i].ImageURL = refs[0].url
				decisions[i] = ShopeeItemImageDecision{
					ImageURL: refs[0].url,
					Reason:   ShopeeItemImageReasonSingleFallback,
				}
				used[0] = true
				continue
			}
			if len(refs) > 1 {
				decisions[i].Reason = ShopeeItemImageReasonAmbiguous
			}
			continue
		}
		idx := nearestShopeeProductImageRefIndex(refs, pos, used)
		if idx < 0 {
			continue
		}
		out[i].ImageURL = refs[idx].url
		decisions[i] = ShopeeItemImageDecision{
			ImageURL: refs[idx].url,
			Reason:   ShopeeItemImageReasonNearest,
		}
		used[idx] = true
	}

	return out, decisions
}

func shopeeProductImageBlocks(bodyHTML string, refs []shopeeImageRef) []shopeeProductImageBlock {
	blocks := make([]shopeeProductImageBlock, 0, len(refs))
	for i, ref := range refs {
		start := ref.start
		end := len(bodyHTML)
		if i+1 < len(refs) {
			end = refs[i+1].start
		}
		if start < 0 || end <= start || end > len(bodyHTML) {
			continue
		}
		text := normalizeShopeeHTMLText(bodyHTML[start:end])
		blocks = append(blocks, shopeeProductImageBlock{
			refIndex:       i,
			normalizedText: normalizeShopeeMatchText(text),
			qty:            parseShopeeBlockQty(text),
			price:          parseShopeeBlockPrice(text),
		})
	}
	return blocks
}

func matchShopeeProductImageBlockIndex(blocks []shopeeProductImageBlock, item ai.ExtractedItem, used map[int]bool) int {
	hasQty := item.Qty > 0
	hasPrice := item.Price != nil
	if !hasQty && !hasPrice {
		return -1
	}

	matches := []int{}
	for i, block := range blocks {
		if used[block.refIndex] {
			continue
		}
		if !shopeeBlockContainsItemName(block.normalizedText, item.RawName) {
			continue
		}
		if hasQty {
			if block.qty == nil || !sameShopeeNumber(*block.qty, item.Qty) {
				continue
			}
		}
		if hasPrice {
			if block.price == nil || !sameShopeeNumber(*block.price, *item.Price) {
				continue
			}
		}
		matches = append(matches, i)
	}
	if len(matches) != 1 {
		return -1
	}
	return matches[0]
}

func shopeeBlockContainsItemName(normalizedBlockText, rawName string) bool {
	rawName = strings.TrimSpace(rawName)
	if rawName == "" || normalizedBlockText == "" {
		return false
	}
	candidates := []string{rawName}
	if short := firstRunes(rawName, 90); short != rawName {
		candidates = append(candidates, short)
	}
	if short := firstRunes(rawName, 45); short != rawName {
		candidates = append(candidates, short)
	}
	for _, candidate := range candidates {
		normalized := normalizeShopeeMatchText(candidate)
		if normalized != "" && strings.Contains(normalizedBlockText, normalized) {
			return true
		}
	}
	return false
}

func shopeeNormalizedItemNameCounts(items []ai.ExtractedItem) map[string]int {
	counts := map[string]int{}
	for _, item := range items {
		name := normalizeShopeeMatchText(item.RawName)
		if name != "" {
			counts[name]++
		}
	}
	return counts
}

func normalizeShopeeHTMLText(raw string) string {
	raw = htmlstd.UnescapeString(raw)
	raw = shopeeHTMLTagPattern.ReplaceAllString(raw, " ")
	return strings.TrimSpace(shopeeWhitespacePattern.ReplaceAllString(raw, " "))
}

func normalizeShopeeMatchText(raw string) string {
	raw = htmlstd.UnescapeString(raw)
	raw = shopeeHTMLTagPattern.ReplaceAllString(raw, " ")
	raw = shopeeWhitespacePattern.ReplaceAllString(raw, " ")
	return strings.ToLower(strings.TrimSpace(raw))
}

func parseShopeeBlockQty(text string) *float64 {
	m := shopeeQtyPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return nil
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseShopeeBlockPrice(text string) *float64 {
	prices := extractShopeePrices(text)
	if len(prices) == 0 {
		return nil
	}
	return &prices[0]
}

func sameShopeeNumber(a, b float64) bool {
	return math.Abs(a-b) < 0.01
}

func scopeShopeeItemImageHTML(bodyHTML, orderID string) string {
	bodyHTML = strings.TrimSpace(bodyHTML)
	orderID = normalizeShopeeOrderID(orderID)
	if bodyHTML == "" || orderID == "" {
		return bodyHTML
	}
	matches := uniqueShopeeOrderIDMatches(bodyHTML)
	for i, m := range matches {
		if m.id != orderID {
			continue
		}
		start := m.start
		end := len(bodyHTML)
		if i+1 < len(matches) {
			end = matches[i+1].start
		}
		if start >= 0 && end > start && end <= len(bodyHTML) {
			return strings.TrimSpace(bodyHTML[start:end])
		}
	}
	return scopeShopeeHTMLToOrder(bodyHTML, orderID)
}

func shopeeProductImageRefs(bodyHTML string) []shopeeImageRef {
	matches := imgSrcPattern.FindAllStringSubmatchIndex(bodyHTML, -1)
	refs := make([]shopeeImageRef, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		if len(m) < 4 || m[2] < 0 || m[3] < 0 {
			continue
		}
		url := htmlstd.UnescapeString(strings.TrimSpace(bodyHTML[m[2]:m[3]]))
		if !isShopeeProductImageURL(url) {
			continue
		}
		key := strings.ToLower(url)
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, shopeeImageRef{url: url, start: m[0], end: m[1]})
	}
	return refs
}

func isShopeeProductImageURL(raw string) bool {
	u := strings.ToLower(strings.TrimSpace(htmlstd.UnescapeString(raw)))
	if u == "" || strings.HasPrefix(u, "data:") {
		return false
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return false
	}
	blocked := []string{
		"tracking.",
		"/tracking/",
		"/open/",
		"pixel",
		"logo",
		"icon",
		"facebook",
		"instagram",
		"banner",
		"spacer",
		"sprite",
	}
	for _, token := range blocked {
		if strings.Contains(u, token) {
			return false
		}
	}
	return strings.Contains(u, "cf.shopee.co.th/file/") ||
		strings.Contains(u, "f.shopee.co.th/file/") ||
		strings.Contains(u, "/file/th-")
}

func nearestShopeeProductImageRefIndex(refs []shopeeImageRef, itemPos int, used map[int]bool) int {
	const maxBeforeDistance = 6000
	const maxAfterDistance = 2500

	bestIdx := -1
	bestDistance := int(^uint(0) >> 1)
	for i, ref := range refs {
		if used[i] {
			continue
		}
		if ref.end <= itemPos {
			dist := itemPos - ref.end
			if dist <= maxBeforeDistance && dist < bestDistance {
				bestDistance = dist
				bestIdx = i
			}
			continue
		}
		dist := ref.start - itemPos
		if dist >= 0 && dist <= maxAfterDistance && dist < bestDistance {
			bestDistance = dist
			bestIdx = i
		}
	}
	return bestIdx
}

func findShopeeItemHTMLPosition(bodyHTML, rawName string) int {
	rawName = strings.Join(strings.Fields(strings.TrimSpace(rawName)), " ")
	if rawName == "" {
		return -1
	}
	candidates := []string{rawName}
	if short := firstRunes(rawName, 90); short != rawName {
		candidates = append(candidates, short)
	}
	if short := firstRunes(rawName, 45); short != rawName {
		candidates = append(candidates, short)
	}

	lowerHTML := strings.ToLower(bodyHTML)
	for _, candidate := range candidates {
		if idx := strings.Index(lowerHTML, strings.ToLower(candidate)); idx >= 0 {
			return idx
		}
		escaped := htmlstd.EscapeString(candidate)
		if escaped != candidate {
			if idx := strings.Index(lowerHTML, strings.ToLower(escaped)); idx >= 0 {
				return idx
			}
		}
	}
	return -1
}

func sameURL(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
