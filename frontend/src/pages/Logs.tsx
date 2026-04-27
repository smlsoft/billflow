import { useEffect, useState } from 'react'
import api from '../api/client'
import dayjs from 'dayjs'
import './Logs.css'

interface AuditLog {
  id: string
  user_id?: string
  action: string
  target_id?: string
  source?: string
  level?: string
  duration_ms?: number
  trace_id?: string
  detail?: Record<string, unknown>
  created_at: string
}

interface LogsResponse {
  data: AuditLog[]
  total: number
  page: number
  page_size: number
}

const ACTION_CONFIG: Record<string, { label: string; bg: string; color: string; emoji: string }> = {
  bill_created: { label: 'สร้างบิล',      bg: '#eff6ff', color: '#1d4ed8', emoji: '📥' },
  sml_sent:     { label: 'ส่ง SML สำเร็จ', bg: '#f0fdf4', color: '#15803d', emoji: '✅' },
  sml_failed:   { label: 'SML ล้มเหลว',   bg: '#fef2f2', color: '#b91c1c', emoji: '❌' },
  bill_pending: { label: 'รอ confirm',     bg: '#fff7ed', color: '#c2410c', emoji: '⏳' },
  bill_retry:   { label: 'Retry',          bg: '#faf5ff', color: '#7c3aed', emoji: '🔄' },
}

const SOURCE_LABELS: Record<string, string> = {
  line: 'LINE', line_oa: 'LINE', email: 'Email', lazada: 'Lazada',
  shopee: 'Shopee', shopee_excel: 'Shopee', manual: 'Manual', sml: 'SML', system: 'System',
}

const LEVEL_CONFIG: Record<string, { label: string; color: string }> = {
  info:  { label: 'info',  color: '#6b7280' },
  warn:  { label: 'warn',  color: '#d97706' },
  error: { label: 'error', color: '#dc2626' },
}

function DurationBadge({ ms }: { ms?: number }) {
  if (!ms) return null
  const color = ms > 3000 ? '#dc2626' : ms > 1000 ? '#d97706' : '#6b7280'
  return <span style={{ color, fontSize: '0.72rem', marginLeft: 4 }}>{ms}ms</span>
}

function TraceIdChip({ id }: { id?: string }) {
  if (!id) return null
  const short = id.length > 20 ? id.slice(0, 16) + '…' : id
  return (
    <span
      className="log-trace-id"
      title={id}
      onClick={() => { navigator.clipboard?.writeText(id) }}
      style={{ cursor: 'copy', fontFamily: 'monospace', fontSize: '0.7rem', color: '#94a3b8', marginLeft: 4 }}
    >
      {short}
    </span>
  )
}

function JsonBlock({ label, data }: { label: string; data: unknown }) {
  const [open, setOpen] = useState(false)
  if (!data) return null
  const str = typeof data === 'string' ? data : JSON.stringify(data, null, 2)
  return (
    <div>
      <button type="button" className="log-json-toggle" onClick={() => setOpen((o) => !o)}>
        {open ? '▲' : '▼'} {label}
      </button>
      {open && <pre className="log-json-pre">{str}</pre>}
    </div>
  )
}

function LogRow({ log }: { log: AuditLog }) {
  const cfg = ACTION_CONFIG[log.action] ?? { label: log.action, bg: 'var(--color-bg-alt)', color: 'var(--color-text-muted)', emoji: '•' }
  const source = log.source || (log.detail?.source as string) || ''
  const docNo = (log.detail?.doc_no as string) || ''
  const errMsg = (log.detail?.error as string) || ''
  const via = (log.detail?.via as string) || ''
  const isError = log.level === 'error'
  const billLink = log.target_id ? `/bills/${log.target_id}` : null

  return (
    <div className="log-row" style={isError ? { borderLeft: '3px solid #dc2626', paddingLeft: 8 } : undefined}>
      <div className="log-timeline-col">
        <div className="log-dot" style={{ '--dot-bg': cfg.bg, '--dot-border': cfg.color, background: cfg.bg, borderColor: cfg.color } as React.CSSProperties}>
          {cfg.emoji}
        </div>
        <div className="log-dot-line" />
      </div>
      <div className="log-content">
        <div className="log-meta">
          <span className="log-action-badge" style={{ background: cfg.bg, color: cfg.color }}>{cfg.label}</span>
          {source && <span className="log-source-tag">{SOURCE_LABELS[source] ?? source}</span>}
          {via && <span className="log-via-tag">via {via}</span>}
          {docNo && <span className="log-doc-no">{docNo}</span>}
          {log.level && log.level !== 'info' && (
            <span style={{ fontSize: '0.7rem', color: LEVEL_CONFIG[log.level]?.color ?? '#6b7280', fontWeight: 600 }}>
              [{log.level}]
            </span>
          )}
          <DurationBadge ms={log.duration_ms} />
          <TraceIdChip id={log.trace_id} />
          <span className="log-timestamp">{dayjs(log.created_at).format('DD/MM/YY HH:mm:ss')}</span>
        </div>
        {log.target_id && (
          <p className="log-bill-id">
            Bill ID: {billLink ? <a href={billLink}>{log.target_id}</a> : log.target_id}
          </p>
        )}
        {errMsg && <p className="log-error-msg">{errMsg}</p>}
        <div className="log-json-blocks">
          {log.detail?.raw_data != null && <JsonBlock label="raw_data" data={log.detail.raw_data} />}
          {log.detail?.sml_payload != null && <JsonBlock label="sml_payload" data={log.detail.sml_payload} />}
          {log.detail?.sml_response != null && <JsonBlock label="sml_response" data={log.detail.sml_response} />}
        </div>
      </div>
    </div>
  )
}

