import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
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

// ─── Editable item row ───────────────────────────────────────────────────────
function ItemRow({
  item,
  billId,
  editable,
  onUpdated,
}: {
  item: BillItem
  billId: string
  editable: boolean
  onUpdated: (updated: BillItem) => void
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
    } finally {
      setSaving(false)
    }
  }

  if (!editing) {
    return (
      <tr>
        <td>{item.raw_name}</td>
        <td>
          {item.item_code
            ? <span className="bill-item-code">{item.item_code}</span>
            : <span className="bill-item-code bill-item-code--empty">—</span>}
        </td>
        <td className="text-right">{item.qty}</td>
        <td>{item.unit_code || '—'}</td>
        <td className="text-right bill-item-amount">฿{(item.price ?? 0).toLocaleString()}</td>
        <td className="text-right bill-item-amount">
          ฿{((item.qty ?? 0) * (item.price ?? 0)).toLocaleString()}
        </td>
        <td className="text-center">
          <span className={`badge ${item.mapped ? 'badge-success' : 'badge-warning'}`}>
            {item.mapped ? 'จับคู่แล้ว' : 'ยังไม่จับคู่'}
          </span>
        </td>
        {editable && (
          <td className="text-center">
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => { reset(); setEditing(true) }}>
              แก้ไข
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
        <td>{item.raw_name}</td>
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
        <td className="text-center">
          <span className="badge badge-warning">กำลังแก้ไข</span>
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

// ─── Needs Review: smart catalog match dropdown ──────────────────────────────
function MatchDropdown({
  item,
  billId,
  onConfirmed,
}: {
  item: BillItem
  billId: string
  onConfirmed: (itemId: string, match: CatalogMatch) => void
}) {
  const candidates: CatalogMatch[] = item.candidates ?? []
  const [selected, setSelected] = useState<string>(candidates[0]?.item_code ?? '')
  const [saving, setSaving] = useState(false)

  const selectedMatch = candidates.find((c) => c.item_code === selected)

  function scoreBorder(score: number) {
    if (score >= 0.85) return '#22c55e'
    if (score >= 0.6) return '#f59e0b'
    return '#ef4444'
  }

  const handleConfirm = async () => {
    if (!selectedMatch) return
    setSaving(true)
    try {
      await api.post(`/api/bills/${billId}/items/${item.id}/confirm-match`, {
        item_code: selectedMatch.item_code,
        unit_code: selectedMatch.unit_code,
      })
      onConfirmed(item.id, selectedMatch)
    } catch {
      // silently fail — parent can show error
    } finally {
      setSaving(false)
    }
  }

  if (item.mapped) {
    return (
      <div className="match-confirmed">
        ✓ {item.item_code} <span className="match-confirmed-unit">({item.unit_code})</span>
      </div>
    )
  }

  if (candidates.length === 0) {
    return <span className="match-no-candidates">ไม่พบสินค้าใกล้เคียง</span>
  }

  return (
    <div
      className="match-dropdown-wrap"
      style={{ borderLeft: `3px solid ${scoreBorder(candidates[0]?.score ?? 0)}` }}
    >
      <select
        className="match-select"
        value={selected}
        aria-label={`เลือกสินค้าสำหรับ ${item.raw_name}`}
        onChange={(e) => setSelected(e.target.value)}
      >
        {candidates.map((c) => (
          <option key={c.item_code} value={c.item_code}>
            [{Math.round(c.score * 100)}%] {c.item_code} — {c.item_name} ({c.unit_code})
          </option>
        ))}
      </select>
      <button
        type="button"
        className="btn btn-primary btn-xs"
        onClick={handleConfirm}
        disabled={saving || !selected}
      >
        {saving ? '...' : 'ยืนยัน'}
      </button>
    </div>
  )
}

function NeedsReviewSection({
  bill,
  onItemConfirmed,
}: {
  bill: Bill
  onItemConfirmed: (itemId: string, match: CatalogMatch) => void
}) {
  const unmapped = (bill.items ?? []).filter((i) => !i.mapped)
  const mapped = (bill.items ?? []).filter((i) => i.mapped)

  return (
    <div className="needs-review-section">
      <div className="needs-review-header">
        <span className="needs-review-icon">🛒</span>
        <div>
          <strong>รอยืนยัน Shopee Order</strong>
          {bill.sml_order_id && (
            <span className="needs-review-order-id"> #{bill.sml_order_id}</span>
          )}
          <div className="needs-review-sub">
            ยืนยัน {mapped.length}/{(bill.items ?? []).length} รายการแล้ว
          </div>
        </div>
      </div>

      <div className="needs-review-items">
        {(bill.items ?? []).map((item) => (
          <div key={item.id} className={`needs-review-item ${item.mapped ? 'needs-review-item--done' : ''}`}>
            <div className="needs-review-item-name">
              {item.raw_name}
              <span className="needs-review-item-qty"> × {item.qty} {item.unit_code ?? ''}</span>
            </div>
            <MatchDropdown item={item} billId={bill.id} onConfirmed={onItemConfirmed} />
          </div>
        ))}
      </div>

      {unmapped.length > 0 && (
        <div className="needs-review-hint">
          กรุณายืนยันสินค้าที่ยังไม่ map ({unmapped.length} รายการ) ก่อนส่ง SML
        </div>
      )}
    </div>
  )
}

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

  const handleItemConfirmed = (itemId: string, match: CatalogMatch) => {
    setBill((prev) => {
      if (!prev) return prev
      return {
        ...prev,
        items: (prev.items ?? []).map((item) =>
          item.id === itemId
            ? { ...item, mapped: true, item_code: match.item_code, unit_code: match.unit_code }
            : item
        ),
      }
    })
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
              className={bill.status === 'failed' ? 'btn btn-danger btn-sm' : 'btn btn-primary btn-sm'}
              onClick={handleRetry}
              disabled={retrying}
            >
              <RefreshIcon />
              {retrying
                ? 'กำลังส่ง...'
                : bill.status === 'failed'
                  ? 'ลองส่ง SML อีกครั้ง'
                  : `ยืนยันและส่งไปยัง SML ${isPurchase ? '(PO)' : ''}`}
            </button>
          </div>
        )}
      </div>

      {/* Needs Review — Shopee email smart match */}
      {bill.status === 'needs_review' && (
        <NeedsReviewSection bill={bill} onItemConfirmed={handleItemConfirmed} />
      )}

      {/* Items */}
      <h3 className="bill-detail-section-title">รายการสินค้า ({bill.items?.length ?? 0} รายการ)</h3>
      <div className="bill-items-table-wrap">
        <table className="bill-items-table">
          <thead>
            <tr>
              <th>ชื่อสินค้า</th>
              <th>Item Code</th>
              <th className="text-right">จำนวน</th>
              <th>หน่วย</th>
              <th className="text-right">ราคา/หน่วย</th>
              <th className="text-right">รวม</th>
              <th className="text-center">Mapping</th>
              {canEdit && <th className="text-center">แก้ไข</th>}
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
              />
            ))}
          </tbody>
        </table>
      </div>

      {/* JSON sections */}
      {(bill.raw_data || bill.sml_payload || bill.sml_response) && (
        <>
          <h3 className="bill-detail-section-title">ข้อมูล Request / Response</h3>
          <JsonSection label="raw_data (ข้อมูลดิบที่รับมา)" data={bill.raw_data} />
          <JsonSection label="sml_payload (ข้อมูลที่ส่งไป SML)" data={bill.sml_payload} />
          <JsonSection label="sml_response (ผลตอบกลับจาก SML)" data={bill.sml_response} />
        </>
      )}
    </div>
  )
}
