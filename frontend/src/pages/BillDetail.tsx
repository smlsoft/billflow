import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
import { getBill, retryBill } from '../hooks/useBills'
import type { Bill, BillItem, CatalogMatch } from '../types'
import BillStatusBadge from '../components/BillStatusBadge'
import api from '../api/client'
import dayjs from 'dayjs'
import './BillDetail.css'

const SOURCE_LABELS: Record<string, string> = {
  line: 'LINE OA', email: 'Email', lazada: 'Lazada', shopee: 'Shopee',
  shopee_email: 'Shopee Email', shopee_shipped: 'Shopee จัดส่งแล้ว', manual: 'Manual',
}

const ChevronIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
    <polyline points="6,9 12,15 18,9"/>
  </svg>
)

const ArrowLeftIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <line x1="19" y1="12" x2="5" y2="12"/><polyline points="12,19 5,12 12,5"/>
  </svg>
)

const RefreshIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <polyline points="23,4 23,10 17,10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
  </svg>
)

function JsonSection({ label, data }: { label: string; data: unknown }) {
  const [open, setOpen] = useState(false)
  if (!data) return null
  const str = typeof data === 'string' ? data : JSON.stringify(data, null, 2)
  return (
    <div className="bill-json-section">
      <button type="button" className={`json-toggle${open ? ' json-toggle--open' : ''}`} onClick={() => setOpen((o) => !o)}>
        {label}
        <span className={`json-toggle-chevron${open ? ' json-toggle-chevron--open' : ''}`}>
          <ChevronIcon />
        </span>
      </button>
      {open && <pre className="json-pre">{str}</pre>}
    </div>
  )
}

