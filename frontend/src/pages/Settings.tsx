import { useEffect, useState } from 'react'
import { useAuth } from '../hooks/useAuth'
import client from '../api/client'
import type { DashboardStats, PlatformColumnMapping } from '../types'
import './Settings.css'

type ConfigStatus = {
  line_configured: boolean
  imap_configured: boolean
  sml_configured: boolean
  ai_configured: boolean
  auto_confirm_threshold: number
}

const PLATFORM_FIELDS: Array<{ key: string; label: string }> = [
  { key: 'order_id',    label: 'หมายเลขออเดอร์' },
  { key: 'buyer_name',  label: 'ชื่อลูกค้า' },
  { key: 'buyer_phone', label: 'เบอร์โทร' },
  { key: 'item_name',   label: 'ชื่อสินค้า' },
  { key: 'sku',         label: 'SKU' },
  { key: 'qty',         label: 'จำนวน' },
  { key: 'price',       label: 'ราคาต่อหน่วย' },
]

function StatusRow({ ok, label }: { ok: boolean; label: string }) {
  return (
    <div className="settings-status-row">
      <span className={`settings-status-dot ${ok ? 'settings-status-dot--ok' : 'settings-status-dot--err'}`} />
      <span className="settings-status-label">{label}</span>
      <span className={`settings-status-tag ${ok ? 'settings-status-tag--ok' : 'settings-status-tag--err'}`}>
        {ok ? 'พร้อมใช้งาน' : 'ยังไม่ได้ตั้งค่า'}
      </span>
    </div>
  )
}

