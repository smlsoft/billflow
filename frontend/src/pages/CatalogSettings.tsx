import { useEffect, useState, useRef, useCallback, useMemo, useReducer } from 'react'
import api from '../api/client'
import type { CatalogItem } from '../types'
import './CatalogSettings.css'

interface CatalogStats {
  total: number
  embedded: number
  pending: number
  error: number
  index_size: number
  embed_running: boolean
}

interface ListResponse {
  data: CatalogItem[]
  total: number
  page: number
  per_page: number
}

type StatusFilter = '' | 'pending' | 'done' | 'error'

const SearchIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
  </svg>
)

const ClearIcon = () => (
  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
    <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
  </svg>
)

function Pagination({ page, total, perPage, onChange }: { page: number; total: number; perPage: number; onChange: (p: number) => void }) {
  const totalPages = Math.max(1, Math.ceil(total / perPage))
  if (totalPages <= 1) return null

  const pages: (number | '…')[] = []
  if (totalPages <= 7) {
    for (let i = 1; i <= totalPages; i++) pages.push(i)
  } else {
    pages.push(1)
    if (page > 3) pages.push('…')
    for (let i = Math.max(2, page - 1); i <= Math.min(totalPages - 1, page + 1); i++) pages.push(i)
    if (page < totalPages - 2) pages.push('…')
    pages.push(totalPages)
  }

  return (
    <div className="catalog-pagination">
      <button type="button" className="catalog-page-btn" disabled={page <= 1} onClick={() => onChange(page - 1)}>‹</button>
      {pages.map((p, i) =>
        p === '…'
          ? <span key={`e${i}`} className="catalog-page-ellipsis">…</span>
          : <button
              type="button"
              key={p}
              className={`catalog-page-btn${page === p ? ' catalog-page-btn--active' : ''}`}
              onClick={() => onChange(p as number)}
            >{p}</button>
      )}
      <button type="button" className="catalog-page-btn" disabled={page >= totalPages} onClick={() => onChange(page + 1)}>›</button>
    </div>
  )
}

// Fetch params bundled so useCallback has stable deps
interface FetchParams { page: number; filter: StatusFilter; query: string }