// ─── Artifact section: original source files attached to a bill ─────────────
interface BillArtifact {
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

const KIND_META: Record<string, { icon: string; label: string }> = {
  email_pdf:      { icon: '📄', label: 'PDF ต้นฉบับ' },
  email_html:     { icon: '📧', label: 'Email HTML body' },
  email_envelope: { icon: '📨', label: 'Email envelope' },
  xlsx:           { icon: '📊', label: 'Shopee Excel' },
  image:          { icon: '🖼️', label: 'รูปภาพ' },
  audio:          { icon: '🎙️', label: 'ไฟล์เสียง' },
  chat_history:   { icon: '💬', label: 'LINE chat' },
}

function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(2)} MB`
}

// Fetch the artifact through the authenticated axios client and hand the
// result off as a blob URL — needed because <a target="_blank"> can't
// attach Authorization headers, and we don't want to leak the JWT into a
// query string that ends up in browser history / proxy logs.
async function openArtifact(
  billID: string,
  artID: string,
  filename: string,
  mode: 'preview' | 'download',
) {
  try {
    const res = await api.get(`/api/bills/${billID}/artifacts/${artID}/${mode}`, {
      responseType: 'blob',
    })
    const blobURL = URL.createObjectURL(res.data as Blob)
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

function ArtifactSection({ billId }: { billId: string }) {
  const [items, setItems] = useState<BillArtifact[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let alive = true
    api.get<{ data: BillArtifact[] }>(`/api/bills/${billId}/artifacts`)
      .then((r) => { if (alive) setItems(r.data.data ?? []) })
      .catch(() => { if (alive) setItems([]) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [billId])

  if (loading || items.length === 0) return null

  return (
    <>
      <h3 className="bill-detail-section-title">📎 หลักฐานต้นฉบับ ({items.length})</h3>
      <div style={{
        background: 'white', border: '1px solid #e2e8f0', borderRadius: 8,
        padding: 12, marginBottom: 16,
      }}>
        {items.map((a) => {
          const meta = KIND_META[a.kind] ?? { icon: '📎', label: a.kind }
          const previewable = a.content_type === 'application/pdf'
            || (a.content_type ?? '').startsWith('image/')
            || (a.content_type ?? '').startsWith('text/')
            || a.content_type === 'application/json'
          return (
            <div key={a.id} style={{
              display: 'flex', gap: 12, alignItems: 'center',
              padding: '8px 0', borderBottom: '1px solid #f1f5f9',
              fontSize: '0.9rem',
            }}>
              <span style={{ fontSize: '1.2rem' }}>{meta.icon}</span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ color: '#0f172a', fontWeight: 500, wordBreak: 'break-word' }}>{a.filename}</div>
                <div style={{ color: '#64748b', fontSize: '0.78rem', marginTop: 2 }}>
                  {meta.label} · {fmtSize(a.size_bytes)} · {dayjs(a.created_at).format('DD/MM/YY HH:mm')}
                </div>
              </div>
              <div style={{ display: 'flex', gap: 8, whiteSpace: 'nowrap' }}>
                {previewable && (
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => openArtifact(billId, a.id, a.filename, 'preview')}
                    title={a.sha256 ? `SHA256: ${a.sha256.slice(0, 16)}…` : ''}
                  >
                    👁 ดู
                  </button>
                )}
                <button
                  type="button"
                  className="btn btn-secondary btn-sm"
                  onClick={() => openArtifact(billId, a.id, a.filename, 'download')}
                  title={a.sha256 ? `SHA256: ${a.sha256.slice(0, 16)}…` : ''}
                >
                  ⬇ ดาวน์โหลด
                </button>
              </div>
            </div>
          )
        })}
      </div>
    </>
  )
}

// ─── Raw data card: human-readable view of bill.raw_data ─────────────────────
//
// Why a card and not raw JSON? raw_data is the audit trail "where this bill
// came from" and gets opened by non-engineers. Each ingestion flow writes
// the same standardized fields (flow tag, subject, from, customer_*,
// doc_date, etc.) so we can render labeled rows with an icon and colour-
// coded flow badge. Falls back to the raw JSON in a collapsed <details>
// so engineers can still see the full payload.
function RawDataCard({
  data,
  items,
}: {
  data: Record<string, unknown> | null | undefined
  items?: BillItem[]
}) {
  if (!data) return null

  const flow = (data.flow as string | undefined) ?? ''
  const FLOW_META: Record<string, { label: string; icon: string; bg: string; fg: string }> = {
    email_pdf:           { label: 'Email + PDF',         icon: '📎', bg: '#dbeafe', fg: '#1e40af' },
    shopee_email_order:  { label: 'Shopee Email (Order)',icon: '🛒', bg: '#ffedd5', fg: '#9a3412' },
    shopee_shipped:      { label: 'Shopee จัดส่งแล้ว (PO)',  icon: '📦', bg: '#fef3c7', fg: '#92400e' },
    shopee_excel:        { label: 'Shopee Excel',        icon: '📊', bg: '#dcfce7', fg: '#166534' },
  }
  const meta = FLOW_META[flow]

  const get = (k: string) => {
    const v = data[k]
    if (v == null) return ''
    return String(v)
  }

  const subject  = get('subject')
  const from     = get('from')
  const customer = get('customer_name')
  const phone    = get('customer_phone')
  const docDate  = get('doc_date')
  const note     = get('note')
  const file     = get('email_file')
  const msgID    = get('email_message_id')
  const orderID  = get('shopee_order_id') || get('order_id')
  const status   = get('status')

  const fieldRow = (icon: string, label: string, value: string, mono = false) => {
    if (!value) return null
    return (
      <div style={{ display: 'flex', gap: 12, padding: '6px 0', borderBottom: '1px solid #f1f5f9', fontSize: '0.9rem' }}>
        <div style={{ width: 30, fontSize: '1rem' }}>{icon}</div>
        <div style={{ minWidth: 130, color: '#64748b', fontSize: '0.85rem' }}>{label}</div>
        <div style={{
          flex: 1, color: '#0f172a',
          fontFamily: mono ? 'monospace' : undefined,
          fontSize: mono ? '0.82rem' : undefined,
          wordBreak: 'break-word',
        }}>{value}</div>
      </div>
    )
  }

  return (
    <div style={{
      background: 'white', border: '1px solid #e2e8f0', borderRadius: 8,
      padding: 16, marginBottom: 16,
    }}>
      {meta && (
        <div style={{
          display: 'inline-flex', alignItems: 'center', gap: 6,
          padding: '4px 10px', borderRadius: 12,
          background: meta.bg, color: meta.fg,
          fontSize: '0.8rem', fontWeight: 600, marginBottom: 12,
        }}>
          <span>{meta.icon}</span>
          <span>{meta.label}</span>
        </div>
      )}
      {fieldRow('📧', 'หัวข้ออีเมล', subject)}
      {fieldRow('📨', 'ผู้ส่ง', from, true)}
      {fieldRow('🛒', 'หมายเลขคำสั่งซื้อ', orderID, true)}
      {fieldRow('📅', 'วันที่เอกสาร', docDate)}
      {fieldRow('👤', 'ลูกค้า', customer)}
      {fieldRow('📞', 'เบอร์โทร', phone)}
      {fieldRow('📝', 'หมายเหตุ', note)}
      {fieldRow('📎', 'ไฟล์แนบ', file, true)}
      {fieldRow('🏷️', 'สถานะ Shopee', status)}
      {fieldRow('🆔', 'Message ID', msgID, true)}
      {items && items.length > 0 && (
        <div style={{
          marginTop: 12, paddingTop: 12, borderTop: '1px solid #e2e8f0',
        }}>
          <div style={{ fontSize: '0.85rem', color: '#64748b', marginBottom: 8 }}>
            📋 รายการที่ระบบดึงมา ({items.length} รายการ):
          </div>
          {items.map((it) => {
            const candidate = (it.candidates ?? []).find((c) => c.item_code === it.item_code)
            const score = candidate?.score
            const scorePct = score != null ? Math.round(score * 100) : null
            const scoreColor =
              score == null ? '#94a3b8' :
              score >= 0.85 ? '#15803d' :
              score >= 0.6 ? '#a16207' : '#b91c1c'
            return (
              <div key={it.id} style={{
                display: 'flex', gap: 8, padding: '6px 0',
                fontSize: '0.85rem', alignItems: 'flex-start',
                borderBottom: '1px dashed #f1f5f9',
              }}>
                <div style={{ flex: 1, color: '#0f172a', wordBreak: 'break-word' }}>
                  {it.raw_name}
                  <div style={{ fontSize: '0.78rem', color: '#64748b', marginTop: 2 }}>
                    →{' '}
                    <code style={{ color: it.item_code ? '#0f172a' : '#ef4444' }}>
                      {it.item_code ?? '(ยังไม่ map)'}
                    </code>
                    {' '}
                    {candidate?.item_name && (
                      <span style={{ color: '#475569' }}>· {candidate.item_name}</span>
                    )}
                  </div>
                </div>
                <div style={{ width: 90, textAlign: 'right', color: '#475569', whiteSpace: 'nowrap' }}>
                  {it.qty} × ฿{(it.price ?? 0).toLocaleString()}
                </div>
                {scorePct != null && (
                  <div style={{
                    width: 50, textAlign: 'center', fontWeight: 600, color: scoreColor,
                    fontSize: '0.78rem',
                  }}>
                    {scorePct}%
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
      <details style={{ marginTop: 12 }}>
        <summary style={{ cursor: 'pointer', fontSize: '0.8rem', color: '#64748b' }}>
          ดู JSON ดิบ (raw_data + items + candidates)
        </summary>
        <pre className="json-pre" style={{ marginTop: 8 }}>
{JSON.stringify({ raw_data: data, items: items ?? [] }, null, 2)}
        </pre>
      </details>
    </div>
  )
}

// ─── Map item modal: search catalog + create new product ────────────────────
function MapItemModal({
  rawName,
  currentCode,
  currentUnit,
  currentPrice,
  onPick,
  onClose,
}: {
  rawName: string
  currentCode: string
  currentUnit: string
  currentPrice: number
  onPick: (code: string, unitCode: string) => void
  onClose: () => void
}) {
  const [view, setView] = useState<'search' | 'create'>('search')
  // search state
  const [query, setQuery] = useState(rawName.slice(0, 80))
  const [results, setResults] = useState<CatalogMatch[]>([])
  const [searching, setSearching] = useState(false)
  const [searchError, setSearchError] = useState('')
  // create-product form state
  const [form, setForm] = useState({
    code: '',
    name: rawName.slice(0, 80),
    unit_code: currentUnit || 'ชิ้น',
    price: String(currentPrice || 0),
  })
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState('')

  // Debounced search → /api/catalog/search
  useEffect(() => {
    if (view !== 'search') return
    const q = query.trim()
    if (q.length < 2) {
      setResults([])
      return
    }
    const handle = setTimeout(async () => {
      setSearching(true)
      setSearchError('')
      try {
        const res = await api.get<{ results: CatalogMatch[] }>(`/api/catalog/search`, {
          params: { q, top: 10 },
        })
        setResults(res.data.results ?? [])
      } catch (err: unknown) {
        setSearchError(err instanceof Error ? err.message : 'search failed')
      } finally {
        setSearching(false)
      }
    }, 300)
    return () => clearTimeout(handle)
  }, [query, view])

  const handleCreate = async () => {
    setCreating(true)
    setCreateError('')
    try {
      const payload = {
        code: form.code.trim(),
        name: form.name.trim(),
        unit_code: form.unit_code.trim(),
        price: Number(form.price) || 0,
      }
      const res = await api.post<{ code: string; unit_code: string }>(
        '/api/catalog/products',
        payload,
      )
      onPick(res.data.code, res.data.unit_code)
      onClose()
    } catch (err: unknown) {
      // axios error shape — try to extract server message
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setCreateError(e?.response?.data?.error || e?.message || 'create failed')
    } finally {
      setCreating(false)
    }
  }

  function scoreBorder(score: number) {
    if (score >= 0.85) return '#22c55e'
    if (score >= 0.6) return '#f59e0b'
    return '#ef4444'
  }

  return (
    <div
      style={{
        position: 'fixed', inset: 0, background: 'rgba(15,23,42,0.55)', zIndex: 1000,
        display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: 'white', borderRadius: 12, padding: 24, width: '100%',
          maxWidth: 640, maxHeight: '90vh', overflow: 'auto',
          boxShadow: '0 20px 50px rgba(0,0,0,0.25)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
          <h3 style={{ margin: 0, fontSize: '1.1rem' }}>
            {view === 'search' ? 'เลือกสินค้าจาก SML Catalog' : 'สร้างสินค้าใหม่'}
          </h3>
          <button type="button" className="btn btn-ghost btn-sm" onClick={onClose}>✕</button>
        </div>

        <div style={{ marginBottom: 16, padding: 12, background: '#f8fafc', borderRadius: 6, fontSize: '0.85rem' }}>
          <div style={{ color: '#64748b', marginBottom: 4 }}>ชื่อสินค้า (raw):</div>
          <div style={{ color: '#0f172a', fontWeight: 500, wordBreak: 'break-word' }}>{rawName}</div>
          {currentCode && (
            <div style={{ marginTop: 6, color: '#64748b', fontSize: '0.8rem' }}>
              ปัจจุบัน: <code style={{ color: '#0f172a' }}>{currentCode}</code> ({currentUnit || '—'})
            </div>
          )}
        </div>

        {view === 'search' ? (
          <>
            <input
              className="form-input"
              autoFocus
              placeholder="ค้นหาด้วยชื่อสินค้า..."
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              style={{ width: '100%', marginBottom: 12 }}
            />

            {searching && <div style={{ color: '#64748b', fontSize: '0.85rem' }}>กำลังค้นหา...</div>}
            {searchError && <div className="alert alert-danger">{searchError}</div>}

            {!searching && results.length === 0 && query.trim().length >= 2 && (
              <div style={{ padding: 16, textAlign: 'center', color: '#64748b', background: '#f8fafc', borderRadius: 6 }}>
                ไม่พบสินค้าที่ตรง
              </div>
            )}

            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 16 }}>
              {results.map((r) => (
                <button
                  key={r.item_code}
                  type="button"
                  onClick={() => { onPick(r.item_code, r.unit_code); onClose() }}
                  style={{
                    textAlign: 'left',
                    padding: 10,
                    border: `2px solid ${scoreBorder(r.score)}`,
                    borderRadius: 6,
                    background: 'white',
                    cursor: 'pointer',
                    fontSize: '0.9rem',
                  }}
                >
                  <div style={{ fontWeight: 600, color: '#0f172a' }}>{r.item_code}</div>
                  <div style={{ color: '#475569', marginTop: 2 }}>{r.item_name}</div>
                  <div style={{ marginTop: 4, fontSize: '0.75rem', color: '#64748b' }}>
                    หน่วย: {r.unit_code || '—'} · score: {(r.score * 100).toFixed(0)}%
                  </div>
                </button>
              ))}
            </div>

            <div style={{
              borderTop: '1px solid #e2e8f0', paddingTop: 12, display: 'flex',
              justifyContent: 'space-between', alignItems: 'center',
            }}>
              <span style={{ fontSize: '0.85rem', color: '#64748b' }}>ไม่เจอที่ตรง?</span>
              <button
                type="button"
                className="btn btn-primary btn-sm"
                onClick={() => {
                  // Pre-fill form with sensible defaults
                  setForm((f) => ({ ...f, name: query.trim() || rawName.slice(0, 80) }))
                  setView('create')
                }}
              >
                + สร้างสินค้าใหม่
              </button>
            </div>
          </>
        ) : (
          <>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <label style={{ fontSize: '0.85rem', color: '#475569' }}>
                รหัสสินค้า (Item Code) <span style={{ color: '#ef4444' }}>*</span>
                <input
                  className="form-input"
                  autoFocus
                  value={form.code}
                  placeholder="เช่น CON-99001 หรือ INGU-VIT-30ML"
                  onChange={(e) => setForm((f) => ({ ...f, code: e.target.value }))}
                  style={{ width: '100%', marginTop: 4 }}
                />
              </label>
              <label style={{ fontSize: '0.85rem', color: '#475569' }}>
                ชื่อสินค้า <span style={{ color: '#ef4444' }}>*</span>
                <input
                  className="form-input"
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  style={{ width: '100%', marginTop: 4 }}
                />
              </label>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <label style={{ fontSize: '0.85rem', color: '#475569' }}>
                  หน่วย <span style={{ color: '#ef4444' }}>*</span>
                  <input
                    className="form-input"
                    value={form.unit_code}
                    placeholder="เช่น ชิ้น, ถุง, กระป๋อง"
                    onChange={(e) => setForm((f) => ({ ...f, unit_code: e.target.value }))}
                    style={{ width: '100%', marginTop: 4 }}
                  />
                </label>
                <label style={{ fontSize: '0.85rem', color: '#475569' }}>
                  ราคา/หน่วย
                  <input
                    className="form-input"
                    type="number"
                    step="any"
                    value={form.price}
                    onChange={(e) => setForm((f) => ({ ...f, price: e.target.value }))}
                    style={{ width: '100%', marginTop: 4 }}
                  />
                </label>
              </div>

              {createError && <div className="alert alert-danger">{createError}</div>}
            </div>

            <div style={{ marginTop: 20, display: 'flex', justifyContent: 'space-between' }}>
              <button type="button" className="btn btn-ghost btn-sm" disabled={creating} onClick={() => setView('search')}>
                ← กลับไปค้นหา
              </button>
              <button
                type="button"
                className="btn btn-primary"
                disabled={creating || !form.code.trim() || !form.name.trim() || !form.unit_code.trim()}
                onClick={handleCreate}
              >
                {creating ? 'กำลังสร้าง...' : 'สร้างและเลือกสินค้านี้'}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

// ─── Add item form (collapsible) ──────────────────────────────────────────────
function AddItemForm({
  billId,
  onAdded,
}: {
  billId: string
  onAdded: (item: BillItem) => void
}) {
  const [open, setOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState({ raw_name: '', item_code: '', unit_code: '', qty: '1', price: '0' })

  const reset = () => setDraft({ raw_name: '', item_code: '', unit_code: '', qty: '1', price: '0' })

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!draft.raw_name.trim() || Number(draft.qty) <= 0) return
    setAdding(true)
    try {
      const payload: Record<string, unknown> = {
        raw_name: draft.raw_name.trim(),
        qty: Number(draft.qty),
      }
      if (draft.item_code.trim()) payload.item_code = draft.item_code.trim()
      if (draft.unit_code.trim()) payload.unit_code = draft.unit_code.trim()
      if (Number(draft.price) > 0) payload.price = Number(draft.price)

      const res = await api.post<BillItem>(`/api/bills/${billId}/items`, payload)
      onAdded(res.data)
      reset()
      setOpen(false)
    } catch (err) {
      console.error('add item failed', err)
    } finally {
      setAdding(false)
    }
  }

  if (!open) {
    return (
      <div style={{ marginTop: 12, textAlign: 'left' }}>
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => setOpen(true)}>
          + เพิ่มรายการสินค้า
        </button>
      </div>
    )
  }

  return (
    <form
      onSubmit={handleSubmit}
      style={{
        marginTop: 12, padding: 16, border: '1px dashed #cbd5e1',
        borderRadius: 8, background: '#f8fafc',
      }}
    >
      <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr 1fr', gap: 8, marginBottom: 8 }}>
        <input
          className="form-input"
          placeholder="ชื่อสินค้า (raw)"
          value={draft.raw_name}
          onChange={(e) => setDraft((d) => ({ ...d, raw_name: e.target.value }))}
          autoFocus
          required
        />
        <input
          className="form-input"
          placeholder="Item Code (option)"
          value={draft.item_code}
          onChange={(e) => setDraft((d) => ({ ...d, item_code: e.target.value }))}
          style={{ fontFamily: 'monospace' }}
        />
        <input
          className="form-input"
          placeholder="หน่วย"
          value={draft.unit_code}
          onChange={(e) => setDraft((d) => ({ ...d, unit_code: e.target.value }))}
        />
        <input
          className="form-input"
          type="number"
          step="any"
          min="0"
          placeholder="ราคา/หน่วย"
          value={draft.price}
          onChange={(e) => setDraft((d) => ({ ...d, price: e.target.value }))}
        />
      </div>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
        <label style={{ fontSize: '0.85rem', color: '#475569' }}>
          จำนวน:
          <input
            className="form-input"
            type="number"
            step="any"
            min="0"
            value={draft.qty}
            onChange={(e) => setDraft((d) => ({ ...d, qty: e.target.value }))}
            style={{ width: 80, marginLeft: 8, display: 'inline-block' }}
          />
        </label>
        <button type="submit" className="btn btn-primary btn-sm" disabled={adding}>
          {adding ? 'กำลังเพิ่ม...' : 'เพิ่มรายการ'}
        </button>
        <button
          type="button"
          className="btn btn-ghost btn-sm"
          onClick={() => { reset(); setOpen(false) }}
          disabled={adding}
        >
          ยกเลิก
        </button>
      </div>
    </form>
  )
}

// ─── Catalog metadata cache + lookup helper ───────────────────────────────────
//
// Sources of (item_name, score) for a row, in priority order:
//   1. The item's `candidates` JSON — saved at extraction time, contains
//      item_name + score from the embedding search.
//   2. Lazy fetch /api/catalog/:code — for codes the user picked manually
//      via MapItemModal that are not in candidates.
//
// Cache keyed by item_code so we don't re-fetch on every render or for
// duplicate codes within the same bill.
const catalogMetaCache = new Map<string, { item_name: string; price?: number | null; unit_code?: string }>()

interface MatchInfo {
  itemName: string | null
  score: number | null     // 0..1, null if user-picked code outside candidates
  catalogPrice: number | null
}

function useMatchInfo(item: BillItem): MatchInfo {
  const code = item.item_code ?? ''
  const candidate = (item.candidates ?? []).find((c) => c.item_code === code)
  const [fetched, setFetched] = useState<{ item_name: string; price?: number | null } | null>(
    () => (code && catalogMetaCache.has(code) ? catalogMetaCache.get(code) ?? null : null)
  )

  useEffect(() => {
    if (!code || candidate) return
    if (catalogMetaCache.has(code)) {
      setFetched(catalogMetaCache.get(code) ?? null)
      return
    }
    let cancelled = false
    api
      .get<{ item_name: string; price?: number | null }>(`/api/catalog/${encodeURIComponent(code)}`)
      .then((res) => {
        if (cancelled) return
        const meta = { item_name: res.data.item_name, price: res.data.price }
        catalogMetaCache.set(code, meta)
        setFetched(meta)
      })
      .catch(() => { /* code not in catalog (user-typed?) — leave blank */ })
    return () => { cancelled = true }
  }, [code, candidate])

  if (candidate) {
    return {
      itemName: candidate.item_name,
      score: candidate.score,
      catalogPrice: typeof (candidate as { price?: number }).price === 'number'
        ? (candidate as { price?: number }).price ?? null
        : null,
    }
  }
  return { itemName: fetched?.item_name ?? null, score: null, catalogPrice: fetched?.price ?? null }
}

// Score → color + label
function scoreStyle(score: number | null) {
  if (score == null) return { color: '#94a3b8', bg: '#f1f5f9', label: 'manual', icon: '✎' }
  const pct = Math.round(score * 100)
  if (score >= 0.95) return { color: '#15803d', bg: '#dcfce7', label: `${pct}%`, icon: '✓' }
  if (score >= 0.85) return { color: '#15803d', bg: '#dcfce7', label: `${pct}%`, icon: '✓' }
  if (score >= 0.60) return { color: '#a16207', bg: '#fef3c7', label: `${pct}%`, icon: '⚠' }
  return { color: '#b91c1c', bg: '#fee2e2', label: `${pct}%`, icon: '⚠' }
}

function MatchBadge({ score }: { score: number | null }) {
  const s = scoreStyle(score)
  const tooltip = score == null
    ? 'รหัสนี้ไม่อยู่ใน top-5 catalog candidates ที่ระบบหาให้ — น่าจะแก้ผ่าน MapItemModal'
    : `ความใกล้เคียงกับ catalog (จาก embedding cosine similarity): ${s.label}`
  return (
    <span
      title={tooltip}
      style={{
        display: 'inline-flex', alignItems: 'center', gap: 4,
        padding: '2px 8px', borderRadius: 12,
        background: s.bg, color: s.color,
        fontSize: '0.78rem', fontWeight: 600, whiteSpace: 'nowrap',
      }}
    >
      <span>{s.icon}</span>
      <span>{s.label}</span>
    </span>
  )
}

// ─── Editable item row ───────────────────────────────────────────────────────
function ItemRow({
  item,
  billId,
  editable,
  onUpdated,
  onDeleted,
}: {
  item: BillItem
  billId: string
  editable: boolean
  onUpdated: (updated: BillItem) => void
  onDeleted: (itemId: string) => void
}) {
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [showMapModal, setShowMapModal] = useState(false)
  const [draft, setDraft] = useState({
    item_code: item.item_code ?? '',
    unit_code: item.unit_code ?? '',
    qty: String(item.qty ?? 0),
    price: String(item.price ?? 0),
  })

  const reset = () => {
    setDraft({
      item_code: item.item_code ?? '',
      unit_code: item.unit_code ?? '',
      qty: String(item.qty ?? 0),
      price: String(item.price ?? 0),
    })
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      const payload: Record<string, unknown> = {
        item_code: draft.item_code,
        unit_code: draft.unit_code,
        qty: Number(draft.qty),
        price: Number(draft.price),
      }
      await api.put(`/api/bills/${billId}/items/${item.id}`, payload)

      // F1 learning: backend created an ai_learned mapping if item_code changed.
      // Surface the success so user knows the system is learning.
      const prevCode = item.item_code ?? ''
      if (draft.item_code && draft.item_code !== prevCode) {
        toast.success('✓ จดจำการจับคู่นี้แล้ว — ครั้งถัดไประบบจะ map ให้อัตโนมัติ', {
          duration: 3500,
        })
      }

      onUpdated({
        ...item,
        item_code: draft.item_code,
        unit_code: draft.unit_code,
        qty: Number(draft.qty),
        price: Number(draft.price),
        mapped: draft.item_code !== '',
      })
      setEditing(false)
    } catch (err) {
      console.error('update item failed', err)
      toast.error('บันทึกไม่สำเร็จ')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!window.confirm(`ลบรายการ "${item.raw_name.slice(0, 40)}..." ?`)) return
    try {
      await api.delete(`/api/bills/${billId}/items/${item.id}`)
      onDeleted(item.id)
    } catch (err) {
      console.error('delete item failed', err)
    }
  }

  const matchInfo = useMatchInfo(item)
  // Catalog vs bill price diff warning — flag if > 30% off and we have both sides
  const billPrice = item.price ?? 0
  const catalogPrice = matchInfo.catalogPrice ?? 0
  const priceMismatch =
    billPrice > 0 && catalogPrice > 0 &&
    Math.abs(billPrice - catalogPrice) / catalogPrice > 0.3

  if (!editing) {
    return (
      <tr>
        <td style={{ maxWidth: 280 }}>{item.raw_name}</td>
        <td>
          {item.item_code
            ? <span className="bill-item-code">{item.item_code}</span>
            : <span className="bill-item-code bill-item-code--empty">—</span>}
        </td>
        <td style={{ maxWidth: 260, color: matchInfo.itemName ? '#0f172a' : '#94a3b8' }}>
          {matchInfo.itemName ?? '—'}
        </td>
        <td className="text-center">
          <MatchBadge score={matchInfo.score} />
        </td>
        <td className="text-right">{item.qty}</td>
        <td>{item.unit_code || '—'}</td>
        <td className="text-right bill-item-amount">
          ฿{(item.price ?? 0).toLocaleString()}
          {priceMismatch && (
            <div
              title={`Catalog ราคา ฿${catalogPrice.toLocaleString()} — ต่างจากบิล ${Math.round(Math.abs(billPrice - catalogPrice) / catalogPrice * 100)}%`}
              style={{ fontSize: '0.7rem', color: '#a16207', marginTop: 2 }}
            >
              ⚠ catalog ฿{catalogPrice.toLocaleString()}
            </div>
          )}
        </td>
        <td className="text-right bill-item-amount">
          ฿{((item.qty ?? 0) * (item.price ?? 0)).toLocaleString()}
        </td>
        {editable && (
          <td className="text-center" style={{ whiteSpace: 'nowrap' }}>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => { reset(); setEditing(true) }}>
              แก้ไข
            </button>
            {' '}
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={handleDelete}
              title="ลบรายการ"
              style={{ color: '#ef4444' }}
            >
              ลบ
            </button>
          </td>
        )}
      </tr>
    )
  }

  return (
    <>
      {showMapModal && (
        <MapItemModal
          rawName={item.raw_name}
          currentCode={draft.item_code}
          currentUnit={draft.unit_code}
          currentPrice={Number(draft.price) || 0}
          onPick={(code, unit) => setDraft((d) => ({ ...d, item_code: code, unit_code: unit || d.unit_code }))}
          onClose={() => setShowMapModal(false)}
        />
      )}
      <tr>
        <td style={{ maxWidth: 280 }}>{item.raw_name}</td>
        <td>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => setShowMapModal(true)}
            style={{ minWidth: 140, justifyContent: 'flex-start' }}
            title="เปิดเพื่อค้นหาหรือสร้างสินค้าใหม่"
          >
            {draft.item_code
              ? <span style={{ fontFamily: 'monospace' }}>{draft.item_code}</span>
              : <span style={{ color: '#94a3b8' }}>เลือกสินค้า...</span>}
          </button>
        </td>
        <td style={{ maxWidth: 260, color: matchInfo.itemName ? '#0f172a' : '#94a3b8' }}>
          {matchInfo.itemName ?? '—'}
        </td>
        <td className="text-center">
          <MatchBadge score={matchInfo.score} />
        </td>
        <td className="text-right">
          <input
            className="form-input"
            type="number"
            step="any"
            value={draft.qty}
            onChange={(e) => setDraft((d) => ({ ...d, qty: e.target.value }))}
            style={{ width: 80, textAlign: 'right' }}
          />
        </td>
        <td>
          <input
            className="form-input"
            value={draft.unit_code}
            onChange={(e) => setDraft((d) => ({ ...d, unit_code: e.target.value }))}
            style={{ width: 80 }}
          />
        </td>
        <td className="text-right">
          <input
            className="form-input"
            type="number"
            step="any"
            value={draft.price}
            onChange={(e) => setDraft((d) => ({ ...d, price: e.target.value }))}
            style={{ width: 100, textAlign: 'right' }}
          />
        </td>
        <td className="text-right bill-item-amount">
          ฿{(Number(draft.qty || 0) * Number(draft.price || 0)).toLocaleString()}
        </td>
        <td className="text-center" style={{ whiteSpace: 'nowrap' }}>
          <button type="button" className="btn btn-primary btn-sm" disabled={saving} onClick={handleSave}>
            {saving ? '...' : 'บันทึก'}
          </button>
          {' '}
          <button type="button" className="btn btn-ghost btn-sm" disabled={saving} onClick={() => setEditing(false)}>
            ยกเลิก
          </button>
        </td>
      </tr>
    </>
  )
}

// MatchDropdown + NeedsReviewSection were removed — superseded by the per-row
// MapItemModal (search + create new) in ItemRow's edit mode, which covers the
// same needs_review flow with full catalog search instead of just top-5.

export default function BillDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [bill, setBill] = useState<Bill | null>(null)
  const [loading, setLoading] = useState(true)
  const [retrying, setRetrying] = useState(false)
  const [retryError, setRetryError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    getBill(id).then(setBill).catch(() => setBill(null)).finally(() => setLoading(false))
  }, [id])

  const handleRetry = async () => {
    if (!id) return
    setRetrying(true)
    setRetryError(null)
    try {
      await retryBill(id)
      const updated = await getBill(id)
      setBill(updated)
    } catch {
      setRetryError('Retry ล้มเหลว — กรุณาลองใหม่อีกครั้ง')
    } finally {
      setRetrying(false)
    }
  }

  if (loading) {
    return (
      <div>
        <div className="skeleton" style={{ height: 28, width: 120, marginBottom: 20 }} />
        <div className="skeleton" style={{ height: 200, borderRadius: 8 }} />
      </div>
    )
  }

  if (!bill) {
    return (
      <div>
        <button type="button" className="bill-detail-back" onClick={() => navigate(-1)}>
          <ArrowLeftIcon /> กลับ
        </button>
        <div className="alert alert-danger">ไม่พบบิลที่ต้องการ</div>
      </div>
    )
  }

  const rawData = bill.raw_data as Record<string, unknown> | null
  const total = (bill.items ?? []).reduce((s, i) => s + (i.qty ?? 0) * (i.price ?? 0), 0)
  const canSend = bill.status === 'failed' || bill.status === 'pending' || bill.status === 'needs_review'
  const canEdit = canSend
  const isPurchase = bill.bill_type === 'purchase'
  const counterpartyLabel = isPurchase ? 'ผู้ขาย (Supplier)' : 'ลูกค้า'

  return (
    <div>
      <button type="button" className="bill-detail-back" onClick={() => navigate(-1)}>
        <ArrowLeftIcon /> กลับ
      </button>

      {/* Main info card */}
      <div className="bill-detail-card">
        <div className="bill-detail-card-header">
          <h2 className="bill-detail-doc-no">
            {bill.sml_doc_no ?? bill.id.slice(0, 8)}
          </h2>
          <BillStatusBadge status={bill.status} />
        </div>

        <div className="bill-detail-card-body">
          <div className="bill-detail-info-grid">
            <div className="bill-detail-info-row">
              <span className="bill-detail-info-label">{counterpartyLabel}</span>
              <span className="bill-detail-info-value">{(rawData?.customer_name as string) || '—'}</span>
            </div>
            <div className="bill-detail-info-row">
              <span className="bill-detail-info-label">เบอร์โทร</span>
              <span className="bill-detail-info-value">{(rawData?.customer_phone as string) || '—'}</span>
            </div>
            <div className="bill-detail-info-row">
              <span className="bill-detail-info-label">Platform</span>
              <span className="bill-detail-info-value">{SOURCE_LABELS[bill.source] ?? bill.source}</span>
            </div>
            <div className="bill-detail-info-row">
              <span className="bill-detail-info-label">วันที่สร้าง</span>
              <span className="bill-detail-info-value">{dayjs(bill.created_at).format('DD/MM/YYYY HH:mm')}</span>
            </div>
            {bill.sent_at && (
              <div className="bill-detail-info-row">
                <span className="bill-detail-info-label">ส่ง SML เมื่อ</span>
                <span className="bill-detail-info-value">{dayjs(bill.sent_at).format('DD/MM/YYYY HH:mm')}</span>
              </div>
            )}
            {bill.ai_confidence != null && (
              <div className="bill-detail-info-row">
                <span className="bill-detail-info-label">AI Confidence</span>
                <span className="bill-detail-info-value">{Math.round(bill.ai_confidence * 100)}%</span>
              </div>
            )}
          </div>

          {bill.error_msg && (
            <div className="alert alert-danger">{bill.error_msg}</div>
          )}
          {retryError && (
            <div className="alert alert-danger alert-danger--mt">{retryError}</div>
          )}
        </div>

        <div className="bill-detail-total">
          <span className="bill-detail-total-label">ยอดรวมทั้งหมด</span>
          <span className="bill-detail-total-amount">฿{total.toLocaleString()}</span>
        </div>

        {canSend && (
          <div className="bill-detail-actions">
            <button
              type="button"
              className="btn btn-primary btn-sm"
              onClick={handleRetry}
              disabled={retrying}
            >
              <RefreshIcon />
              {retrying
                ? 'กำลังส่ง...'
                : `${bill.status === 'failed' ? '⚠️ ' : ''}ยืนยันและส่งไปยัง SML${isPurchase ? ' (PO)' : ''}`}
            </button>
          </div>
        )}
      </div>

      {/* Items — per-row MapItemModal handles needs_review now */}
      <h3 className="bill-detail-section-title">รายการสินค้า ({bill.items?.length ?? 0} รายการ)</h3>
      <div className="bill-items-table-wrap">
        <table className="bill-items-table">
          <thead>
            <tr>
              <th>ชื่อสินค้า (จาก source)</th>
              <th>Item Code</th>
              <th>SML Item Name</th>
              <th className="text-center">Match</th>
              <th className="text-right">จำนวน</th>
              <th>หน่วย</th>
              <th className="text-right">ราคา/หน่วย</th>
              <th className="text-right">รวม</th>
              {canEdit && <th className="text-center">จัดการ</th>}
            </tr>
          </thead>
          <tbody>
            {(bill.items ?? []).map((item) => (
              <ItemRow
                key={item.id}
                item={item}
                billId={bill.id}
                editable={canEdit}
                onUpdated={(updated) => {
                  setBill((prev) => {
                    if (!prev) return prev
                    return {
                      ...prev,
                      items: (prev.items ?? []).map((it) =>
                        it.id === updated.id ? { ...it, ...updated } : it
                      ),
                    }
                  })
                }}
                onDeleted={(id) => {
                  setBill((prev) => {
                    if (!prev) return prev
                    return { ...prev, items: (prev.items ?? []).filter((it) => it.id !== id) }
                  })
                }}
              />
            ))}
          </tbody>
        </table>
      </div>

      {canEdit && (
        <AddItemForm
          billId={bill.id}
          onAdded={(newItem) => {
            setBill((prev) => {
              if (!prev) return prev
              return { ...prev, items: [...(prev.items ?? []), newItem] }
            })
          }}
        />
      )}

      {/* Source metadata + SML request/response */}
      {bill.raw_data && (
        <>
          <h3 className="bill-detail-section-title">ข้อมูลที่รับมา</h3>
          <RawDataCard data={bill.raw_data as Record<string, unknown>} items={bill.items} />
        </>
      )}
      <ArtifactSection billId={bill.id} />
      {(bill.sml_payload || bill.sml_response) && (
        <>
          <h3 className="bill-detail-section-title">SML Request / Response</h3>
          <JsonSection label="sml_payload (ข้อมูลที่ส่งไป SML)" data={bill.sml_payload} />
          <JsonSection label="sml_response (ผลตอบกลับจาก SML)" data={bill.sml_response} />
        </>
      )}
    </div>
  )
}