function ColumnMappingEditor({ platform }: { platform: 'lazada' | 'shopee' }) {
  const [mappings, setMappings] = useState<PlatformColumnMapping[]>([])
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    client.get<{ mappings: PlatformColumnMapping[] }>(`/api/settings/column-mappings/${platform}`)
      .then((r) => {
        const map = new Map(r.data.mappings.map((m) => [m.field_name, m]))
        setMappings(PLATFORM_FIELDS.map((f) => map.get(f.key) ?? { platform, field_name: f.key, column_name: '' }))
      })
      .catch(() => {
        setMappings(PLATFORM_FIELDS.map((f) => ({ platform, field_name: f.key, column_name: '' })))
      })
  }, [platform])

  const updateColumnName = (fieldName: string, value: string) => {
    setMappings((prev) => prev.map((m) => m.field_name === fieldName ? { ...m, column_name: value } : m))
  }

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    setSaved(false)
    try {
      await client.put(`/api/settings/column-mappings/${platform}`, { mappings })
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } }
      setError(err?.response?.data?.error ?? 'บันทึกไม่สำเร็จ')
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <table className="settings-col-table">
        <thead>
          <tr>
            <th>Field</th>
            <th>ชื่อ Column ในไฟล์ Excel</th>
          </tr>
        </thead>
        <tbody>
          {PLATFORM_FIELDS.map((f) => {
            const m = mappings.find((x) => x.field_name === f.key)
            return (
              <tr key={f.key}>
                <td>
                  <div className="settings-col-field-key">{f.key}</div>
                  <div className="settings-col-field-label">{f.label}</div>
                </td>
                <td>
                  <input
                    className="form-input"
                    type="text"
                    value={m?.column_name ?? ''}
                    onChange={(e) => updateColumnName(f.key, e.target.value)}
                    placeholder="ชื่อ column จริงในไฟล์..."
                  />
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      <div className="settings-save-row">
        <button type="button" className="btn btn-primary btn-sm" onClick={handleSave} disabled={saving}>
          {saving ? 'กำลังบันทึก...' : 'บันทึก'}
        </button>
        {saved && <span className="settings-save-ok">บันทึกแล้ว</span>}
        {error && <span className="settings-save-err">{error}</span>}
      </div>
    </>
  )
}

export default function Settings() {
  const { user } = useAuth()
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [config, setConfig] = useState<ConfigStatus | null>(null)
  // Shopee column mapping is hardcoded server-side, so the editor only handles Lazada.
  const colMapTab: 'lazada' | 'shopee' = 'lazada'

  useEffect(() => {
    client.get<DashboardStats>('/api/dashboard/stats').then((r) => setStats(r.data)).catch(() => null)
    client.get<ConfigStatus>('/api/settings/status').then((r) => setConfig(r.data)).catch(() => null)
  }, [])

  return (
    <div>
      <h1 className="settings-title">ตั้งค่า</h1>

      {/* User info */}
      <div className="settings-card">
        <div className="settings-card-header">
          <h2 className="settings-card-title">ข้อมูลผู้ใช้</h2>
        </div>
        <div className="settings-card-body">
          <div className="settings-info-row">
            <span className="settings-info-label">ชื่อ</span>
            <span className="settings-info-value">{user?.name}</span>
          </div>
          <div className="settings-info-row">
            <span className="settings-info-label">อีเมล</span>
            <span className="settings-info-value">{user?.email}</span>
          </div>
          <div className="settings-info-row">
            <span className="settings-info-label">สิทธิ์</span>
            <span className="settings-info-value">{user?.role}</span>
          </div>
        </div>
      </div>

      {/* Connection status */}
      <div className="settings-card">
        <div className="settings-card-header">
          <h2 className="settings-card-title">สถานะการเชื่อมต่อ</h2>
        </div>
        <div className="settings-card-body">
          {config ? (
            <>
              <StatusRow ok={config.line_configured} label="LINE OA Webhook" />
              <StatusRow ok={config.imap_configured} label="Email (IMAP)" />
              <StatusRow ok={config.sml_configured} label="SML ERP API" />
              <StatusRow ok={config.ai_configured} label="OpenRouter AI" />
              <div className="settings-threshold">
                <span>Auto-confirm Threshold</span>
                <span className="settings-threshold-value">{(config.auto_confirm_threshold * 100).toFixed(0)}%</span>
              </div>
            </>
          ) : (
            <p className="settings-version">ไม่สามารถโหลดสถานะการเชื่อมต่อได้</p>
          )}
        </div>
      </div>

      {/* Column mapping (admin only) */}
      {user?.role === 'admin' && (
        <div className="settings-card settings-card--wide">
          <div className="settings-card-header">
            <h2 className="settings-card-title">Column Mapping — Lazada</h2>
          </div>
          <div className="settings-card-body">
            <p className="settings-col-desc">
              กำหนดชื่อ column ในไฟล์ Excel ให้ตรงกับ field ที่ระบบใช้งาน
              <br/>
              <small style={{ color: '#94a3b8' }}>
                ℹ️ Shopee ใช้ column hardcoded — ปรับใน code ที่ <code>backend/internal/handlers/shopee_import.go</code>
              </small>
            </p>
            <ColumnMappingEditor key={colMapTab} platform={colMapTab} />
          </div>
        </div>
      )}

      {/* Stats summary */}
      {stats && (
        <div className="settings-card">
          <div className="settings-card-header">
            <h2 className="settings-card-title">สรุประบบ</h2>
          </div>
          <div className="settings-card-body">
            <div className="settings-stats-grid">
              <div className="settings-stat-row">
                <span className="settings-stat-label">บิลทั้งหมด</span>
                <span className="settings-stat-value">{stats.total_bills}</span>
              </div>
              <div className="settings-stat-row">
                <span className="settings-stat-label">SML สำเร็จ</span>
                <span className="settings-stat-value">{stats.sml_success}</span>
              </div>
              <div className="settings-stat-row">
                <span className="settings-stat-label">รอดำเนินการ</span>
                <span className="settings-stat-value">{stats.pending}</span>
              </div>
              <div className="settings-stat-row">
                <span className="settings-stat-label">ล้มเหลว</span>
                <span className="settings-stat-value">{stats.sml_failed}</span>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* About */}
      <div className="settings-card">
        <div className="settings-card-header">
          <h2 className="settings-card-title">เกี่ยวกับระบบ</h2>
        </div>
        <div className="settings-card-body">
          <p className="settings-version">BillFlow v0.2.0 — AI-powered bill processing system</p>
        </div>
      </div>
    </div>
  )
}
