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
  smlDocNo?: string,
  orderID?: string,
  orderDocMap?: Record<string, string>,
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

      // ตัด Shopee footer ออกตั้งแต่ "ขั้นตอนต่อไป" ลงไป
      // ใช้ innerHTML cut แทน DOM traversal — หา marker text แล้วตัด HTML ทิ้งจากจุดนั้น
      try {
        const FOOTER_TRIGGER = 'ขั้นตอนต่อไป'
        const bodyHTML = doc.body.innerHTML
        // หาตำแหน่ง opening <div ที่ครอบ "ขั้นตอนต่อไป"
        const triggerIdx = bodyHTML.indexOf(FOOTER_TRIGGER)
        if (triggerIdx > 0) {
          // walk back หา <div ที่เริ่มก่อน trigger (closest opening div)
          const openDivIdx = bodyHTML.lastIndexOf('<div', triggerIdx)
          // ลบ 2 separator divs ก่อนหน้าด้วย — หา <div ที่อยู่ก่อน openDivIdx
          // separator มักเป็น empty div เส้นขีด height:1px
          let cutIdx = openDivIdx
          // หา <div ก่อน openDivIdx อีก 2 ครั้ง
          const sep1 = bodyHTML.lastIndexOf('<div', cutIdx - 1)
          if (sep1 > 0) {
            const sep2 = bodyHTML.lastIndexOf('<div', sep1 - 1)
            // ใช้ sep2 ถ้าใกล้พอ (< 200 chars จาก openDivIdx)
            if (sep2 > 0 && openDivIdx - sep2 < 300) cutIdx = sep2
            else if (openDivIdx - sep1 < 200) cutIdx = sep1
          }
          doc.body.innerHTML = bodyHTML.slice(0, cutIdx)
        }
      } catch { /* best-effort */ }

      // inject sml doc_no banner ที่ด้านบนสุด
      if (smlDocNo && doc.body) {
        const banner = doc.createElement('div')
        banner.style.cssText = [
          'font-family: monospace',
          'font-size: 13px',
          'font-weight: bold',
          'padding: 6px 10px',
          'background: #f3f4f6',
          'border-bottom: 2px solid #374151',
          'margin-bottom: 10px',
          'print-color-adjust: exact',
          '-webkit-print-color-adjust: exact',
        ].join(';')
        const hasMultiple = orderDocMap && Object.keys(orderDocMap).length > 1
        if (hasMultiple) {
          banner.textContent = 'เลขเอกสาร SML:'
          Object.entries(orderDocMap!).forEach(([oid, docNo]) => {
            if (!docNo) return
            const line = doc.createElement('div')
            line.style.cssText = 'padding-left:12px;font-size:12px;font-weight:normal'
            line.textContent = `${oid} → ${docNo}`
            banner.appendChild(line)
          })
        } else {
          const orderPart = orderID ? ` | คำสั่งซื้อ: ${orderID}` : ''
          banner.textContent = `เลขเอกสาร SML: ${smlDocNo}${orderPart}`
        }
        doc.body.insertBefore(banner, doc.body.firstChild)
      }

      // inject label ใต้แต่ละ order_id ที่ปรากฏใน email HTML
      if (orderDocMap && doc.body) {
        const bodyHTML = doc.body.innerHTML
        let patched = bodyHTML
        Object.entries(orderDocMap).forEach(([oid, docNo]) => {
          if (!docNo || !oid) return
          const escaped = oid.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
          // inject หลัง order_id ทุกที่ที่พบใน text node (ใช้ regex แทน DOM เพื่อความเร็ว)
          patched = patched.replace(
            new RegExp(`(${escaped})(?![^<]*→)`, 'g'),
            `$1<span style="display:inline-block;margin-left:6px;font-family:monospace;font-size:11px;font-weight:bold;color:#059669;background:#ecfdf5;border:1px solid #a7f3d0;border-radius:3px;padding:0 4px">→ ${docNo}</span>`,
          )
        })
        if (patched !== bodyHTML) {
          doc.body.innerHTML = patched
        }
      }

      win.onafterprint = cleanup
      setTimeout(cleanup, 60_000)
      setTimeout(() => {
        try {
          win.focus()
          win.print()
          resolve()
        } catch (err) {
          cleanup()
          reject(err)
        }
      }, 100)
    }
    iframe.onerror = () => {
      cleanup()
      reject(new Error('โหลดไฟล์สำหรับพิมพ์ไม่สำเร็จ'))
    }
    document.body.appendChild(iframe)
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
