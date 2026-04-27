import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useBills } from '../hooks/useBills'
import BillTable from '../components/BillTable'
import './Bills.css'

const PER_PAGE = 20

const STATUS_OPTIONS = [
  { value: '', label: 'ทุกสถานะ' },
  { value: 'pending', label: 'รอดำเนินการ' },
  { value: 'needs_review', label: 'รอตรวจสอบ' },
  { value: 'confirmed', label: 'ยืนยันแล้ว' },
  { value: 'sent', label: 'SML สำเร็จ' },
  { value: 'failed', label: 'ล้มเหลว' },
]

const SearchIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
  </svg>
)

export default function Bills() {
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState('')
  const [search, setSearch] = useState('')
  const { data, loading } = useBills({ page, per_page: PER_PAGE, status, search })

  const totalPages = data ? Math.ceil(data.total / PER_PAGE) : 1
  const hasMore = data ? page * PER_PAGE < data.total : false

  const handleSearch = (val: string) => {
    setSearch(val)
    setPage(1)
  }

  const handleStatus = (val: string) => {
    setStatus(val)
    setPage(1)
  }

  return (
    <div>
      <div className="bills-header">
        <div>
          <h1 className="bills-title">รายการบิล</h1>
          <p className="bills-subtitle">ติดตามและจัดการบิลทั้งหมดในระบบ</p>
        </div>
      </div>

      <div className="bills-filter-bar">
        <div className="bills-search-wrap">
          <span className="bills-search-icon"><SearchIcon /></span>
          <input
            className="bills-search-input"
            placeholder="ค้นหาผู้ขาย / เลขบิล..."
            value={search}
            onChange={(e) => handleSearch(e.target.value)}
          />
        </div>
        <select
          className="form-select"
          value={status}
          onChange={(e) => handleStatus(e.target.value)}
        >
          {STATUS_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
      </div>

      {loading ? (
        <div className="bills-loading">
          {Array.from({ length: 8 }).map((_, i) => (
            <div key={i} className="skeleton bills-loading-row" />
          ))}
        </div>
      ) : (
        <>
          <BillTable bills={data?.data ?? []} onRowClick={(id) => navigate(`/bills/${id}`)} />
          <div className="bills-footer">
            <span className="bills-total-label">ทั้งหมด {data?.total ?? 0} รายการ</span>
            <div className="bills-pagination">
              <button
                className="btn btn-secondary btn-sm"
                disabled={page <= 1}
                onClick={() => setPage((p) => p - 1)}
              >
                ก่อนหน้า
              </button>
              <span className="bills-page-label">หน้า {page} / {totalPages}</span>
              <button
                className="btn btn-secondary btn-sm"
                disabled={!hasMore}
                onClick={() => setPage((p) => p + 1)}
              >
                ถัดไป
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