export default function Logs() {
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [source, setSource] = useState('')
  const [action, setAction] = useState('')
  const [level, setLevel] = useState('')
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')
  const pageSize = 30

  const load = async (p = page) => {
    setLoading(true)
    try {
      const params: Record<string, string | number> = { page: p, page_size: pageSize }
      if (source) params.source = source
      if (action) params.action = action
      if (level) params.level = level
      if (dateFrom) params.date_from = dateFrom
      if (dateTo) params.date_to = dateTo
      const res = await api.get<LogsResponse>('/api/logs', { params })
      setLogs(res.data.data || [])
      setTotal(res.data.total || 0)
      setPage(p)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load(1) }, [source, action, level, dateFrom, dateTo])

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div>
      <div className="logs-header">
        <h1 className="logs-title">Activity Log</h1>
        <p className="logs-subtitle">บันทึกทุก event — รับอะไรมา ส่งอะไรไป SML ผลลัพธ์เป็นอย่างไร</p>
      </div>

      <div className="logs-filter-bar">
        <div className="logs-filter-field">
          <label className="logs-filter-label" htmlFor="logs-source">Channel</label>
          <select id="logs-source" className="form-select" value={source} onChange={(e) => setSource(e.target.value)}>
            <option value="">ทั้งหมด</option>
            <option value="line">LINE</option>
            <option value="email">Email</option>
            <option value="lazada">Lazada</option>
            <option value="shopee_excel">Shopee</option>
            <option value="sml">SML</option>
            <option value="system">System</option>
          </select>
        </div>
        <div className="logs-filter-field">
          <label className="logs-filter-label" htmlFor="logs-action">Action</label>
          <select id="logs-action" className="form-select" value={action} onChange={(e) => setAction(e.target.value)}>
            <option value="">ทั้งหมด</option>
            <option value="bill_created">สร้างบิล</option>
            <option value="sml_sent">ส่ง SML สำเร็จ</option>
            <option value="sml_failed">SML ล้มเหลว</option>
            <option value="bill_pending">รอ confirm</option>
            <option value="shopee_import_done">Shopee นำเข้าสำเร็จ</option>
          </select>
        </div>
        <div className="logs-filter-field">
          <label className="logs-filter-label" htmlFor="logs-level">Level</label>
          <select id="logs-level" className="form-select" value={level} onChange={(e) => setLevel(e.target.value)}>
            <option value="">ทั้งหมด</option>
            <option value="info">info</option>
            <option value="warn">warn</option>
            <option value="error">error</option>
          </select>
        </div>
        <div className="logs-filter-field">
          <label className="logs-filter-label" htmlFor="logs-date-from">ตั้งแต่</label>
          <input id="logs-date-from" type="date" className="form-input" value={dateFrom} onChange={(e) => setDateFrom(e.target.value)} />
        </div>
        <div className="logs-filter-field">
          <label className="logs-filter-label" htmlFor="logs-date-to">ถึง</label>
          <input id="logs-date-to" type="date" className="form-input" value={dateTo} onChange={(e) => setDateTo(e.target.value)} />
        </div>
        <button
          type="button"
          className="btn btn-secondary btn-sm"
          onClick={() => { setSource(''); setAction(''); setLevel(''); setDateFrom(''); setDateTo('') }}
        >
          ล้างตัวกรอง
        </button>
      </div>

      <p className="logs-summary">{loading ? 'กำลังโหลด...' : `พบ ${total.toLocaleString()} รายการ`}</p>

      <div className="logs-timeline">
        {logs.length === 0 && !loading && (
          <p className="logs-timeline-empty">ไม่มีข้อมูล</p>
        )}
        {logs.map((log) => <LogRow key={log.id} log={log} />)}
      </div>

      {totalPages > 1 && (
        <div className="logs-pagination">
          <button type="button" className="btn btn-secondary btn-sm" disabled={page <= 1} onClick={() => load(page - 1)}>
            ก่อนหน้า
          </button>
          <span className="logs-page-label">หน้า {page} / {totalPages}</span>
          <button type="button" className="btn btn-secondary btn-sm" disabled={page >= totalPages} onClick={() => load(page + 1)}>
            ถัดไป
          </button>
        </div>
      )}
    </div>
  )
}
