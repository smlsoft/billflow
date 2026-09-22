// Package emailpreview prepares source marketplace email HTML for BillFlow's
// on-screen preview and immutable PDF snapshots.
package emailpreview

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

const previewResetCSS = `<style id="billflow-email-preview-reset">*{box-sizing:border-box}html,body{margin:0!important;padding:0!important;background:#fff!important}img{display:block;max-width:100%}table{margin:0!important}</style>`

// PrepareHTML is the single visual contract shared by the BillFlow email
// dialog and exports. It leaves source content intact, apart from the existing
// payment-total highlighting and layout reset applied by the dialog.
func PrepareHTML(input string) string {
	html := decorateMarketplaceEmailPreviewHTML(input)
	if strings.Contains(html, `id="billflow-email-preview-reset"`) {
		return html
	}
	if idx := indexCaseInsensitive(html, "<head"); idx >= 0 {
		if end := strings.Index(html[idx:], ">"); end >= 0 {
			at := idx + end + 1
			return html[:at] + previewResetCSS + html[at:]
		}
	}
	if idx := indexCaseInsensitive(html, "<body"); idx >= 0 {
		return html[:idx] + "<head>" + previewResetCSS + "</head>" + html[idx:]
	}
	return "<!doctype html><html><head><meta charset=\"utf-8\">" + previewResetCSS + "</head><body>" + html + "</body></html>"
}

// PrintOrder is the BillFlow context that is intentionally added to an
// immutable marketplace-email PDF. The source email remains the main content;
// these fields make the saved copy traceable to its SML purchase order and
// card-payment reference.
type PrintOrder struct {
	OrderID       string
	SMLDocNo      string
	PaymentMethod string
}

type PrintContext struct {
	SourceChannel string
	Orders        []PrintOrder
}

// PreparePrintableHTML applies the same base preview treatment as PrepareHTML,
// then adds the minimal BillFlow print context that operators rely on: POL next
// to each marketplace order and a card-payment stamp when an order uses TTxxxx.
func PreparePrintableHTML(input string, context PrintContext) string {
	prepared := PrepareHTML(input)
	orders := normalizedPrintOrders(context.Orders)
	if len(orders) == 0 {
		return prepared
	}

	doc, err := html.Parse(strings.NewReader(prepared))
	if err != nil {
		return prepared
	}
	body := findElement(doc, "body")
	if body == nil {
		return prepared
	}

	annotated := map[string]bool{}
	forEachElement(body, "tr", func(row *html.Node) {
		if !isOrderDetailRow(row) {
			return
		}
		for _, order := range orders {
			if !annotated[orderKey(order.OrderID)] && insertOrderDocLabel(row, order) {
				annotated[orderKey(order.OrderID)] = true
			}
		}
	})
	for _, order := range orders {
		if !annotated[orderKey(order.OrderID)] && insertOrderDocLabel(body, order) {
			annotated[orderKey(order.OrderID)] = true
		}
	}

	if paymentMethods := cardPaymentMethods(orders); len(paymentMethods) > 0 {
		body.AppendChild(createPaymentStamp(context.SourceChannel, paymentMethods))
	}

	var output bytes.Buffer
	if err := html.Render(&output, doc); err != nil {
		return prepared
	}
	return output.String()
}

func normalizedPrintOrders(input []PrintOrder) []PrintOrder {
	seen := map[string]bool{}
	orders := make([]PrintOrder, 0, len(input))
	for _, order := range input {
		order.OrderID = strings.TrimSpace(strings.TrimLeft(order.OrderID, "#"))
		order.SMLDocNo = strings.TrimSpace(order.SMLDocNo)
		order.PaymentMethod = strings.TrimSpace(order.PaymentMethod)
		key := orderKey(order.OrderID)
		if key == "" || order.SMLDocNo == "" || seen[key] {
			continue
		}
		seen[key] = true
		orders = append(orders, order)
	}
	return orders
}

