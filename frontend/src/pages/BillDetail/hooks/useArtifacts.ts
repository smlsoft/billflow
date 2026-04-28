import { useState, useEffect } from 'react'
import api from '@/api/client'

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
          : 'application/octet-stream')
    const fullType = headerCT || fallbackCT
    const blob = new Blob([original], { type: fullType })
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
