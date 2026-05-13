import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
} from '@/components/ui/table'
import type { Bill, BillItem } from '@/types'
import { isShopeeSalesBill } from '@/lib/shopeeBill'
import { BillItemRow } from './BillItemRow'

interface Props {
  bill: Bill
  canEdit: boolean
  onItemUpdated: (updated: BillItem) => void
  onItemDeleted: (itemId: string) => void
  // BillTotal's "ดู →" link sets this to the offending item id; the matching
  // row briefly flashes (1.5s) so admin's eye is drawn to the right place
  // even when the items list is long.
  highlightItemId?: string | null
}

export function BillItemsTable({
  bill,
  canEdit,
  onItemUpdated,
  onItemDeleted,
  highlightItemId,
}: Props) {
  const items = bill.items ?? []
  const rawNameLabel = isShopeeSalesBill(bill) ? 'ชื่อสินค้าจาก Excel' : 'ชื่อสินค้าจากอีเมล'
  const issueCount = items.filter((item) => {
    return (
      !item.item_code ||
      !item.unit_code ||
      !item.qty ||
      item.qty <= 0 ||
      item.price == null ||
      item.price <= 0
    )
  }).length

  return (
    <Card className="rounded-2xl border-border/70 shadow-sm">
      <CardHeader className="flex flex-row items-start justify-between gap-3 pb-3">
        <div>
          <CardTitle className="text-sm font-semibold">
            รายการสินค้า ({items.length} รายการ)
          </CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">
            ตรวจรหัสสินค้า หน่วย จำนวน และราคาให้ครบก่อนส่งเข้า SML
          </p>
        </div>
        {issueCount > 0 ? (
          <span className="rounded-md bg-warning/10 px-2 py-1 text-xs font-medium text-warning">
            ต้องแก้ {issueCount} รายการ
          </span>
        ) : items.length > 0 ? (
          <span className="rounded-md bg-success/10 px-2 py-1 text-xs font-medium text-success">
            พร้อมส่ง
          </span>
        ) : null}
      </CardHeader>
      <CardContent className="p-0">
        <div className="overflow-x-auto">
          <Table className="min-w-[1080px]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-[360px]">{rawNameLabel}</TableHead>
                <TableHead className="w-[220px]">รหัสสินค้า SML</TableHead>
                <TableHead className="w-[300px]">ชื่อสินค้าใน SML</TableHead>
                <TableHead className="w-[130px] text-center">ความมั่นใจ</TableHead>
                <TableHead className="w-[110px] text-right">จำนวน</TableHead>
                <TableHead className="w-[120px]">หน่วย</TableHead>
                <TableHead className="w-[140px] text-right">ราคา</TableHead>
                <TableHead className="w-[140px] text-right">รวม</TableHead>
                {canEdit && <TableHead className="w-[170px] text-center">จัดการ</TableHead>}
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <BillItemRow
                  key={item.id}
                  item={item}
                  billId={bill.id}
                  editable={canEdit}
                  onUpdated={onItemUpdated}
                  onDeleted={onItemDeleted}
                  highlighted={item.id === highlightItemId}
                  rawNameLabel={rawNameLabel}
                />
              ))}
              {items.length === 0 && (
                <TableRow>
                  <td
                    colSpan={canEdit ? 9 : 8}
                    className="py-8 text-center text-sm text-muted-foreground"
                  >
                    ยังไม่มีรายการสินค้า
                  </td>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>

      </CardContent>
    </Card>
  )
}