func orderKey(orderID string) string {
	return strings.ToUpper(strings.TrimSpace(strings.TrimLeft(orderID, "#")))
}

func cardPaymentMethods(orders []PrintOrder) []string {
	seen := map[string]bool{}
	methods := []string{}
	for _, order := range orders {
		payment := strings.ToUpper(strings.TrimSpace(order.PaymentMethod))
		if !strings.HasPrefix(payment, "TT") || seen[payment] {
			continue
		}
		seen[payment] = true
		methods = append(methods, payment)
	}
	return methods
}

func findElement(root *html.Node, tag string) *html.Node {
	if root.Type == html.ElementNode && strings.EqualFold(root.Data, tag) {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func forEachElement(root *html.Node, tag string, fn func(*html.Node)) {
	if root.Type == html.ElementNode && strings.EqualFold(root.Data, tag) {
		fn(root)
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		forEachElement(child, tag, fn)
	}
}

func isOrderDetailRow(row *html.Node) bool {
	text := strings.Join(strings.Fields(nodeText(row)), " ")
	return strings.Contains(text, "หมายเลขคำสั่งซื้อ") || strings.Contains(text, "เลขคำสั่งซื้อ")
}

func nodeText(root *html.Node) string {
	var out strings.Builder
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.TextNode {
			out.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return out.String()
}

func insertOrderDocLabel(root *html.Node, order PrintOrder) bool {
	variants := orderIDVariants(order.OrderID)
	if len(variants) == 0 {
		return false
	}
	var textNodes []*html.Node
	collectTextNodes(root, &textNodes)
	for _, textNode := range textNodes {
		if skipPrintNode(textNode) {
			continue
		}
		index, length := findOrderID(textNode.Data, variants)
		if index < 0 {
			continue
		}
		insertLabelAfterText(textNode, index, length, order.SMLDocNo)
		return true
	}
	return false
}

func collectTextNodes(root *html.Node, out *[]*html.Node) {
	if root.Type == html.TextNode {
		*out = append(*out, root)
		return
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		collectTextNodes(child, out)
	}
}

func skipPrintNode(node *html.Node) bool {
	for current := node.Parent; current != nil; current = current.Parent {
		if current.Type != html.ElementNode {
			continue
		}
		switch strings.ToLower(current.Data) {
		case "button", "script", "style", "noscript":
			return true
		}
		for _, attr := range current.Attr {
			if attr.Key == "data-billflow-order-label" || attr.Key == "data-billflow-print" || attr.Key == "hidden" {
				return true
			}
		}
	}
	return false
}

func orderIDVariants(orderID string) []string {
	clean := strings.TrimSpace(strings.TrimLeft(orderID, "#"))
	if clean == "" {
		return nil
	}
	return []string{"#" + clean, clean}
}

func findOrderID(text string, variants []string) (int, int) {
	upper := strings.ToUpper(text)
	for _, variant := range variants {
		if idx := strings.Index(upper, strings.ToUpper(variant)); idx >= 0 {
			return idx, len(variant)
		}
	}
	return -1, 0
}

func insertLabelAfterText(textNode *html.Node, index, length int, docNo string) {
	if anchor := closestElement(textNode, "a"); anchor != nil && anchor.Parent != nil {
		anchor.Parent.InsertBefore(createOrderDocLabel(docNo), anchor.NextSibling)
		return
	}
	parent := textNode.Parent
	if parent == nil {
		return
	}
	next := textNode.NextSibling
	after := &html.Node{Type: html.TextNode, Data: textNode.Data[index+length:]}
	textNode.Data = textNode.Data[:index+length]
	parent.InsertBefore(createOrderDocLabel(docNo), next)
	parent.InsertBefore(after, next)
}

func closestElement(node *html.Node, tag string) *html.Node {
	for current := node.Parent; current != nil; current = current.Parent {
		if current.Type == html.ElementNode && strings.EqualFold(current.Data, tag) {
			return current
		}
	}
	return nil
}

func createOrderDocLabel(docNo string) *html.Node {
	label := &html.Node{Type: html.ElementNode, Data: "span", Attr: []html.Attribute{
		{Key: "data-billflow-order-label", Val: "true"},
		{Key: "style", Val: "display:inline-block;margin-left:6px;font-family:monospace;font-size:11px;font-weight:bold;color:#059669;background:#ecfdf5;border:1px solid #a7f3d0;border-radius:3px;padding:0 4px;vertical-align:baseline;print-color-adjust:exact;-webkit-print-color-adjust:exact"},
	}}
	label.AppendChild(&html.Node{Type: html.TextNode, Data: "→ " + docNo})
	return label
}

func createPaymentStamp(sourceChannel string, paymentMethods []string) *html.Node {
	position := "top:8mm"
	if strings.EqualFold(strings.TrimSpace(sourceChannel), "lazada") {
		position = "bottom:8mm"
	}
	stamp := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{
		{Key: "data-billflow-print", Val: "true"},
		{Key: "style", Val: "position:fixed;right:10mm;" + position + ";z-index:2147483647;display:inline-flex;flex-direction:column;gap:3px;max-width:150px;padding:5px 8px;background:#fff;border:1.5px solid #111827;border-radius:6px;color:#111827;font-family:system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;line-height:1.15;text-align:center;box-shadow:0 1px 6px rgba(0,0,0,.14);print-color-adjust:exact;-webkit-print-color-adjust:exact"},
	}}
	for _, method := range paymentMethods {
		block := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "style", Val: "display:flex;flex-direction:column;gap:1px;white-space:nowrap"}}}
		label := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "style", Val: "font-size:10.5px;font-weight:700"}}}
		label.AppendChild(&html.Node{Type: html.TextNode, Data: "จ่ายบัตรเครดิต"})
		value := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "style", Val: "font-size:12px;font-weight:800"}}}
		value.AppendChild(&html.Node{Type: html.TextNode, Data: method})
		block.AppendChild(label)
		block.AppendChild(value)
		stamp.AppendChild(block)
	}
	return stamp
}

