import { useState, useEffect } from 'react'
import api from '@/api/client'
import type { EmailPrintEvent } from '@/types'

export interface BillArtifact {
  id: string
  bill_id: string
  kind: string
  filename: string
  content_type?: string
  size_bytes: number
  sha256?: string
  source_meta?: Record<string, unknown>
  created_at: string
}

export interface ArtifactPrintOrderContext {
  orderId?: string
  smlDocNo?: string
  partyCode?: string
  partyName?: string
  paymentMethod?: string
}

export interface ArtifactPrintContext {
  orders: ArtifactPrintOrderContext[]
}

export interface ArtifactPrintBatchItem {
  billID: string
  artID: string
  filename: string
  printContext?: ArtifactPrintContext
}

export function useArtifacts(billId: string) {
  const [items, setItems] = useState<BillArtifact[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let alive = true
    api
      .get<{ data: BillArtifact[] }>(`/api/bills/${billId}/artifacts`)
      .then((r) => {
        if (alive) setItems(r.data.data ?? [])
      })
      .catch(() => {
        if (alive) setItems([])
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [billId])

  return { items, loading }
}

async function fetchArtifactBlob(
  billID: string,
  artID: string,
  filename: string,
  mode: 'preview' | 'download',
): Promise<Blob> {
  const res = await api.get(
    `/api/bills/${billID}/artifacts/${artID}/${mode}`,
    { responseType: 'blob' },
  )
  // Some browsers / axios builds drop the `charset=utf-8` parameter when
  // building the Blob from the response, leaving plain `text/html` —
  // that's enough to make the new tab default to Latin-1 and mangle Thai.
  // Reconstruct the Blob with the full Content-Type from the response
  // header (or fall back to a UTF-8 default for text-y files).
  const original = res.data as Blob
  const headerCT = (res.headers['content-type'] ?? '').toString()
  const fallbackCT =
    original.type ||
    (filename.endsWith('.html')
      ? 'text/html; charset=utf-8'
      : filename.endsWith('.json')
        ? 'application/json; charset=utf-8'
        : filename.endsWith('.txt')
          ? 'text/plain; charset=utf-8'
          : 'application/octet-stream')
  return new Blob([original], { type: headerCT || fallbackCT })
}

// Fetch artifact through the authenticated axios client and hand the result off
// as a blob URL — needed because <a target="_blank"> can't attach Authorization
// headers, and we don't want to leak the JWT into query strings.
export async function openArtifact(
  billID: string,
  artID: string,
  filename: string,
  mode: 'preview' | 'download',
): Promise<void> {
  try {
    const blob = await fetchArtifactBlob(billID, artID, filename, mode)
    const blobURL = URL.createObjectURL(blob)
    if (mode === 'download') {
      const a = document.createElement('a')
      a.href = blobURL
      a.download = filename
      document.body.appendChild(a)
      a.click()
      a.remove()
      // Tab is still alive so it's safe to revoke once download has started.
      setTimeout(() => URL.revokeObjectURL(blobURL), 2000)
    } else {
      window.open(blobURL, '_blank', 'noopener')
      // Don't revoke immediately — the new tab needs the URL alive.
      setTimeout(() => URL.revokeObjectURL(blobURL), 60_000)
    }
  } catch (err) {
    console.error('artifact open failed', err)
  }
}

export async function printArtifact(
  billID: string,
  artID: string,
  filename: string,
  printContext?: ArtifactPrintContext,
): Promise<void> {
  const blob = await fetchArtifactBlob(billID, artID, filename, 'preview')
  const blobURL = URL.createObjectURL(blob)
  const iframe = document.createElement('iframe')
  iframe.src = blobURL
  iframe.title = filename
  iframe.style.position = 'fixed'
  iframe.style.left = '-10000px'
  iframe.style.top = '0'
  iframe.style.width = '1px'
  iframe.style.height = '1px'
  iframe.style.border = '0'
  iframe.style.opacity = '0'

  await new Promise<void>((resolve, reject) => {
    let cleaned = false
    const cleanup = () => {
      if (cleaned) return
      cleaned = true
      URL.revokeObjectURL(blobURL)
      iframe.remove()
    }
    iframe.onload = () => {
      const win = iframe.contentWindow
      if (!win) {
        cleanup()
        reject(new Error('ไม่สามารถเปิดหน้าต่างพิมพ์ได้'))
        return
      }

      const doc = win.document
      decoratePrintableDocument(doc, printContext, 'fixed')

      win.onafterprint = cleanup
      setTimeout(cleanup, 60_000)
      const runPrint = async () => {
        await waitForPrintableImages(doc, 4000)
        await delay(100)
        try {
          win.focus()
          win.print()
          resolve()
        } catch (err) {
          cleanup()
          reject(err)
        }
      }
      void runPrint().catch((err) => {
        cleanup()
        reject(err)
      })
    }
    iframe.onerror = () => {
      cleanup()
      reject(new Error('โหลดไฟล์สำหรับพิมพ์ไม่สำเร็จ'))
    }
    document.body.appendChild(iframe)
  })
}

export async function printArtifactsBatch(items: ArtifactPrintBatchItem[]): Promise<void> {
  if (items.length === 0) return
  const parser = new DOMParser()
  const prepared = await Promise.all(items.map(async (item) => {
    const blob = await fetchArtifactBlob(item.billID, item.artID, item.filename, 'preview')
    const html = await blob.text()
    const sourceDoc = parser.parseFromString(html, 'text/html')
    decoratePrintableDocument(sourceDoc, item.printContext, 'absolute')
    return {
      title: item.filename,
      headHTML: sourceDoc.head?.innerHTML ?? '',
      bodyHTML: sourceDoc.body?.innerHTML ?? html,
    }
  }))

  const iframe = document.createElement('iframe')
  iframe.title = 'BillFlow bulk print'
  iframe.style.position = 'fixed'
  iframe.style.left = '-10000px'
  iframe.style.top = '0'
  iframe.style.width = '1px'
  iframe.style.height = '1px'
  iframe.style.border = '0'
  iframe.style.opacity = '0'
  document.body.appendChild(iframe)

  await new Promise<void>((resolve, reject) => {
    const win = iframe.contentWindow
    if (!win) {
      iframe.remove()
      reject(new Error('ไม่สามารถเปิดหน้าต่างพิมพ์ได้'))
      return
    }
    const doc = win.document
    doc.open()
    doc.write('<!doctype html><html><head><meta charset="utf-8"><title>BillFlow</title></head><body></body></html>')
    doc.close()

    const baseStyle = doc.createElement('style')
    baseStyle.textContent = `
      html,body{margin:0;padding:0;background:#fff}
      .billflow-print-page{position:relative;min-height:96vh;page-break-after:always;break-after:page;padding:0 0 28px 0}
      .billflow-print-page:last-child{page-break-after:auto;break-after:auto}
      @page{margin:12mm}
    `
    doc.head.appendChild(baseStyle)

    prepared.forEach((entry) => {
      if (entry.headHTML) {
        const headFragment = doc.createElement('div')
        headFragment.innerHTML = entry.headHTML
        Array.from(headFragment.childNodes).forEach((node) => doc.head.appendChild(node))
      }
      const page = doc.createElement('section')
      page.className = 'billflow-print-page'
      page.setAttribute('data-title', entry.title)
      page.innerHTML = entry.bodyHTML
      doc.body.appendChild(page)
    })

    const cleanup = () => iframe.remove()
    win.onafterprint = cleanup
    setTimeout(cleanup, 60_000)
    const runPrint = async () => {
      await waitForPrintableImages(doc, 6000)
      await delay(150)
      try {
        win.focus()
        win.print()
        resolve()
      } catch (err) {
        cleanup()
        reject(err)
      }
    }
    void runPrint().catch((err) => {
      cleanup()
      reject(err)
    })
  })
}

export async function recordArtifactPrint(
  billID: string,
  artID: string,
): Promise<EmailPrintEvent> {
  const res = await api.post<{ data: EmailPrintEvent }>(
    `/api/bills/${billID}/artifacts/${artID}/print-events`,
  )
  return res.data.data
}

function decoratePrintableDocument(
  doc: Document,
  printContext: ArtifactPrintContext | undefined,
  stampPosition: 'fixed' | 'absolute',
): void {
  try {
    trimMarketplacePrintFooter(doc)
  } catch { /* best-effort */ }

  const printOrders = normalizePrintOrders(printContext)
  const paymentLines = paymentPrintLines(printOrders)
  const orderDocMap = Object.fromEntries(
    printOrders
      .filter((order) => order.orderId && order.smlDocNo)
      .map((order) => [order.orderId!, order.smlDocNo!]),
  )

  if (paymentLines.length > 0 && doc.body) {
    insertPaymentPrintStamp(doc, paymentLines, stampPosition)
  }

  if (Object.keys(orderDocMap).length > 0 && doc.body) {
    injectOrderDocTextLabels(doc, orderDocMap)
  }
}

function normalizePrintOrders(printContext?: ArtifactPrintContext): ArtifactPrintOrderContext[] {
  const seen = new Set<string>()
  const out: ArtifactPrintOrderContext[] = []
  for (const order of printContext?.orders ?? []) {
    const smlDocNo = (order.smlDocNo ?? '').trim()
    if (!smlDocNo) continue
    const orderId = (order.orderId ?? '').trim()
    const key = orderId || smlDocNo
    if (seen.has(key)) continue
    seen.add(key)
    out.push({
      orderId,
      smlDocNo,
      partyCode: (order.partyCode ?? '').trim(),
      partyName: (order.partyName ?? '').trim(),
      paymentMethod: (order.paymentMethod ?? '').trim(),
    })
  }
  return out
}

function paymentPrintLines(orders: ArtifactPrintOrderContext[]): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const order of orders) {
    const line = formatCreditCardPaymentMethod(order.paymentMethod)
    if (!line || seen.has(line)) continue
    seen.add(line)
    out.push(line)
  }
  return out
}

