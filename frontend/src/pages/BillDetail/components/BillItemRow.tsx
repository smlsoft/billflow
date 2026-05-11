import { useEffect, useRef, useState } from 'react'
import { AlertCircle, Check, Edit, Trash2, X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { TableRow, TableCell } from '@/components/ui/table'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { cn } from '@/lib/utils'
import api from '@/api/client'
import type { BillItem } from '@/types'
import { useMatchInfo } from '../hooks/useMatchInfo'
import { scoreStyle } from '../utils/formatters'
import { rowIssueReason } from '../utils/validation'
import { MapItemModal } from './MapItemModal'

interface Props {
  item: BillItem
  billId: string
  editable: boolean
  onUpdated: (updated: BillItem) => void
  onDeleted: (itemId: string) => void
  // When true, briefly flash this row (1.5s) so the admin's eye lands on
  // it. Triggered by the BillTotal warning card's "ดู →" link.
  highlighted?: boolean
  rawNameLabel?: string
}

function MatchBadge({ score }: { score: number | null }) {
  const s = scoreStyle(score)
  const tooltip =
    score == null
      ? 'รายการนี้ถูกเลือกหรือพิมพ์เอง'
      : `ความใกล้เคียงกับสินค้าใน SML: ${s.label}`
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

function IssueBadge({ reason }: { reason: string }) {
  if (!reason) return null
  return (
    <span className="mt-2 inline-flex max-w-full items-center gap-1.5 rounded-md border border-warning/30 bg-warning/10 px-2 py-1 text-xs font-medium text-warning">
      <AlertCircle className="h-3.5 w-3.5 shrink-0" />
      <span className="break-words">{reason}</span>
    </span>
  )
}

export function BillItemRow({
  item,
  billId,
  editable,
  onUpdated,
  onDeleted,
  highlighted,
  rawNameLabel = 'ชื่อสินค้าจากต้นทาง',
}: Props) {
  // When the parent flips `highlighted` true (admin clicked "ดู →" in the
  // BillTotal warning card) we scroll this row into view + add a brief tint
  // ring so the admin's eye lands on the right place. Self-clearing flag.
  const rowRef = useRef<HTMLTableRowElement>(null)
  const [flashing, setFlashing] = useState(false)
  useEffect(() => {
    if (!highlighted) return
    rowRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    setFlashing(true)
    const t = setTimeout(() => setFlashing(false), 1500)
    return () => clearTimeout(t)
  }, [highlighted])

  // Per-row validation reason — concatenates each rule the row violates.
  // Empty string when the row is fine; the indicator cell stays empty.
  const issueReason = rowIssueReason(item)
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
        <TableRow
          ref={rowRef}
          className={cn(
            'transition-colors',
            flashing && 'bg-warning/15 ring-2 ring-warning/40',
          )}
        >
          <TableCell className="max-w-[360px] align-top">
            <div className="break-words text-sm leading-6 text-foreground">
              {item.raw_name}
            </div>
            {item.source_sku && (
              <div className="mt-1 text-[11px] text-muted-foreground">
                SKU ต้นทาง: <code className="font-mono">{item.source_sku}</code>
                {!item.item_code && <span className="text-warning"> · ยังไม่พบในสินค้า SML</span>}
              </div>
            )}
            <IssueBadge reason={issueReason} />
          </TableCell>
          <TableCell>
            {item.item_code ? (
              <code className="font-mono text-xs text-foreground">{item.item_code}</code>
            ) : (
              <span className="text-muted-foreground">—</span>
            )}
          </TableCell>
          <TableCell className="max-w-[300px] break-words text-sm">
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
                title={`ราคาใน SML ฿${catalogPrice.toLocaleString()} — ต่างจากบิล ${Math.round((Math.abs(billPrice - catalogPrice) / catalogPrice) * 100)}%`}
              >
                ราคา SML ฿{catalogPrice.toLocaleString()}
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
                {item.item_code ? 'แก้ไข' : 'จับคู่'}
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
          sourceImageUrl={item.source_image_url}
          rawNameLabel={rawNameLabel}
          onPick={(code, unit) =>
            setDraft((d) => ({ ...d, item_code: code, unit_code: unit || d.unit_code }))
          }
          onClose={() => setShowMapModal(false)}
        />
      )}
      <TableRow className="bg-muted/20 hover:bg-muted/20">
        <TableCell colSpan={9} className="p-3">
          <div className="rounded-lg border border-border bg-card p-4 shadow-sm">
            <div className="grid gap-4 xl:grid-cols-[minmax(260px,1fr)_minmax(360px,1.15fr)_420px]">
              <div className="space-y-2">
                <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {rawNameLabel}
                </p>
                <p className="break-words text-sm font-medium leading-6 text-foreground">
                  {item.raw_name}
                </p>
                <IssueBadge reason={issueReason} />
              </div>

              <div className="space-y-3">
                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-muted-foreground">
                    สินค้าใน SML
                  </label>
                  <Button
                    type="button"
                    variant="outline"
                    className="h-10 w-full justify-start gap-2 px-3 text-left"
                    onClick={() => setShowMapModal(true)}
                    title="เปิดเพื่อค้นหาหรือสร้างสินค้าใหม่"
                  >
                    <span className="font-mono text-xs">
                      {draft.item_code || 'เลือกสินค้า'}
                    </span>
                    {matchInfo.itemName && (
                      <span className="truncate text-sm font-normal text-muted-foreground">
                        {matchInfo.itemName}
                      </span>
                    )}
                  </Button>
                </div>
                <div className="flex items-center gap-2">
                  <MatchBadge score={matchInfo.score} />
                  <span className="text-xs text-muted-foreground">
                    ระบบจะจดจำคู่จับคู่นี้หลังบันทึก
                  </span>
                </div>
              </div>

              <div className="grid grid-cols-3 gap-3">
                <label className="space-y-1.5">
                  <span className="text-xs font-medium text-muted-foreground">จำนวน</span>
                  <Input
                    type="number"
                    step="any"
                    value={draft.qty}
                    onChange={(e) => setDraft((d) => ({ ...d, qty: e.target.value }))}
                    className="h-10 text-right"
                  />
                </label>
                <label className="space-y-1.5">
                  <span className="text-xs font-medium text-muted-foreground">หน่วย</span>
                  <Input
                    value={draft.unit_code}
                    onChange={(e) => setDraft((d) => ({ ...d, unit_code: e.target.value }))}
                    className="h-10"
                  />
                </label>
                <label className="space-y-1.5">
                  <span className="text-xs font-medium text-muted-foreground">ราคา</span>
                  <Input
                    type="number"
                    step="any"
                    value={draft.price}
                    onChange={(e) => setDraft((d) => ({ ...d, price: e.target.value }))}
                    className="h-10 text-right"
                  />
                </label>
                <div className="col-span-3 flex items-center justify-between rounded-md bg-muted/50 px-3 py-2">
                  <span className="text-xs font-medium text-muted-foreground">รวมรายการนี้</span>
                  <span className="tabular-nums text-sm font-semibold text-foreground">
                    ฿{(Number(draft.qty || 0) * Number(draft.price || 0)).toLocaleString()}
                  </span>
                </div>
                <div className="col-span-3 flex justify-end gap-2">
                  <Button
                    type="button"
                    variant="ghost"
                    disabled={saving}
                    onClick={() => setEditing(false)}
                  >
                    <X className="h-4 w-4" />
                    ยกเลิก
                  </Button>
                  <Button
                    type="button"
                    disabled={saving}
                    onClick={handleSave}
                  >
                    <Check className="h-4 w-4" />
                    {saving ? 'กำลังบันทึก...' : 'บันทึก'}
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </TableCell>
      </TableRow>
    </>
  )
}
