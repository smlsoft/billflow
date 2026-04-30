import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
} from '@/components/ui/table'
import type { Bill, BillItem } from '@/types'
import { BillItemRow } from './BillItemRow'
import { AddItemForm } from './AddItemForm'

interface Props {
  bill: Bill
  canEdit: boolean
  onItemUpdated: (updated: BillItem) => void
  onItemDeleted: (itemId: string) => void
  onItemAdded: (item: BillItem) => void
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
  onItemAdded,
  highlightItemId,
}: Props) {
  const items = bill.items ?? []

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-semibold">
          รายการสินค้า ({items.length} รายการ)
        </CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                {/* Tiny first column for validation status icon — shows ⚠
                    when the row blocks SML send (missing item_code etc.) */}
                <TableHead className="w-6 px-1" aria-label="status" />
                <TableHead className="w-[220px]">ชื่อสินค้า (จาก source)</TableHead>
                <TableHead>Item Code</TableHead>
                <TableHead>SML Item Name</TableHead>
                <TableHead className="text-center">Match</TableHead>
                <TableHead className="text-right">จำนวน</TableHead>
                <TableHead>หน่วย</TableHead>
                <TableHead className="text-right">ราคา/หน่วย</TableHead>
                <TableHead className="text-right">รวม</TableHead>
                {canEdit && <TableHead className="text-center">จัดการ</TableHead>}
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
                />
              ))}
              {items.length === 0 && (
                <TableRow>
                  <td
                    colSpan={canEdit ? 10 : 9}
                    className="py-8 text-center text-sm text-muted-foreground"
                  >
                    ยังไม่มีรายการสินค้า
                  </td>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>

        {canEdit && (
          <div className="px-4 pb-4">
            <AddItemForm billId={bill.id} onAdded={onItemAdded} />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