function formatCreditCardPaymentMethod(paymentMethod?: string): string {
  const cleanPayment = (paymentMethod ?? '').trim()
  if (!cleanPayment || !cleanPayment.toUpperCase().startsWith('TT')) return ''
  return cleanPayment
}

function insertPaymentPrintStamp(
  doc: Document,
  paymentMethods: string[],
  stampPosition: 'fixed' | 'absolute',
): void {
  const inlineStamp = createPaymentPrintStamp(doc, paymentMethods, false)
  const anchor = findPaymentSectionAnchor(doc)
  if (anchor && anchor.parentElement) {
    anchor.insertAdjacentElement('afterend', inlineStamp)
    return
  }

  const floatingStamp = createPaymentPrintStamp(doc, paymentMethods, true)
  floatingStamp.style.position = stampPosition
  floatingStamp.style.right = '16px'
  floatingStamp.style.bottom = '16px'
  floatingStamp.style.zIndex = '2147483647'
  floatingStamp.style.maxWidth = '180px'
  floatingStamp.style.boxShadow = '0 1px 6px rgba(0,0,0,.14)'
  doc.body?.appendChild(floatingStamp)
}

function createPaymentPrintStamp(
  doc: Document,
  paymentMethods: string[],
  floating: boolean,
): HTMLElement {
  const stamp = doc.createElement('div')
  stamp.setAttribute('data-billflow-print', 'true')
  stamp.style.cssText = [
    floating ? 'display:inline-flex' : 'display:flex',
    'flex-direction:column',
    'gap:3px',
    floating ? '' : 'width:max-content',
    floating ? '' : 'margin:6px 0 0 auto',
    'padding:5px 8px',
    'background:#ffffff',
    'border:1.5px solid #111827',
    'border-radius:6px',
    'color:#111827',
    'font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif',
    'line-height:1.15',
    'text-align:center',
    'print-color-adjust:exact',
    '-webkit-print-color-adjust:exact',
  ].filter(Boolean).join(';')

  paymentMethods.forEach((method) => {
    const block = doc.createElement('div')
    block.style.cssText = 'display:flex;flex-direction:column;gap:1px;white-space:nowrap'

    const label = doc.createElement('div')
    label.style.cssText = 'font-size:10.5px;font-weight:700'
    label.textContent = 'จ่ายบัตรเครดิต'

    const value = doc.createElement('div')
    value.style.cssText = 'font-size:12px;font-weight:800'
    value.textContent = method

    block.appendChild(label)
    block.appendChild(value)
    stamp.appendChild(block)
  })
  return stamp
}

