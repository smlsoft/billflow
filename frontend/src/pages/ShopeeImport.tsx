import { useState, useRef, useEffect } from 'react'
import client from '../api/client'
import './ShopeeImport.css'

interface ShopeeConfig {
  server_url: string; guid: string; provider: string; config_file_name: string
  database_name: string; doc_format_code: string; cust_code: string; sale_code: string
  branch_code: string; wh_code: string; shelf_code: string; unit_code: string
  vat_type: number; vat_rate: number; doc_time: string
}

interface ShopeeOrderItem { sku: string; product_name: string; price: number; qty: number }
interface ShopeeOrder {
  order_id: string; doc_date: string; status: string
  items: ShopeeOrderItem[]; item_count: number; total_qty: number; duplicate: boolean
}
interface PreviewResponse {
  orders: ShopeeOrder[]; warnings: string[]
  total_orders: number; duplicate_count: number; skipped_count: number
}
interface ConfirmResult { order_id: string; success: boolean; doc_no?: string; message?: string }

function fmt(n: number) { return n.toLocaleString('th-TH', { minimumFractionDigits: 2 }) }

function ConfigField({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <div className="shopee-field">
      <label className="shopee-field-label">{label}</label>
      <input className="form-input" value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  )
}

function ConfigDialog({ config, onSave, onCancel }: { config: ShopeeConfig; onSave: (c: ShopeeConfig) => void; onCancel: () => void }) {
  const [cfg, setCfg] = useState<ShopeeConfig>({ ...config })
  const set = (k: keyof ShopeeConfig, v: string | number) => setCfg((p) => ({ ...p, [k]: v }))

  return (
    <div className="shopee-overlay">
      <div className="shopee-dialog">
        <h2 className="shopee-dialog-title">ตั้งค่า Shopee SML</h2>
        <div className="shopee-dialog-grid">
          <ConfigField label="Server URL" value={cfg.server_url} onChange={(v) => set('server_url', v)} />
          <ConfigField label="Doc Format Code" value={cfg.doc_format_code} onChange={(v) => set('doc_format_code', v)} />
          <ConfigField label="GUID" value={cfg.guid} onChange={(v) => set('guid', v)} />
          <ConfigField label="Provider" value={cfg.provider} onChange={(v) => set('provider', v)} />
          <ConfigField label="Config File Name" value={cfg.config_file_name} onChange={(v) => set('config_file_name', v)} />
          <ConfigField label="Database Name" value={cfg.database_name} onChange={(v) => set('database_name', v)} />
          <ConfigField label="รหัสลูกค้า (Cust Code)" value={cfg.cust_code} onChange={(v) => set('cust_code', v)} />
          <ConfigField label="รหัสพนักงานขาย (Sale Code)" value={cfg.sale_code} onChange={(v) => set('sale_code', v)} />
          <ConfigField label="รหัสสาขา (Branch Code)" value={cfg.branch_code} onChange={(v) => set('branch_code', v)} />
          <ConfigField label="รหัสคลัง (WH Code)" value={cfg.wh_code} onChange={(v) => set('wh_code', v)} />
          <ConfigField label="รหัสชั้นวาง (Shelf Code)" value={cfg.shelf_code} onChange={(v) => set('shelf_code', v)} />
          <ConfigField label="หน่วย (Unit Code)" value={cfg.unit_code} onChange={(v) => set('unit_code', v)} />
          <ConfigField label="เวลาเอกสาร (Doc Time)" value={cfg.doc_time} onChange={(v) => set('doc_time', v)} />
        </div>
        <div className="shopee-dialog-row">
          <div className="shopee-field">
            <label className="shopee-field-label" htmlFor="shopee-vat-type">VAT Type</label>
            <select id="shopee-vat-type" className="form-select" value={cfg.vat_type} onChange={(e) => set('vat_type', Number(e.target.value))}>
              <option value={0}>0 — แยกนอก</option>
              <option value={1}>1 — รวมใน</option>
              <option value={2}>2 — ศูนย์%</option>
            </select>
          </div>
          <div className="shopee-field">
            <label className="shopee-field-label" htmlFor="shopee-vat-rate">VAT Rate (%)</label>
            <input id="shopee-vat-rate" className="form-input" type="number" value={cfg.vat_rate} onChange={(e) => set('vat_rate', Number(e.target.value))} />
          </div>
        </div>
        <div className="shopee-dialog-actions">
          <button type="button" className="btn btn-secondary" onClick={onCancel}>ยกเลิก</button>
          <button type="button" className="btn btn-primary" onClick={() => onSave(cfg)}>บันทึก</button>
        </div>
      </div>
    </div>
  )
}

