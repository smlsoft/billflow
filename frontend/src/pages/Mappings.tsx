import { useEffect, useState } from 'react'
import client from '../api/client'
import type { Mapping, MappingStats } from '../types'
import LearningProgress from '../components/LearningProgress'
import toast from 'react-hot-toast'
import './Mappings.css'

const EditIcon = () => (
  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
    <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
  </svg>
)

const TrashIcon = () => (
  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <polyline points="3,6 5,6 21,6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
    <path d="M10 11v6"/><path d="M14 11v6"/>
    <path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/>
  </svg>
)

const CheckIcon = () => (
  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
    <polyline points="20,6 9,17 4,12"/>
  </svg>
)

const XIcon = () => (
  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
    <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
  </svg>
)

interface NewMapping {
  raw_name: string
  item_code: string
  unit_code: string
}

export default function Mappings() {
  const [mappings, setMappings] = useState<Mapping[]>([])
  const [stats, setStats] = useState<MappingStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [editId, setEditId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [newMapping, setNewMapping] = useState<NewMapping>({ raw_name: '', item_code: '', unit_code: '' })
  const [adding, setAdding] = useState(false)

  const fetchAll = async () => {
    setLoading(true)
    try {
      const [mRes, sRes] = await Promise.all([
        client.get<{ data: Mapping[] }>('/api/mappings'),
        client.get<MappingStats>('/api/mappings/stats'),
      ])
      setMappings(mRes.data.data ?? [])
      setStats(sRes.data)
    } catch {
      toast.error('โหลด mapping ไม่สำเร็จ')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchAll() }, [])

  const handleSave = async (id: string) => {
    try {
      await client.put(`/api/mappings/${id}`, { mapped_name: editName })
      setEditId(null)
      fetchAll()
      toast.success('บันทึกสำเร็จ')
    } catch {
      toast.error('บันทึกไม่สำเร็จ')
    }
  }

  const handleDelete = async (id: string) => {
    if (!window.confirm('ลบ mapping นี้?')) return
    try {
      await client.delete(`/api/mappings/${id}`)
      fetchAll()
      toast.success('ลบสำเร็จ')
    } catch {
      toast.error('ลบไม่สำเร็จ')
    }
  }

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newMapping.raw_name || !newMapping.item_code) return
    setAdding(true)
    try {
      await client.post('/api/mappings', newMapping)
      setNewMapping({ raw_name: '', item_code: '', unit_code: '' })
      fetchAll()
      toast.success('เพิ่ม mapping สำเร็จ')
    } catch {
      toast.error('เพิ่ม mapping ไม่สำเร็จ')
    } finally {
      setAdding(false)
    }
  }

  return (
    <div>
      <div className="mappings-header">
        <div>
          <h1 className="mappings-title">Mapping สินค้า</h1>
          <p className="mappings-subtitle">จัดการการจับคู่ชื่อสินค้ากับรหัสใน SML (F1 Learning)</p>
        </div>
      </div>

      <div className="mappings-layout">
        {/* Table */}
        <div>
          {loading ? (
            <div className="mappings-table-wrap">
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="skeleton" style={{ height: 46, margin: '1px 0' }} />
              ))}
            </div>
          ) : (
            <div className="mappings-table-wrap">
              <table className="mappings-table">
                <thead>
                  <tr>
                    <th>ชื่อดิบ (Raw Name)</th>
                    <th>Item Code</th>
                    <th>หน่วย</th>
                    <th className="text-center">ใช้งาน</th>
                    <th className="text-center">จัดการ</th>
                  </tr>
                </thead>
                <tbody>
                  {mappings.map((m) => (
                    <tr key={m.id}>
                      <td><span className="mappings-raw-name">{m.raw_name}</span></td>
                      <td>
                        {editId === m.id ? (
                          <input
                            className="form-input"
                            value={editName}
                            onChange={(e) => setEditName(e.target.value)}
                            autoFocus
                          />
                        ) : (
                          <span className="mappings-mapped-name">{m.mapped_name}</span>
                        )}
                      </td>
                      <td>{m.unit || '—'}</td>
                      <td className="text-center">
                        <span className="mappings-usage">{m.usage_count}</span>
                      </td>
                      <td className="text-center">
                        <div className="mappings-actions">
                          {editId === m.id ? (
                            <>
                              <button type="button" className="btn btn-primary btn-sm btn-icon" onClick={() => handleSave(m.id)} title="บันทึก">
                                <CheckIcon />
                              </button>
                              <button type="button" className="btn btn-secondary btn-sm btn-icon" onClick={() => setEditId(null)} title="ยกเลิก">
                                <XIcon />
                              </button>
                            </>
                          ) : (
                            <>
                              <button type="button" className="btn btn-ghost btn-sm btn-icon" onClick={() => { setEditId(m.id); setEditName(m.mapped_name) }} title="แก้ไข">
                                <EditIcon />
                              </button>
                              <button type="button" className="btn btn-ghost btn-sm btn-icon" onClick={() => handleDelete(m.id)} title="ลบ">
                                <TrashIcon />
                              </button>
                            </>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                  {mappings.length === 0 && (
                    <tr>
                      <td colSpan={5} className="mappings-table-empty">ยังไม่มี mapping</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {/* Sidebar */}
        <div className="mappings-sidebar">
          {/* Add form */}
          <div className="mappings-add-card">
            <div className="mappings-add-card-header">เพิ่ม Mapping ใหม่</div>
            <form className="mappings-add-card-body" onSubmit={handleAdd} aria-label="เพิ่ม mapping ใหม่">
              <div className="mappings-add-field">
                <label className="mappings-add-label">ชื่อดิบ</label>
                <input
                  className="form-input"
                  placeholder="ชื่อสินค้าจาก LINE / Email"
                  value={newMapping.raw_name}
                  onChange={(e) => setNewMapping((p) => ({ ...p, raw_name: e.target.value }))}
                  required
                />
              </div>
              <div className="mappings-add-field">
                <label className="mappings-add-label">Item Code (SML)</label>
                <input
                  className="form-input"
                  placeholder="เช่น CEM001"
                  value={newMapping.item_code}
                  onChange={(e) => setNewMapping((p) => ({ ...p, item_code: e.target.value }))}
                  required
                />
              </div>
              <div className="mappings-add-field">
                <label className="mappings-add-label">หน่วย</label>
                <input
                  className="form-input"
                  placeholder="เช่น ถุง, เส้น"
                  value={newMapping.unit_code}
                  onChange={(e) => setNewMapping((p) => ({ ...p, unit_code: e.target.value }))}
                />
              </div>
              <button type="submit" className="btn btn-primary" disabled={adding}>
                {adding ? 'กำลังเพิ่ม...' : 'เพิ่ม Mapping'}
              </button>
            </form>
          </div>

          {/* F1 stats */}
          {stats && <LearningProgress stats={stats} />}
        </div>
      </div>
    </div>
  )
}
