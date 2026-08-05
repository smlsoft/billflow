// Package emailpreview prepares source marketplace email HTML for BillFlow's
// on-screen preview and immutable PDF snapshots.
package emailpreview

import "strings"

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
