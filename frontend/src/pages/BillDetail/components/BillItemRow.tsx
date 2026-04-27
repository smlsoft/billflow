import { useState } from 'react'
import { Edit, Check, X, Trash2 } from 'lucide-react'
import toast from 'react-hot-toast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { TableRow, TableCell } from '@/components/ui/table'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { cn } from '@/lib/utils'
import api from '@/api/client'
import type { BillItem } from '@/types'
import { useMatchInfo } from '../hooks/useMatchInfo'
import { scoreStyle } from '../utils/formatters'
import { MapItemModal } from './MapItemModal'

interface Props {
  item: BillItem
  billId: string
  editable: boolean
  onUpdated: (updated: BillItem) => void
  onDeleted: (itemId: string) => void
}

function MatchBadge({ score }: { score: number | null }) {
  const s = scoreStyle(score)
  const tooltip =
    score == null
      ? 'รหัสนี้ไม่อยู่ใน top-5 catalog candidates ที่ระบบหาให้ — น่าจะแก้ผ่าน MapItemModal'
      : `ความใกล้เคียงกับ catalog (จาก embedding cosine similarity): ${s.label}`
  return (
    <span
      title={tooltip}
      className={cn(
        'inline-flex items-center gap-1 rounded-full px-2 py-0.5',
        'text-xs font-semibold whitespace-nowrap',
        s.bg,
        s.color,
      )}
    >
      <span>{s.icon}</span>
      <span>{s.label}</span>
    </span>
  )
}