function findPaymentSectionAnchor(doc: Document): Element | null {
  if (!doc.body) return null
  const markers = ['รายละเอียดการชำระเงิน', 'วิธีการชำระเงิน', 'วิธีชำระเงิน']
  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_TEXT)
  let node: Node | null
  while ((node = walker.nextNode())) {
    const text = node.textContent?.replace(/\s+/g, ' ').trim() ?? ''
    if (!markers.some((marker) => text.includes(marker))) continue
    const parent = node.parentElement
    if (!parent) continue
    return parent.closest('table,section,div,p') ?? parent
  }
  return null
}

function trimMarketplacePrintFooter(doc: Document): void {
  if (!doc.body) return
  const bodyHTML = doc.body.innerHTML
  const markers = ['อย่าลืมซื้อสินค้านี้', 'ขั้นตอนต่อไป']
  const triggerIdx = markers
    .map((marker) => bodyHTML.indexOf(marker))
    .filter((idx) => idx > 0)
    .sort((a, b) => a - b)[0]
  if (!triggerIdx) return

  const cutIdx = findPrintFooterCutIndex(bodyHTML, triggerIdx)
  if (cutIdx > 0) {
    doc.body.innerHTML = bodyHTML.slice(0, cutIdx)
  }
}

interface OrderDocPrintEntry {
  key: string
  docNo: string
  variants: string[]
}