function SummaryCard({ label, value, colorClass }: { label: string; value: number; colorClass: string }) {
  return (
    <div className={`shopee-summary-card ${colorClass}`}>
      <div className="shopee-summary-value">{value}</div>
      <div className="shopee-summary-label">{label}</div>
    </div>
  )
}

type Step = 'idle' | 'uploading' | 'preview' | 'confirming' | 'done'

export default function ShopeeImport() {
  const fileRef = useRef<HTMLInputElement>(null)
  const [step, setStep] = useState<Step>('idle')
  const [config, setConfig] = useState<ShopeeConfig | null>(null)
  const [showConfigDialog, setShowConfigDialog] = useState(false)
  const [preview, setPreview] = useState<PreviewResponse | null>(null)
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set())
  const [results, setResults] = useState<{ success_count: number; fail_count: number; results: ConfirmResult[] } | null>(null)
  const [error, setError] = useState('')
  const [expandedOrders, setExpandedOrders] = useState<Set<string>>(new Set())

  // Load config once on mount — saves a round-trip + dialog popup before every upload.
  useEffect(() => {
    let alive = true
    client.get<ShopeeConfig>('/api/settings/shopee-config')
      .then((res) => { if (alive) setConfig(res.data) })
      .catch(() => { if (alive) setError('โหลด config ไม่ได้') })
    return () => { alive = false }
  }, [])

  const handlePickFile = () => {
    if (!config) return
    fileRef.current?.click()
  }

  const handleConfigSave = (cfg: ShopeeConfig) => {
    setConfig(cfg)
    setShowConfigDialog(false)
  }

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file || !config) return
    e.target.value = ''
    setStep('uploading'); setError(''); setPreview(null); setResults(null)
    const form = new FormData()
    form.append('file', file)
    try {
      const res = await client.post<PreviewResponse>('/api/import/shopee/preview', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      const data = res.data
      setPreview(data)
      setSelectedIDs(new Set(data.orders.filter((o) => !o.duplicate).map((o) => o.order_id)))
      setStep('preview')
    } catch (err: unknown) {
      setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'อัปโหลดไฟล์ไม่ได้')
      setStep('idle')
    }
  }

  const handleConfirm = async () => {
    if (!preview || !config || selectedIDs.size === 0) return
    setStep('confirming'); setError('')
    try {
      const res = await client.post(
        '/api/import/shopee/confirm',
        { config, order_ids: Array.from(selectedIDs), orders: preview.orders },
      )
      setResults(res.data); setStep('done')
    } catch (err: unknown) {
      setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'ส่งข้อมูลไม่ได้')
      setStep('preview')
    }
  }

  const toggleOrder = (id: string) => setSelectedIDs((prev) => { const s = new Set(prev); if (s.has(id)) s.delete(id); else s.add(id); return s })
  const toggleAll = () => {
    if (!preview) return
    const nonDup = preview.orders.filter((o) => !o.duplicate).map((o) => o.order_id)
    setSelectedIDs(selectedIDs.size === nonDup.length ? new Set() : new Set(nonDup))
  }
  const toggleExpand = (id: string) => setExpandedOrders((prev) => { const s = new Set(prev); if (s.has(id)) s.delete(id); else s.add(id); return s })

  return (
    <div>
      <div className="shopee-header">
        <h1 className="shopee-title">นำเข้า Shopee</h1>
        <p className="shopee-subtitle">อัปโหลดไฟล์ Excel จาก Shopee Seller Center → สร้างใบกำกับสินค้าใน SML</p>
      </div>

      <div className="alert" style={{
        background: '#eff6ff', borderLeft: '3px solid #3b82f6', color: '#1e40af',
        padding: 12, borderRadius: 6, marginBottom: 16, fontSize: '0.9rem',
      }}>
        💡 <strong>ใช้สำหรับ bulk import เท่านั้น</strong> — ถ้าตั้ง email forwarding ของ Shopee แล้ว
        ระบบดึง order/shipping emails อัตโนมัติทุก 5 นาที (ดูที่ <a href="/bills?source=shopee_email">Shopee Email</a> และ <a href="/bills?source=shopee_shipped">Shopee จัดส่งแล้ว</a>)
      </div>

      <input ref={fileRef} type="file" accept=".xlsx" className="visually-hidden" onChange={handleFileChange} />

      {showConfigDialog && config && (
        <ConfigDialog config={config} onSave={handleConfigSave} onCancel={() => setShowConfigDialog(false)} />
      )}

      {error && <div className="alert alert-danger shopee-alert">{error}</div>}

      {(step === 'idle' || step === 'uploading') && (
        <>
          {config && (
            <div style={{
              background: '#f8fafc', border: '1px solid #e2e8f0', borderRadius: 6,
              padding: 10, marginBottom: 16, display: 'flex', alignItems: 'center',
              justifyContent: 'space-between', fontSize: '0.85rem',
            }}>
              <div style={{ color: '#475569' }}>
                <strong>SML config:</strong>{' '}
                <code>{config.database_name}</code> /{' '}
                Cust: <code>{config.cust_code || '—'}</code> /{' '}
                WH: <code>{config.wh_code || '—'}</code> /{' '}
                Doc: <code>{config.doc_format_code}</code> /{' '}
                VAT: {config.vat_rate}% (type {config.vat_type})
              </div>
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={() => setShowConfigDialog(true)}
              >
                แก้ config
              </button>
            </div>
          )}
          <div className="shopee-dropzone">
            {step === 'uploading' ? (
              <p className="shopee-dropzone-text">กำลังวิเคราะห์ไฟล์...</p>
            ) : (
              <>
                <div className="shopee-dropzone-icon">📂</div>
                <p className="shopee-dropzone-text">คลิกเพื่อเลือกไฟล์ Excel (.xlsx) จาก Shopee</p>
                <button type="button" className="btn btn-primary" onClick={handlePickFile} disabled={!config}>
                  {config ? 'เลือกไฟล์ Shopee' : 'กำลังโหลด config...'}
                </button>
              </>
            )}
          </div>
        </>
      )}

      {step === 'preview' && preview && (
        <>
          <div className="shopee-summary-row">
            <SummaryCard label="Orders ทั้งหมด" value={preview.total_orders} colorClass="shopee-result-total" />
            <SummaryCard label="เลือกแล้ว" value={selectedIDs.size} colorClass="shopee-result-success" />
            <SummaryCard label="ซ้ำ (ข้ามไป)" value={preview.duplicate_count} colorClass="" />
          </div>

          {(preview.warnings ?? []).length > 0 && (
            <div className="alert alert-warning shopee-alert">
              <strong>คำเตือน ({(preview.warnings ?? []).length} รายการ)</strong>
              <ul className="shopee-warning-list">
                {(preview.warnings ?? []).map((w, i) => <li key={i}>{w}</li>)}
              </ul>
            </div>
          )}

          <div className="shopee-action-bar">
            <button type="button" className="btn btn-secondary btn-sm" onClick={toggleAll}>
              {selectedIDs.size === preview.orders.filter((o) => !o.duplicate).length ? 'ยกเลิกทั้งหมด' : 'เลือกทั้งหมด'}
            </button>
            <button type="button" className="btn btn-primary btn-sm" disabled={selectedIDs.size === 0} onClick={handleConfirm}>
              ยืนยันส่ง {selectedIDs.size} Orders → SML
            </button>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => { setStep('idle'); setPreview(null) }}>
              เลือกไฟล์ใหม่
            </button>
          </div>

          <div className="shopee-table-wrap">
            <table className="shopee-table">
              <thead>
                <tr>
                  <th scope="col">
                    <input type="checkbox"
                      aria-label="เลือกทั้งหมด"
                      checked={selectedIDs.size === preview.orders.filter((o) => !o.duplicate).length}
                      onChange={toggleAll}
                    />
                  </th>
                  <th>Order ID</th>
                  <th>วันที่</th>
                  <th>สถานะ</th>
                  <th>สินค้า</th>
                  <th>Qty รวม</th>
                  <th>หมายเหตุ</th>
                </tr>
              </thead>
              <tbody>
                {preview.orders.map((order) => (
                  <>
                    <tr key={order.order_id} className={order.duplicate ? 'is-duplicate' : ''}>
                      <td>
                        <input type="checkbox" aria-label={`เลือก order ${order.order_id}`} checked={selectedIDs.has(order.order_id)} disabled={order.duplicate} onChange={() => toggleOrder(order.order_id)} />
                      </td>
                      <td>
                        <button type="button" className="shopee-order-id" onClick={() => toggleExpand(order.order_id)}>
                          {order.order_id}
                        </button>
                      </td>
                      <td>{order.doc_date}</td>
                      <td><span className="shopee-status-tag">{order.status}</span></td>
                      <td>{order.item_count} รายการ</td>
                      <td>{order.total_qty}</td>
                      <td>{order.duplicate && <span className="shopee-dup-tag">มีในระบบแล้ว</span>}</td>
                    </tr>
                    {expandedOrders.has(order.order_id) && (
                      <tr key={order.order_id + '_detail'}>
                        <td colSpan={7} className="shopee-detail-cell">
                          <table className="shopee-detail-table">
                            <thead>
                              <tr>
                                <th>SKU</th>
                                <th>ชื่อสินค้า</th>
                                <th className="text-right">ราคา</th>
                                <th className="text-right">จำนวน</th>
                              </tr>
                            </thead>
                            <tbody>
                              {order.items.map((item, i) => (
                                <tr key={i}>
                                  <td><span className="shopee-detail-sku">{item.sku}</span></td>
                                  <td>{item.product_name}</td>
                                  <td className="text-right">{fmt(item.price)}</td>
                                  <td className="text-right">{item.qty}</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </td>
                      </tr>
                    )}
                  </>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {step === 'confirming' && (
        <div className="shopee-processing">กำลังส่งข้อมูลไป SML... กรุณารอสักครู่</div>
      )}

      {step === 'done' && results && (
        <>
          <div className="shopee-summary-row">
            <SummaryCard label="สำเร็จ" value={results.success_count} colorClass="shopee-result-success" />
            <SummaryCard label="ล้มเหลว" value={results.fail_count} colorClass="shopee-result-fail" />
            <SummaryCard label="ทั้งหมด" value={results.results.length} colorClass="shopee-result-total" />
          </div>
          <div className="shopee-result-actions">
            <button type="button" className="btn btn-primary" onClick={() => { setStep('idle'); setPreview(null); setResults(null) }}>
              นำเข้าไฟล์ใหม่
            </button>
            <a href="/bills?source=shopee" className="btn btn-secondary">ดูบิล Shopee ทั้งหมด →</a>
          </div>
          <div className="shopee-table-wrap">
            <table className="shopee-table">
              <thead>
                <tr><th>Order ID</th><th>ผล</th><th>Doc No / ข้อความ</th></tr>
              </thead>
              <tbody>
                {results.results.map((r) => (
                  <tr key={r.order_id}>
                    <td><span className="shopee-detail-sku">{r.order_id}</span></td>
                    <td>
                      <span className={r.success ? 'shopee-result-ok' : 'shopee-result-fail-text'}>
                        {r.success ? 'สำเร็จ' : 'ล้มเหลว'}
                      </span>
                    </td>
                    <td>{r.doc_no ?? r.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}