export function BillItemRow({ item, billId, editable, onUpdated, onDeleted }: Props) {
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [showMapModal, setShowMapModal] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [draft, setDraft] = useState({
    item_code: item.item_code ?? '',
    unit_code: item.unit_code ?? '',
    qty: String(item.qty ?? 0),
    price: String(item.price ?? 0),
  })

  const reset = () =>
    setDraft({
      item_code: item.item_code ?? '',
      unit_code: item.unit_code ?? '',
      qty: String(item.qty ?? 0),
      price: String(item.price ?? 0),
    })

  const handleSave = async () => {
    setSaving(true)
    try {
      await api.put(`/api/bills/${billId}/items/${item.id}`, {
        item_code: draft.item_code,
        unit_code: draft.unit_code,
        qty: Number(draft.qty),
        price: Number(draft.price),
      })

      // F1 learning: backend registers ai_learned mapping if item_code changed.
      const prevCode = item.item_code ?? ''
      if (draft.item_code && draft.item_code !== prevCode) {
        toast.success('✓ จดจำการจับคู่นี้แล้ว — ครั้งถัดไประบบจะ map ให้อัตโนมัติ', {
          duration: 3500,
        })
      }

      onUpdated({
        ...item,
        item_code: draft.item_code,
        unit_code: draft.unit_code,
        qty: Number(draft.qty),
        price: Number(draft.price),
        mapped: draft.item_code !== '',
      })
      setEditing(false)
    } catch (err) {
      console.error('update item failed', err)
      toast.error('บันทึกไม่สำเร็จ')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    await api.delete(`/api/bills/${billId}/items/${item.id}`)
    onDeleted(item.id)
  }

  const matchInfo = useMatchInfo(item)
  const billPrice = item.price ?? 0
  const catalogPrice = matchInfo.catalogPrice ?? 0
  const priceMismatch =
    billPrice > 0 &&
    catalogPrice > 0 &&
    Math.abs(billPrice - catalogPrice) / catalogPrice > 0.3

  if (!editing) {
    return (
      <>
        <TableRow>
          <TableCell className="max-w-[280px] break-words">{item.raw_name}</TableCell>
          <TableCell>
            {item.item_code ? (
              <code className="font-mono text-xs text-foreground">{item.item_code}</code>
            ) : (
              <span className="text-muted-foreground">—</span>
            )}
          </TableCell>
          <TableCell className="max-w-[260px] break-words text-sm">
            <span className={matchInfo.itemName ? 'text-foreground' : 'text-muted-foreground'}>
              {matchInfo.itemName ?? '—'}
            </span>
          </TableCell>
          <TableCell className="text-center">
            <MatchBadge score={matchInfo.score} />
          </TableCell>
          <TableCell className="text-right tabular-nums">{item.qty}</TableCell>
          <TableCell>{item.unit_code || '—'}</TableCell>
          <TableCell className="text-right tabular-nums font-medium">
            ฿{(item.price ?? 0).toLocaleString()}
            {priceMismatch && (
              <div
                className="text-[11px] text-amber-700 mt-0.5"
                title={`Catalog ราคา ฿${catalogPrice.toLocaleString()} — ต่างจากบิล ${Math.round((Math.abs(billPrice - catalogPrice) / catalogPrice) * 100)}%`}
              >
                ⚠ catalog ฿{catalogPrice.toLocaleString()}
              </div>
            )}
          </TableCell>
          <TableCell className="text-right tabular-nums font-medium">
            ฿{((item.qty ?? 0) * (item.price ?? 0)).toLocaleString()}
          </TableCell>
          {editable && (
            <TableCell className="text-center whitespace-nowrap">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 px-2"
                onClick={() => {
                  reset()
                  setEditing(true)
                }}
              >
                <Edit className="h-3.5 w-3.5" />
                แก้ไข
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-destructive hover:text-destructive"
                onClick={() => setDeleteOpen(true)}
                title="ลบรายการ"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </TableCell>
          )}
        </TableRow>

        <ConfirmDialog
          open={deleteOpen}
          onOpenChange={setDeleteOpen}
          title="ลบรายการสินค้า"
          description={`ยืนยันลบ "${item.raw_name.slice(0, 50)}${item.raw_name.length > 50 ? '...' : ''}" ?`}
          confirmLabel="ลบรายการ"
          variant="destructive"
          onConfirm={handleDelete}
        />
      </>
    )
  }

  // ── Edit mode ────────────────────────────────────────────────────────────────
  return (
    <>
      {showMapModal && (
        <MapItemModal
          open={showMapModal}
          rawName={item.raw_name}
          currentCode={draft.item_code}
          currentUnit={draft.unit_code}
          currentPrice={Number(draft.price) || 0}
          onPick={(code, unit) =>
            setDraft((d) => ({ ...d, item_code: code, unit_code: unit || d.unit_code }))
          }
          onClose={() => setShowMapModal(false)}
        />
      )}
      <TableRow className="bg-muted/20">
        <TableCell className="max-w-[280px] break-words text-sm text-muted-foreground">
          {item.raw_name}
        </TableCell>
        <TableCell>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-8 min-w-[140px] justify-start font-mono text-xs"
            onClick={() => setShowMapModal(true)}
            title="เปิดเพื่อค้นหาหรือสร้างสินค้าใหม่"
          >
            {draft.item_code ? (
              draft.item_code
            ) : (
              <span className="text-muted-foreground font-sans">เลือกสินค้า...</span>
            )}
          </Button>
        </TableCell>
        <TableCell className="text-sm text-muted-foreground">
          {matchInfo.itemName ?? '—'}
        </TableCell>
        <TableCell className="text-center">
          <MatchBadge score={matchInfo.score} />
        </TableCell>
        <TableCell className="text-right">
          <Input
            type="number"
            step="any"
            value={draft.qty}
            onChange={(e) => setDraft((d) => ({ ...d, qty: e.target.value }))}
            className="h-8 w-20 text-right"
          />
        </TableCell>
        <TableCell>
          <Input
            value={draft.unit_code}
            onChange={(e) => setDraft((d) => ({ ...d, unit_code: e.target.value }))}
            className="h-8 w-20"
          />
        </TableCell>
        <TableCell className="text-right">
          <Input
            type="number"
            step="any"
            value={draft.price}
            onChange={(e) => setDraft((d) => ({ ...d, price: e.target.value }))}
            className="h-8 w-24 text-right"
          />
        </TableCell>
        <TableCell className="text-right tabular-nums font-medium">
          ฿{(Number(draft.qty || 0) * Number(draft.price || 0)).toLocaleString()}
        </TableCell>
        <TableCell className="text-center whitespace-nowrap">
          <Button
            type="button"
            size="sm"
            className="h-7 px-2"
            disabled={saving}
            onClick={handleSave}
          >
            <Check className="h-3.5 w-3.5" />
            {saving ? '...' : 'บันทึก'}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 px-2"
            disabled={saving}
            onClick={() => setEditing(false)}
          >
            <X className="h-3.5 w-3.5" />
            ยกเลิก
          </Button>
        </TableCell>
      </TableRow>
    </>
  )
}