function injectOrderDocTextLabels(doc: Document, orderDocMap: Record<string, string>): void {
  if (!doc.body) return
  const entries = orderDocPrintEntries(orderDocMap)
  if (entries.length === 0) return

  const annotated = new Set<string>()
  injectOrderDocLabelsInDetailRows(doc, entries, annotated)
  injectOrderDocLabelsInFallbackText(doc, entries, annotated)
}

function orderDocPrintEntries(orderDocMap: Record<string, string>): OrderDocPrintEntry[] {
  const seen = new Set<string>()
  const entries: OrderDocPrintEntry[] = []
  for (const [orderIdValue, docNoValue] of Object.entries(orderDocMap)) {
    const orderId = orderIdValue.trim()
    const docNo = docNoValue.trim()
    const key = canonicalOrderID(orderId)
    if (!orderId || !docNo || !key || seen.has(key)) continue
    seen.add(key)
    entries.push({
      key,
      docNo,
      variants: orderIDPrintVariants(orderId),
    })
  }
  return entries
}

function canonicalOrderID(orderId: string): string {
  return orderId.trim().replace(/^#+/, '').toUpperCase()
}

function orderIDPrintVariants(orderId: string): string[] {
  const clean = orderId.trim()
  const withoutHash = clean.replace(/^#+/, '')
  const withHash = withoutHash ? `#${withoutHash}` : ''
  return [clean, withHash, withoutHash]
    .map((value) => value.trim())
    .filter((value, index, values) => value && values.indexOf(value) === index)
    .sort((a, b) => b.length - a.length)
}

function injectOrderDocLabelsInDetailRows(
  doc: Document,
  entries: OrderDocPrintEntry[],
  annotated: Set<string>,
): void {
  if (!doc.body) return
  const rows = Array.from(doc.body.querySelectorAll('tr'))
  for (const row of rows) {
    if (hiddenPrintAncestor(row) || row.closest('[data-billflow-print="true"],[data-billflow-order-label="true"]')) {
      continue
    }

    const cells = Array.from(row.children).filter(isTableCellElement)
    if (cells.length < 2) continue

    const labelCellIndex = cells.findIndex(isOrderIDLabelCell)
    if (labelCellIndex < 0) continue

    const valueCells = cells.slice(labelCellIndex + 1)
    for (const entry of entries) {
      if (annotated.has(entry.key)) continue
      for (const cell of valueCells) {
        if (insertOrderDocLabelAfterOrderInElement(doc, cell, entry, true)) {
          annotated.add(entry.key)
          break
        }
      }
    }
  }
}

function injectOrderDocLabelsInFallbackText(
  doc: Document,
  entries: OrderDocPrintEntry[],
  annotated: Set<string>,
): void {
  if (!doc.body) return
  for (const entry of entries) {
    if (annotated.has(entry.key)) continue
    if (insertOrderDocLabelAfterOrderInElement(doc, doc.body, entry, true)) {
      annotated.add(entry.key)
    }
  }
}

function isTableCellElement(element: Element): element is HTMLElement {
  const tagName = element.tagName.toLowerCase()
  return tagName === 'td' || tagName === 'th'
}

function isOrderIDLabelCell(element: Element): boolean {
  const text = normalizeElementText(element)
  return text.includes('หมายเลขคำสั่งซื้อ') || text.includes('เลขคำสั่งซื้อ')
}

function normalizeElementText(element: Element): string {
  return (element.textContent ?? '').replace(/\s+/g, ' ').trim()
}

function insertOrderDocLabelAfterOrderInElement(
  doc: Document,
  element: Element,
  entry: OrderDocPrintEntry,
  allowLinkedOrderText: boolean,
): boolean {
  const walker = doc.createTreeWalker(
    element,
    NodeFilter.SHOW_TEXT,
    {
      acceptNode(node) {
        if (!node.nodeValue) return NodeFilter.FILTER_REJECT
        const parent = node.parentElement
        if (!parent || shouldSkipOrderDocLabelNode(parent, allowLinkedOrderText)) return NodeFilter.FILTER_REJECT
        return findOrderTextMatch(node.nodeValue, entry) !== null
          ? NodeFilter.FILTER_ACCEPT
          : NodeFilter.FILTER_REJECT
      },
    },
  )

  const textNodes: Text[] = []
  while (true) {
    const node = walker.nextNode()
    if (!node) break
    textNodes.push(node as Text)
  }

  for (const textNode of textNodes) {
    const match = findOrderTextMatch(textNode.nodeValue ?? '', entry)
    if (!match) continue

    const link = allowLinkedOrderText ? textNode.parentElement?.closest('a') : null
    if (link && element.contains(link)) {
      if (link.nextElementSibling?.getAttribute('data-billflow-order-label') !== 'true') {
        link.insertAdjacentElement('afterend', createOrderDocLabel(doc, entry.docNo))
      }
      return true
    }

    const afterNode = textNode.splitText(match.index + match.length)
    afterNode.parentNode?.insertBefore(createOrderDocLabel(doc, entry.docNo), afterNode)
    return true
  }
  return false
}

function findOrderTextMatch(text: string, entry: OrderDocPrintEntry): { index: number; length: number } | null {
  const upperText = text.toUpperCase()
  for (const variant of entry.variants) {
    const index = upperText.indexOf(variant.toUpperCase())
    if (index >= 0) {
      return { index, length: variant.length }
    }
  }
  return null
}

function shouldSkipOrderDocLabelNode(element: Element, allowLinkedOrderText: boolean): boolean {
  const tagName = element.tagName.toLowerCase()
  if (['button', 'script', 'style', 'noscript'].includes(tagName)) return true
  if (!allowLinkedOrderText && element.closest('a')) return true
  if (element.closest('button,[data-billflow-print="true"],[data-billflow-order-label="true"]')) return true
  return Boolean(hiddenPrintAncestor(element))
}

function hiddenPrintAncestor(element: Element): Element | null {
  let current: Element | null = element
  while (current) {
    const style = (current.getAttribute('style') || '').replace(/\s/g, '').toLowerCase()
    if (style.includes('display:none') || style.includes('visibility:hidden')) return current
    if (current.getAttribute('hidden') !== null) return current
    current = current.parentElement
  }
  return null
}

function createOrderDocLabel(doc: Document, docNo: string): HTMLElement {
  const label = doc.createElement('span')
  label.setAttribute('data-billflow-order-label', 'true')
  label.style.cssText = [
    'display:inline-block',
    'margin-left:6px',
    'font-family:monospace',
    'font-size:11px',
    'font-weight:bold',
    'color:#059669',
    'background:#ecfdf5',
    'border:1px solid #a7f3d0',
    'border-radius:3px',
    'padding:0 4px',
    'vertical-align:baseline',
    'print-color-adjust:exact',
    '-webkit-print-color-adjust:exact',
  ].join(';')
  label.textContent = `→ ${docNo}`
  return label
}

function findPrintFooterCutIndex(bodyHTML: string, triggerIdx: number): number {
  const lazadaModuleStarts = [
    '<div  style="background:#FFFFFF',
    '<div style="background:#FFFFFF',
    '<div  style="background:#ffffff',
    '<div style="background:#ffffff',
  ]
  for (const marker of lazadaModuleStarts) {
    const idx = bodyHTML.lastIndexOf(marker, triggerIdx)
    if (idx > 0 && triggerIdx - idx < 8000) {
      return idx
    }
  }

  const openDivIdx = bodyHTML.lastIndexOf('<div', triggerIdx)
  let cutIdx = openDivIdx
  const sep1 = bodyHTML.lastIndexOf('<div', cutIdx - 1)
  if (sep1 > 0) {
    const sep2 = bodyHTML.lastIndexOf('<div', sep1 - 1)
    if (sep2 > 0 && openDivIdx - sep2 < 300) cutIdx = sep2
    else if (openDivIdx - sep1 < 200) cutIdx = sep1
  }
  return cutIdx
}

async function waitForPrintableImages(doc: Document, timeoutMs: number): Promise<void> {
  const images = Array.from(doc.images)
  if (images.length === 0) return

  images.forEach((img) => {
    const dataSrc = img.getAttribute('data-src') || img.getAttribute('data-original')
    if (!img.getAttribute('src') && dataSrc) {
      img.setAttribute('src', dataSrc)
    }
    img.removeAttribute('loading')
    img.setAttribute('loading', 'eager')
    img.setAttribute('decoding', 'sync')
    img.setAttribute('referrerpolicy', 'no-referrer')
    ;(img as HTMLImageElement & { fetchPriority?: string }).fetchPriority = 'high'
  })

  const pending = images.filter((img) => {
    const src = img.currentSrc || img.src || img.getAttribute('src') || ''
    if (!src || src.startsWith('data:')) return false
    const style = (img.getAttribute('style') || '').replace(/\s/g, '').toLowerCase()
    if (style.includes('display:none')) return false
    return !img.complete
  })
  if (pending.length === 0) return

  await Promise.race([
    Promise.all(pending.map(waitForImage)),
    delay(timeoutMs),
  ])
}

function waitForImage(img: HTMLImageElement): Promise<void> {
  return new Promise((resolve) => {
    if (img.complete) {
      resolve()
      return
    }
    const done = () => {
      img.removeEventListener('load', done)
      img.removeEventListener('error', done)
      resolve()
    }
    img.addEventListener('load', done, { once: true })
    img.addEventListener('error', done, { once: true })
  })
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
