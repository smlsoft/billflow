import { useState, useCallback } from 'react'
import { useDropzone } from 'react-dropzone'
import client from '../api/client'
import type { BillPreview, ImportConfirmResponse } from '../types'
import './Import.css'

type Step = 'idle' | 'uploading' | 'preview' | 'confirming' | 'result'

function AnomalyBadges({ anomalies, hasBlock }: { anomalies: BillPreview['anomalies']; hasBlock: boolean }) {
  if (!anomalies?.length && !hasBlock) return null
  return (
    <span className="import-anomaly-list">
      {anomalies?.map((a, i) => (
        <span
          key={i}
          title={a.message}
          className={`import-anomaly-badge ${a.severity === 'block' ? 'import-anomaly-badge--block' : 'import-anomaly-badge--warn'}`}
        >
          {a.severity === 'block' ? '🔴' : '🟡'} {a.code}
        </span>
      ))}
    </span>
  )
}

export default function Import() {
  const [step, setStep] = useState<Step>('idle')
  const [platform, setPlatform] = useState<'lazada' | 'shopee'>('lazada')
  const [billType, setBillType] = useState<'sale' | 'purchase'>('sale')
  const [bills, setBills] = useState<BillPreview[]>([])
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [result, setResult] = useState<ImportConfirmResponse | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  const toggleSelect = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleAll = () => {
    const confirmable = bills.filter((b) => !b.has_block).map((b) => b.bill_id)
    if (selectedIds.size === confirmable.length) setSelectedIds(new Set())
    else setSelectedIds(new Set(confirmable))
  }

  const onDrop = useCallback(async (files: File[]) => {
    if (!files.length) return
    setStep('uploading')
    setErrorMsg(null)
    try {
      const form = new FormData()
      form.append('file', files[0])
      form.append('platform', platform)
      form.append('bill_type', billType)
      const res = await client.post('/api/import/upload', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      const data = res.data
      setBills(data.bills || [])
      const preselected = (data.bills as BillPreview[]).filter((b) => !b.has_block).map((b) => b.bill_id)
      setSelectedIds(new Set(preselected))
      setStep('preview')
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } }
      setErrorMsg(err?.response?.data?.error ?? 'อัปโหลดไม่สำเร็จ')
      setStep('idle')
    }
  }, [platform, billType])

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    accept: {
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet': ['.xlsx'],
      'application/vnd.ms-excel': ['.xls'],
    },
    maxFiles: 1,
    disabled: step === 'uploading',
  })

  const handleConfirm = async () => {
    if (selectedIds.size === 0) return
    setStep('confirming')
    try {
      const res = await client.post<ImportConfirmResponse>('/api/import/confirm', {
        bill_ids: Array.from(selectedIds),
      })
      setResult(res.data)
      setStep('result')
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } }
      setErrorMsg(err?.response?.data?.error ?? 'ยืนยันไม่สำเร็จ')
      setStep('preview')
    }
  }

  const reset = () => {
    setStep('idle'); setBills([]); setSelectedIds(new Set())
    setResult(null); setErrorMsg(null)
  }

  const confirmable = bills.filter((b) => !b.has_block)
  const blocked = bills.filter((b) => b.has_block)

  return (
    <div>
      <div className="import-header">
        <h1 className="import-title">นำเข้าบิล — Lazada</h1>
        <p className="import-subtitle">อัปโหลดไฟล์ Excel เพื่อสร้างบิลเข้าระบบ SML อัตโนมัติ</p>
      </div>

      {/* Step 1: Options + Dropzone */}
      {(step === 'idle' || step === 'uploading') && (
        <>
          <div className="import-options">
            <div className="import-options-field">
              <label className="import-options-label" htmlFor="import-platform">Platform</label>
              <select
                id="import-platform"
                className="form-select"
                value={platform}
                onChange={(e) => setPlatform(e.target.value as 'lazada' | 'shopee')}
                disabled={step === 'uploading'}
              >
                <option value="lazada">Lazada</option>
                <option value="shopee">Shopee</option>
              </select>
            </div>
            <div className="import-options-field">
              <label className="import-options-label" htmlFor="import-bill-type">ประเภทบิล</label>
              <select
                id="import-bill-type"
                className="form-select"
                value={billType}
                onChange={(e) => setBillType(e.target.value as 'sale' | 'purchase')}
                disabled={step === 'uploading'}
              >
                <option value="sale">บิลขาย (Sale)</option>
                <option value="purchase">บิลซื้อ (Purchase)</option>
              </select>
            </div>
          </div>

          <div
            {...getRootProps()}
            className={`import-dropzone${isDragActive ? ' import-dropzone--active' : ''}${step === 'uploading' ? ' import-dropzone--disabled' : ''}`}
          >
            <input {...getInputProps()} />
            {step === 'uploading' ? (
              <p className="import-dropzone-text">กำลังประมวลผล...</p>
            ) : isDragActive ? (
              <p className="import-dropzone-text">วางไฟล์ที่นี่</p>
            ) : (
              <>
                <div className="import-dropzone-icon">📊</div>
                <p className="import-dropzone-text">ลากไฟล์ Excel มาวาง หรือคลิกเพื่อเลือก</p>
                <p className="import-dropzone-hint">รองรับ .xlsx, .xls</p>
              </>
            )}
          </div>

          {errorMsg && <div className="alert alert-danger import-alert--top">{errorMsg}</div>}
        </>
      )}

      {/* Step 2: Preview */}
      {step === 'preview' && (
        <>
          <div className="import-preview-bar">
            <p className="import-preview-count">
              <strong>{bills.length}</strong> ออเดอร์จากไฟล์ &nbsp;
              <span className="import-preview-ok">พร้อมยืนยัน {confirmable.length}</span>
              {blocked.length > 0 && <span className="import-preview-blocked">บล็อก {blocked.length}</span>}
            </p>
            <div className="import-preview-actions">
              <button type="button" className="btn btn-secondary btn-sm" onClick={reset}>อัปโหลดใหม่</button>
              <button
                type="button"
                className="btn btn-primary btn-sm"
                onClick={handleConfirm}
                disabled={selectedIds.size === 0}
              >
                ยืนยัน {selectedIds.size} ออเดอร์
              </button>
            </div>
          </div>

          {errorMsg && <div className="alert alert-danger import-alert--bottom">{errorMsg}</div>}

          <div className="import-table-wrap">
            <table className="import-table">
              <thead>
                <tr>
                  <th>
                    <input
                      type="checkbox"
                      checked={selectedIds.size === confirmable.length && confirmable.length > 0}
                      onChange={toggleAll}
                    />
                  </th>
                  <th>หมายเลขออเดอร์</th>
                  <th>ชื่อลูกค้า</th>
                  <th className="text-center">รายการ</th>
                  <th className="text-center">จับคู่</th>
                  <th className="text-right">ยอดรวม</th>
                  <th>Anomaly</th>
                </tr>
              </thead>
              <tbody>
                {bills.map((bill) => (
                  <tr key={bill.bill_id} className={bill.has_block ? 'import-table tbody tr--blocked' : selectedIds.has(bill.bill_id) ? 'import-table tbody tr--selected' : ''}>
                    <td>
                      <input
                        type="checkbox"
                        checked={selectedIds.has(bill.bill_id)}
                        disabled={bill.has_block}
                        onChange={() => toggleSelect(bill.bill_id)}
                      />
                    </td>
                    <td><span className="import-order-id">{bill.order_id || '—'}</span></td>
                    <td>{bill.customer_name}</td>
                    <td className="text-center">{bill.item_count}</td>
                    <td className="text-center">
                      <span className={`import-map-ratio ${bill.mapped_count < bill.item_count ? 'import-map-ratio--partial' : 'import-map-ratio--full'}`}>
                        {bill.mapped_count}/{bill.item_count}
                      </span>
                    </td>
                    <td className="text-right">
                      {bill.total_amount > 0 ? `฿${bill.total_amount.toLocaleString('th-TH', { minimumFractionDigits: 2 })}` : '—'}
                    </td>
                    <td><AnomalyBadges anomalies={bill.anomalies} hasBlock={bill.has_block} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {/* Step 3: Confirming */}
      {step === 'confirming' && (
        <div className="import-processing">กำลังส่งไปยัง SML ERP... โปรดรอสักครู่</div>
      )}

      {/* Step 4: Result */}
      {step === 'result' && result && (
        <>
          <div className="import-result-cards">
            <div className="import-result-card import-result-card--success">
              <div className="import-result-card-value">{result.success}</div>
              <div className="import-result-card-label">สำเร็จ</div>
            </div>
            <div className="import-result-card import-result-card--fail">
              <div className="import-result-card-value">{result.failed}</div>
              <div className="import-result-card-label">ล้มเหลว</div>
            </div>
          </div>

          {result.errors?.length > 0 && (
            <>
              <h3 className="import-errors-title">รายการที่ล้มเหลว</h3>
              <div className="import-table-wrap import-table-wrap--mb">
                <table className="import-table">
                  <thead>
                    <tr>
                      <th>Bill ID</th>
                      <th>สาเหตุ</th>
                    </tr>
                  </thead>
                  <tbody>
                    {result.errors.map((e, i) => (
                      <tr key={i}>
                        <td><span className="import-err-bill-id">{e.bill_id}</span></td>
                        <td><span className="import-err-reason">{e.reason}</span></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          <button type="button" className="btn btn-primary" onClick={reset}>นำเข้าไฟล์ใหม่</button>
        </>
      )}
    </div>
  )
}