export default function CatalogSettings() {
  const [stats, setStats] = useState<CatalogStats | null>(null)
  const [items, setItems] = useState<CatalogItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [syncing, setSyncing] = useState(false)
  const [embedding, setEmbedding] = useState(false)
  const [message, setMessage] = useState<{ text: string; ok: boolean } | null>(null)
  // draft = what user is typing; params = what was actually fetched
  const [draft, setDraft] = useState('')
  const [params, setParams] = useReducer(
    (_prev: FetchParams, next: Partial<FetchParams> & { reset?: boolean }) => {
      const base = next.reset ? { page: 1, filter: '' as StatusFilter, query: '' } : _prev
      return { ...base, page: next.page ?? 1, filter: next.filter ?? base.filter, query: next.query ?? base.query }
    },
    { page: 1, filter: '', query: '' }
  )
  const fileRef = useRef<HTMLInputElement>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const PER_PAGE = 50

  const fetchStats = useCallback(async () => {
    const res = await api.get<CatalogStats>('/api/catalog/stats')
    setStats(res.data)
    return res.data
  }, [])

  const fetchItems = useCallback(async (p: FetchParams) => {
    setLoading(true)
    try {
      const reqParams: Record<string, unknown> = { page: p.page, per_page: PER_PAGE }
      if (p.filter) reqParams.status = p.filter
      if (p.query.trim()) reqParams.q = p.query.trim()
      const res = await api.get<ListResponse>('/api/catalog', { params: reqParams })
      setItems(res.data.data ?? [])
      setTotal(res.data.total ?? 0)
    } finally {
      setLoading(false)
    }
  }, [])

  // Fetch when params change
  useEffect(() => {
    fetchItems(params)
  }, [params, fetchItems])

  // Initial stats load
  useEffect(() => {
    fetchStats()
  }, [fetchStats])

  // Auto-refresh stats while embedding is running
  useEffect(() => {
    if (stats?.embed_running) {
      pollRef.current = setInterval(async () => {
        const s = await fetchStats()
        if (!s.embed_running) {
          if (pollRef.current) clearInterval(pollRef.current)
          fetchItems(params)
        }
      }, 3000)
    } else {
      if (pollRef.current) clearInterval(pollRef.current)
    }
    return () => { if (pollRef.current) clearInterval(pollRef.current) }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stats?.embed_running])

  function notify(text: string, ok = true) {
    setMessage({ text, ok })
    setTimeout(() => setMessage(null), 4000)
  }

  function handleFilterChange(f: StatusFilter) {
    setDraft('')
    setParams({ filter: f, page: 1, query: '' })
  }

  function commitSearch(q: string) {
    setParams({ query: q.trim(), page: 1 })
  }

  function handleSearchKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') commitSearch(draft)
    if (e.key === 'Escape') { setDraft(''); commitSearch('') }
  }

  function handleClearSearch() {
    setDraft('')
    commitSearch('')
  }

  function handlePageChange(p: number) {
    setParams({ page: p })
  }

  async function handleSync() {
    setSyncing(true)
    try {
      const res = await api.post<{ synced: number }>('/api/catalog/sync')
      notify(`Sync สำเร็จ ${res.data.synced} รายการ`)
      fetchStats()
      fetchItems(params)
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      notify(msg ?? 'Sync ล้มเหลว', false)
    } finally {
      setSyncing(false)
    }
  }

  async function handleCSVUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    const form = new FormData()
    form.append('file', file)
    try {
      const res = await api.post<{ synced: number }>('/api/catalog/import-csv', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      notify(`Import CSV สำเร็จ ${res.data.synced} รายการ`)
      fetchStats()
      fetchItems(params)
    } catch {
      notify('Import CSV ล้มเหลว', false)
    } finally {
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  async function handleEmbedAll() {
    setEmbedding(true)
    try {
      const res = await api.post<{ message: string }>('/api/catalog/embed-all')
      notify(res.data.message ?? 'เริ่ม embed แล้ว')
      fetchStats()
    } catch {
      notify('Embed ล้มเหลว', false)
    } finally {
      setEmbedding(false)
    }
  }

  async function handleReload() {
    try {
      await api.post('/api/catalog/reload-index')
      notify('Reload index สำเร็จ')
      fetchStats()
    } catch {
      notify('Reload ล้มเหลว', false)
    }
  }

  async function handleEmbedOne(code: string) {
    try {
      await api.post(`/api/catalog/${code}/embed`)
      notify(`Embed ${code} สำเร็จ`)
      fetchStats()
      fetchItems(params)
    } catch {
      notify(`Embed ${code} ล้มเหลว`, false)
    }
  }

  const pct = useMemo(() =>
    stats && stats.total > 0 ? Math.round(stats.embedded / stats.total * 100) : 0
  , [stats])


  const isEmbedBusy = embedding || (stats?.embed_running ?? false)

  return (
    <div className="catalog-page">

      {/* ── Top bar ── */}
      <div className="catalog-topbar">
        <div className="catalog-heading">
          <h1 className="catalog-title">Catalog สินค้า SML</h1>
          <p className="catalog-subtitle">สินค้าจาก SML ใช้สำหรับ Smart Matching อีเมล Shopee</p>
        </div>
        <div className="catalog-actions">
          <button type="button" className="btn btn-secondary btn-sm" onClick={handleSync} disabled={syncing}>
            {syncing ? '↻ กำลัง Sync...' : '↻ Sync จาก SML API'}
          </button>
          <label className="btn btn-secondary btn-sm catalog-csv-btn">
            ↑ Import CSV
            <input ref={fileRef} type="file" accept=".csv" onChange={handleCSVUpload} hidden />
          </label>
          <button type="button" className="btn btn-primary btn-sm" onClick={handleEmbedAll} disabled={isEmbedBusy}>
            {isEmbedBusy ? '⏳ กำลัง Embed...' : '✦ Embed All Pending'}
          </button>
          <button type="button" className="btn btn-secondary btn-sm" onClick={handleReload}>
            ⚡ Reload Index
          </button>
        </div>
      </div>

      {/* ── Toast ── */}
      {message && (
        <div className={`catalog-toast ${message.ok ? 'catalog-toast--ok' : 'catalog-toast--err'}`}>
          {message.ok ? '✓' : '✕'} {message.text}
        </div>
      )}

      {/* ── Stats row ── */}
      {stats && (
        <div className="catalog-stats-row">
          <div className="catalog-stat-card">
            <span className="catalog-stat-num">{stats.total.toLocaleString()}</span>
            <span className="catalog-stat-lbl">สินค้าทั้งหมด</span>
          </div>
          <div className="catalog-stat-card catalog-stat-card--green">
            <span className="catalog-stat-num catalog-stat-num--green">{stats.embedded.toLocaleString()}</span>
            <span className="catalog-stat-lbl">Embedded</span>
          </div>
          <div className="catalog-stat-card catalog-stat-card--yellow">
            <span className="catalog-stat-num catalog-stat-num--yellow">{stats.pending.toLocaleString()}</span>
            <span className="catalog-stat-lbl">Pending</span>
          </div>
          <div className="catalog-stat-card catalog-stat-card--blue">
            <span className="catalog-stat-num catalog-stat-num--blue">{stats.index_size.toLocaleString()}</span>
            <span className="catalog-stat-lbl">Index Size</span>
          </div>
          {stats.embed_running ? (
            <div className="catalog-stat-card catalog-stat-card--pulse">
              <div className="catalog-embed-running">
                <div className="catalog-embed-running-dot" />
                กำลัง Embed...
              </div>
              <div className="catalog-embed-hint">auto-refresh ทุก 3 วินาที</div>
            </div>
          ) : stats.error > 0 ? (
            <div className="catalog-stat-card catalog-stat-card--red">
              <span className="catalog-stat-num catalog-stat-num--red">{stats.error}</span>
              <span className="catalog-stat-lbl">Error</span>
            </div>
          ) : null}
        </div>
      )}

      {/* ── Progress bar ── */}
      {stats && stats.total > 0 && (
        <div className="catalog-progress-wrap">
          <div className="catalog-progress-meta">
            <span className="catalog-progress-label">Embedding Progress</span>
            <span className="catalog-progress-pct">{stats.embedded.toLocaleString()} / {stats.total.toLocaleString()} ({pct}%)</span>
          </div>
          <div className="catalog-progress-track">
            <div
              className="catalog-progress-fill"
              style={{ '--progress': `${pct}%` } as React.CSSProperties}
            />
          </div>
        </div>
      )}

      {/* ── Toolbar ── */}
      <div className="catalog-toolbar">
        <div className="catalog-tabs">
          {([
            { key: '' as StatusFilter,        label: 'ทั้งหมด',  count: stats?.total,    cls: '' },
            { key: 'done' as StatusFilter,    label: 'Embedded', count: stats?.embedded, cls: '' },
            { key: 'pending' as StatusFilter, label: 'Pending',  count: stats?.pending,  cls: 'catalog-tab-count--yellow' },
            { key: 'error' as StatusFilter,   label: 'Error',    count: stats?.error,    cls: 'catalog-tab-count--red' },
          ]).map(({ key, label, count, cls }) => (
            <button
              type="button"
              key={key}
              className={`catalog-tab${params.filter === key ? ' catalog-tab--active' : ''}`}
              onClick={() => handleFilterChange(key)}
            >
              {label}
              {count != null && count > 0 && (
                <span className={`catalog-tab-count ${params.filter === key ? '' : cls}`}>
                  {count > 9999 ? '9999+' : count}
                </span>
              )}
            </button>
          ))}
        </div>
        <div className="catalog-search">
          <span className="catalog-search-icon"><SearchIcon /></span>
          <input
            type="text"
            className="catalog-search-input"
            placeholder="ค้นหา... (Enter เพื่อค้นหา)"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={handleSearchKeyDown}
          />
          {draft && (
            <button type="button" className="catalog-search-clear" onClick={handleClearSearch} title="ล้างการค้นหา">
              <ClearIcon />
            </button>
          )}
          <button type="button" className="catalog-search-btn" onClick={() => commitSearch(draft)}>
            ค้นหา
          </button>
        </div>
      </div>

      {/* ── Table area ── */}
      <div className="catalog-table-area">
        <div className="catalog-table-scroll">
          <table className="catalog-table">
            <thead>
              <tr>
                <th className="col-code">Item Code</th>
                <th>ชื่อสินค้า</th>
                <th className="col-unit">หน่วย</th>
                <th className="col-price">ราคา</th>
                <th className="col-status">Embed Status</th>
                <th className="col-date">Embedded At</th>
                <th className="col-action">Action</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={7} className="catalog-state-cell">
                    <div className="catalog-loading-spinner" />
                    กำลังโหลด...
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={7} className="catalog-state-cell">
                    {params.query ? `ไม่พบสินค้าที่ตรงกับ "${params.query}"` : 'ไม่มีข้อมูล'}
                  </td>
                </tr>
              ) : items.map((item) => (
                <tr key={item.item_code}>
                  <td className="catalog-col-code">{item.item_code}</td>
                  <td>
                    <div className="catalog-col-name">{item.item_name}</div>
                    {item.item_name2 && <div className="catalog-col-name2">{item.item_name2}</div>}
                  </td>
                  <td className="catalog-col-unit">{item.unit_code || '—'}</td>
                  <td className="catalog-col-price">
                    {item.sale_price != null ? `฿${item.sale_price.toLocaleString()}` : '—'}
                  </td>
                  <td className="col-status">
                    <span className={`catalog-badge catalog-badge--${item.embedding_status}`}>
                      {item.embedding_status === 'done' ? '✓ done' : item.embedding_status}
                    </span>
                  </td>
                  <td className="catalog-col-date">
                    {item.embedded_at ? new Date(item.embedded_at).toLocaleDateString('th-TH') : '—'}
                  </td>
                  <td className="col-action">
                    {item.embedding_status !== 'done' && (
                      <button type="button" className="btn btn-secondary btn-xs" onClick={() => handleEmbedOne(item.item_code)}>
                        Embed
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* ── Footer / pagination ── */}
        <div className="catalog-footer">
          <span className="catalog-footer-info">
            {loading ? 'กำลังโหลด...' : `${total.toLocaleString()} รายการ · หน้า ${params.page} / ${Math.max(1, Math.ceil(total / PER_PAGE))}`}
          </span>
          <Pagination page={params.page} total={total} perPage={PER_PAGE} onChange={handlePageChange} />
        </div>
      </div>

    </div>
  )
}
