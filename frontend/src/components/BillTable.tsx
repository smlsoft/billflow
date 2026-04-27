import type { Bill } from '../types'
import BillStatusBadge from './BillStatusBadge'
import dayjs from 'dayjs'
import './BillTable.css'

const SOURCE_LABELS: Record<string, string> = {
  line:           'LINE',
  email:          'Email',
  lazada:         'Lazada',
  shopee:         'Shopee Excel',
  shopee_email:   'Shopee Order',
  shopee_shipped: 'Shopee Shipped',
  manual:         'Manual',
}

interface Props {
  bills: Bill[]
  onRowClick: (id: string) => void
}

export default function BillTable({ bills, onRowClick }: Props) {
  return (
    <div className="bill-table-wrap">
      <table className="bill-table">
        <thead>
          <tr>
            <th>เลขบิล</th>
            <th>Platform</th>
            <th>วันที่</th>
            <th className="text-right">ยอดรวม</th>
            <th className="text-center">สถานะ</th>
          </tr>
        </thead>
        <tbody>
          {bills.map((b) => (
            <tr key={b.id} onClick={() => onRowClick(b.id)}>
              <td>
                {b.sml_doc_no
                  ? <span className="bill-table-doc-no">{b.sml_doc_no}</span>
                  : <span className="bill-table-doc-id">{b.id.slice(0, 8)}…</span>
                }
                {b.bill_type === 'purchase' && (
                  <span
                    title="Purchase Order"
                    style={{
                      marginLeft: 6, padding: '1px 6px', borderRadius: 4,
                      background: '#fef3c7', color: '#92400e',
                      fontSize: '0.7rem', fontWeight: 600,
                    }}
                  >
                    PO
                  </span>
                )}
              </td>
              <td>
                <span className="bill-source-badge">
                  {SOURCE_LABELS[b.source] ?? b.source}
                </span>
              </td>
              <td>{dayjs(b.created_at).format('DD/MM/YY HH:mm')}</td>
              <td className="text-right">
                <span className="bill-table-amount">
                  ฿{(b.total_amount ?? 0).toLocaleString()}
                </span>
              </td>
              <td className="text-center">
                <BillStatusBadge status={b.status} />
              </td>
            </tr>
          ))}
          {bills.length === 0 && (
            <tr>
              <td colSpan={5} className="bill-table-empty">ไม่พบรายการ</td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}