func decorateMarketplaceEmailPreviewHTML(input string) string {
	html := input
	for _, target := range []struct {
		label  string
		bg     string
		border string
	}{
		{label: "ยอดที่ต้องชำระทั้งหมด", bg: "#fef3c7", border: "#facc15"},
		{label: "จำนวนเงินที่จ่าย", bg: "#dcfce7", border: "#86efac"},
		{label: "ยอดรวมทั้งหมด(รวม VAT)", bg: "#fef3c7", border: "#facc15"},
		{label: "ยอดรวมทั้งหมด (รวม VAT)", bg: "#fef3c7", border: "#facc15"},
	} {
		html = decorateHTMLTableRowsByLabel(html, target.label, target.bg, target.border)
	}
	return html
}

func decorateHTMLTableRowsByLabel(input, label, bg, border string) string {
	if strings.TrimSpace(input) == "" || strings.TrimSpace(label) == "" {
		return input
	}
	out := input
	searchFrom := 0
	for {
		idx := strings.Index(out[searchFrom:], label)
		if idx < 0 {
			return out
		}
		idx += searchFrom
		rowStart := lastIndexCaseInsensitive(out[:idx], "<tr")
		rowEndRel := indexCaseInsensitive(out[idx:], "</tr>")
		if rowStart < 0 || rowEndRel < 0 {
			searchFrom = idx + len(label)
			continue
		}
		rowEnd := idx + rowEndRel + len("</tr>")
		if rowEnd <= rowStart || rowEnd-rowStart > 6000 {
			searchFrom = idx + len(label)
			continue
		}
		row := out[rowStart:rowEnd]
		if strings.Contains(row, `data-billflow-print-highlight="true"`) {
			searchFrom = rowEnd
			continue
		}
		decorated := decorateHTMLRowFragment(row, bg, border)
		out = out[:rowStart] + decorated + out[rowEnd:]
		searchFrom = rowStart + len(decorated)
	}
}

func decorateHTMLRowFragment(row, bg, border string) string {
	rowStyle := printHighlightStyle(bg, border)
	out := styleFirstHTMLTag(row, "<tr", rowStyle, `data-billflow-print-highlight="true"`)
	out = styleAllHTMLTags(out, "<td", rowStyle, `data-billflow-print-highlight-cell="true"`)
	out = styleAllHTMLTags(out, "<th", rowStyle, `data-billflow-print-highlight-cell="true"`)
	return out
}

func printHighlightStyle(bg, border string) string {
	return "background:" + bg + " !important;" +
		"background-color:" + bg + " !important;" +
		"box-shadow:inset 0 0 0 9999px " + bg + " !important;" +
		"border-top:1px solid " + border + " !important;" +
		"border-bottom:1px solid " + border + " !important;" +
		"-webkit-print-color-adjust:exact !important;" +
		"print-color-adjust:exact !important;"
}

func styleFirstHTMLTag(input, tagPrefix, style, dataAttr string) string {
	idx := indexCaseInsensitive(input, tagPrefix)
	if idx < 0 {
		return input
	}
	endRel := strings.Index(input[idx:], ">")
	if endRel < 0 {
		return input
	}
	end := idx + endRel + 1
	return input[:idx] + addStyleToOpeningHTMLTag(input[idx:end], style, dataAttr) + input[end:]
}

func styleAllHTMLTags(input, tagPrefix, style, dataAttr string) string {
	lowerPrefix := strings.ToLower(tagPrefix)
	var out strings.Builder
	pos := 0
	lower := strings.ToLower(input)
	for {
		idxRel := strings.Index(lower[pos:], lowerPrefix)
		if idxRel < 0 {
			out.WriteString(input[pos:])
			return out.String()
		}
		idx := pos + idxRel
		endRel := strings.Index(input[idx:], ">")
		if endRel < 0 {
			out.WriteString(input[pos:])
			return out.String()
		}
		end := idx + endRel + 1
		out.WriteString(input[pos:idx])
		out.WriteString(addStyleToOpeningHTMLTag(input[idx:end], style, dataAttr))
		pos = end
	}
}

func addStyleToOpeningHTMLTag(opening, style, dataAttr string) string {
	if strings.Contains(opening, dataAttr) {
		return opening
	}
	tag := opening
	insertAt := strings.LastIndex(tag, ">")
	if insertAt < 0 {
		return opening
	}
	tag = tag[:insertAt] + " " + dataAttr + tag[insertAt:]

	lower := strings.ToLower(tag)
	styleIdx := strings.Index(lower, "style=")
	if styleIdx < 0 {
		insertAt = strings.LastIndex(tag, ">")
		return tag[:insertAt] + ` style="` + style + `"` + tag[insertAt:]
	}
	valueStart := styleIdx + len("style=")
	for valueStart < len(tag) && (tag[valueStart] == ' ' || tag[valueStart] == '\t' || tag[valueStart] == '\n') {
		valueStart++
	}
	if valueStart >= len(tag) || (tag[valueStart] != '"' && tag[valueStart] != '\'') {
		insertAt = strings.LastIndex(tag, ">")
		return tag[:insertAt] + ` style="` + style + `"` + tag[insertAt:]
	}
	return tag[:valueStart+1] + style + tag[valueStart+1:]
}

func indexCaseInsensitive(input, needle string) int {
	return strings.Index(strings.ToLower(input), strings.ToLower(needle))
}

func lastIndexCaseInsensitive(input, needle string) int {
	return strings.LastIndex(strings.ToLower(input), strings.ToLower(needle))
}
