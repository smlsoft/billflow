import { useEffect, useRef, useState } from 'react'
import { AlertCircle, AlertTriangle, Check, CheckCircle2, Edit, Info, Maximize2, Trash2, X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { TableRow, TableCell } from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { UnitSelect } from '@/components/common/UnitSelect'
import { cn } from '@/lib/utils'
import { LAZADA_FEE_SOURCE_SKU, SHOPEE_SHIPPING_SOURCE_SKU, isMarketplaceFeeSourceSKU } from '@/lib/shopeeBill'
import api from '@/api/client'
import type { BillItem, CatalogMatch } from '@/types'
import { useMatchInfo } from '../hooks/useMatchInfo'
import { scoreStyle } from '../utils/formatters'
import { rowIssueReason } from '../utils/validation'
import { MapItemModal } from './MapItemModal'

export interface DiscountInfo {
  effectiveDiscount: number  // total = coupon + coin
  couponDiscount: number
  coinAmount?: number
  grossTotal: number         // gross ของทุก item (ไม่รวมค่าส่ง)
  platform?: 'shopee' | 'lazada'
}

interface Props {
  item: BillItem
  billId: string
  editable: boolean
  canDelete: boolean
  onDeleted: (itemId: string) => void
  onRefresh: () => Promise<unknown>
  lockSourceAmounts?: boolean
  // When true, briefly flash this row (1.5s) so the admin's eye lands on
  // it. Triggered by the BillTotal warning card's "ดู →" link.
  highlighted?: boolean
  rawNameLabel?: string
  showDiscountColumn?: boolean
  tableColumnCount?: number
  discountInfo?: DiscountInfo
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
  canDelete,
  onDeleted,
  onRefresh,
  lockSourceAmounts = false,
  highlighted,
  rawNameLabel = 'ชื่อสินค้าจากต้นทาง',
  showDiscountColumn = false,
  tableColumnCount = 9,
  discountInfo,
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
  const [confirming, setConfirming] = useState(false)
  const [showMapModal, setShowMapModal] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [imagePreviewOpen, setImagePreviewOpen] = useState(false)
  const [pickedMatch, setPickedMatch] = useState<CatalogMatch | null>(null)
  const [draft, setDraft] = useState({
    item_code: item.item_code ?? '',
    unit_code: item.unit_code ?? '',
    qty: String(item.qty ?? 0),
    price: String(item.price ?? 0),
  })

  const reset = () => {
    setPickedMatch(null)
    setDraft({
      item_code: item.item_code ?? '',
      unit_code: item.unit_code ?? '',
      qty: String(item.qty ?? 0),
      price: String(item.price ?? 0),
    })
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      const payload: {
        item_code: string
        unit_code: string
        qty?: number
        price?: number
      } = {
        item_code: draft.item_code,
        unit_code: draft.unit_code,
      }
      if (!lockSourceAmounts) {
        payload.qty = Number(draft.qty)
        payload.price = Number(draft.price)
      }
      const { data } = await api.put<{
        mapping_scope?: 'item_only' | 'source_sku' | 'raw_name'
      }>(`/api/bills/${billId}/items/${item.id}`, payload)

      const prevCode = item.item_code ?? ''
      if (draft.item_code && draft.item_code !== prevCode) {
        if (data.mapping_scope === 'source_sku') {
          toast.success('บันทึกการจับคู่สำหรับ SKU ต้นทางนี้แล้ว', {
            description: 'สินค้า/สีอื่นที่ชื่อเหมือนกันจะไม่ถูกเปลี่ยนตาม',
            duration: 3500,
          })
        } else if (data.mapping_scope === 'item_only') {
          toast.success('บันทึกเฉพาะรายการนี้แล้ว', {
            description: 'ชื่อสินค้าซ้ำ ระบบจึงไม่กระจายรหัสไปยังรายการอื่น',
            duration: 3500,
          })
        } else {
          toast.success('จดจำการจับคู่นี้แล้ว', {
            description: 'ครั้งถัดไประบบจะจับคู่ให้อัตโนมัติ',
            duration: 3500,
          })
        }
      }
      await onRefresh()
      setEditing(false)
      setPickedMatch(null)
    } catch (err) {
      console.error('update item failed', err)
      toast.error('บันทึกไม่สำเร็จ')
    } finally {
      setSaving(false)
    }
  }

  const handleQuickConfirm = async () => {
    if (!item.item_code) return
    setConfirming(true)
    try {
      const payload: {
        item_code: string
        unit_code?: string
        qty?: number
        price?: number
      } = {
        item_code: item.item_code,
        unit_code: item.unit_code ?? undefined,
      }
      if (!lockSourceAmounts) {
        payload.qty = item.qty
        payload.price = item.price ?? undefined
      }
      await api.put(`/api/bills/${billId}/items/${item.id}`, payload)
      await onRefresh()
      toast.success('ยืนยันการจับคู่สินค้าแล้ว', {
        description: 'ระบบจะจดจำและเปิดให้ส่ง SML เมื่อทุกรายการยืนยันครบ',
      })
    } catch (err) {
      console.error('confirm item match failed', err)
      toast.error('ยืนยันสินค้าไม่สำเร็จ')
    } finally {
      setConfirming(false)
    }
  }

  const handleDelete = async () => {
    await api.delete(`/api/bills/${billId}/items/${item.id}`)
    onDeleted(item.id)
  }

  const matchInfo = useMatchInfo(item)
  const needsConfirm = Boolean(item.item_code && item.mapped !== true)
  const isShopeeShippingLine = item.source_sku === SHOPEE_SHIPPING_SOURCE_SKU
  const isLazadaFeeLine = item.source_sku === LAZADA_FEE_SOURCE_SKU
  const isMarketplaceFeeLine = isMarketplaceFeeSourceSKU(item.source_sku)
  const sourceItemDescription = item.source_variant
    ? `${item.raw_name} ตัวเลือก ${item.source_variant}`
    : item.raw_name
  const editMatchInfo =
    pickedMatch && pickedMatch.item_code === draft.item_code
      ? {
          itemName: pickedMatch.item_name,
          score: pickedMatch.score,
        }
      : matchInfo
  const billPrice = item.price ?? 0
  const discountAmount = item.discount_amount ?? 0
  const grossAmount = (item.qty ?? 0) * billPrice
  const netAmount = Math.max(grossAmount - discountAmount, 0)
  const editQty = lockSourceAmounts ? item.qty ?? 0 : Number(draft.qty || 0)
  const editPrice = lockSourceAmounts ? item.price ?? 0 : Number(draft.price || 0)

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
            <div className="flex items-start gap-2">
              {item.source_image_url && (
                <button
                  type="button"
                  className="group relative h-12 w-12 shrink-0 overflow-hidden rounded border border-border bg-muted outline-none transition-colors hover:border-primary/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  onClick={() => setImagePreviewOpen(true)}
                  aria-label={`ดูรูปสินค้า ${sourceItemDescription}`}
                  title="ดูรูปสินค้า"
                >
                  <img
                    src={item.source_image_url}
                    alt=""
                    className="h-full w-full object-cover"
                    loading="lazy"
                    referrerPolicy="no-referrer"
                  />
                  <span className="absolute bottom-0.5 right-0.5 rounded bg-background/90 p-0.5 text-muted-foreground opacity-0 shadow-sm transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
                    <Maximize2 className="h-3 w-3" aria-hidden="true" />
                  </span>
                </button>
              )}
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2 break-words text-sm leading-6 text-foreground">
                  <span>{item.raw_name}</span>
                  {isShopeeShippingLine && (
                    <span className="inline-flex rounded-md border border-info/30 bg-info/10 px-2 py-0.5 text-[11px] font-medium text-info">
                      ค่าส่งจาก Shopee
                    </span>
                  )}
                  {isLazadaFeeLine && (
                    <span className="inline-flex rounded-md border border-[#f31c9b]/30 bg-[#f31c9b]/10 px-2 py-0.5 text-[11px] font-medium text-[#9f176b] dark:text-[#ff9bd7]">
                      ค่าส่ง/fee จาก Lazada
                    </span>
                  )}
                </div>
                {item.source_variant && !isMarketplaceFeeLine && (
                  <div className="mt-1 break-words text-[11px] text-muted-foreground">
                    ตัวเลือก: <span className="font-medium text-foreground/80">{item.source_variant}</span>
                  </div>
                )}
                {item.source_sku && !isMarketplaceFeeLine && (
                  <div className="mt-1 text-[11px] text-muted-foreground">
                    SKU ต้นทาง: <code className="font-mono">{item.source_sku}</code>
                    {!item.item_code && <span className="text-warning"> · ยังไม่พบในสินค้า SML</span>}
                  </div>
                )}
                <IssueBadge reason={issueReason} />
              </div>
            </div>
          </TableCell>
          <TableCell>
            {item.item_code ? (
              <div className="space-y-1">
                <code className="font-mono text-xs text-foreground">{item.item_code}</code>
                {item.has_hidden_chars && (
                  <div
                    className="inline-flex max-w-full items-center gap-1 rounded-md border border-warning/30 bg-warning/10 px-2 py-0.5 text-[11px] font-medium text-warning"
                    title={`รหัสนี้มาจาก SML และมีอักขระมองไม่เห็น${item.clean_item_code ? ` ควรเป็น ${item.clean_item_code}` : ''}`}
                  >
                    <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
                    <span className="truncate">รหัสมีอักขระซ่อน</span>
                  </div>
                )}
                {needsConfirm && (
                  <div className="inline-flex rounded-md border border-warning/30 bg-warning/10 px-2 py-0.5 text-[11px] font-medium text-warning">
                    ต้องยืนยัน
                  </div>
                )}
              </div>
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
          </TableCell>
          {showDiscountColumn && (
            <TableCell className="text-right tabular-nums">
              {discountAmount > 0 ? (
                <span className="inline-flex items-center justify-end gap-1">
                  <span className="font-medium text-success">-฿{discountAmount.toLocaleString()}</span>
                  {discountInfo && (
                    <TooltipProvider delayDuration={100}>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Info className="h-3 w-3 shrink-0 cursor-pointer text-muted-foreground/50 hover:text-info transition-colors" />
                        </TooltipTrigger>
                        <TooltipContent side="left" className="max-w-[260px] text-xs leading-relaxed">
                          <p className="font-semibold mb-1">วิธีคำนวณส่วนลด row นี้</p>
                          {(() => {
                            const pct = discountInfo.grossTotal > 0
                              ? (discountInfo.effectiveDiscount / discountInfo.grossTotal * 100)
                              : 0
                            const itemGross = grossAmount
                            const coinAmount = discountInfo.coinAmount ?? 0
                            return (
                              <>
                                <p>ส่วนลดรวม {discountInfo.effectiveDiscount.toLocaleString()} บาท</p>
                                <p className="text-muted-foreground">
                                  = คูปอง ฿{discountInfo.couponDiscount.toLocaleString()}
                                  {coinAmount > 0 && ` + Coin ฿${coinAmount.toLocaleString()}`}
                                </p>
                                <p className="mt-1">
                                  อัตรา = {pct.toFixed(3)}%
                                </p>
                                <p>
                                  ส่วนลด row = {discountInfo.effectiveDiscount.toLocaleString()} × ({itemGross.toLocaleString()} / {discountInfo.grossTotal.toLocaleString()})
                                </p>
                                <p className="font-medium text-success mt-1">
                                  = ฿{discountAmount.toLocaleString()}
                                </p>
                              </>
                            )
                          })()}
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  )}
                </span>
              ) : (
                <span className="text-muted-foreground">—</span>
              )}
            </TableCell>
          )}
          <TableCell className="text-right tabular-nums font-medium">
            ฿{netAmount.toLocaleString()}
          </TableCell>
          {editable && (
            <TableCell className="whitespace-nowrap">
              <div className="flex items-center justify-center gap-1.5">
              {needsConfirm && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-8 gap-1.5 px-2.5 text-success"
                  onClick={handleQuickConfirm}
                  disabled={confirming || !item.unit_code}
                  title={item.unit_code ? 'ยืนยันสินค้านี้โดยไม่ต้องเข้าโหมดแก้ไข' : 'ตั้งหน่วยก่อนยืนยันสินค้า'}
                >
                  <CheckCircle2 className="h-3.5 w-3.5" />
                  {confirming ? 'ยืนยัน...' : 'ยืนยัน'}
                </Button>
              )}
              <Button
                type="button"
                variant={item.item_code ? 'outline' : 'default'}
                size="sm"
                className={cn(
                  'h-8 gap-1.5 px-3 font-medium',
                  item.item_code ? 'border-primary/35 text-primary hover:bg-primary/10 hover:text-primary' : 'shadow-sm',
                )}
                onClick={() => {
                  reset()
                  setEditing(true)
                }}
                title={item.item_code ? 'แก้ไขการจับคู่สินค้า' : 'จับคู่สินค้ากับรหัส SML'}
              >
                <Edit className="h-3.5 w-3.5" />
                {item.item_code ? 'แก้ไข' : 'จับคู่สินค้า'}
              </Button>
              {canDelete && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-8 w-8 px-0 text-destructive hover:text-destructive"
                  onClick={() => setDeleteOpen(true)}
                  title="ลบรายการ"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              )}
              </div>
            </TableCell>
          )}
        </TableRow>

        {canDelete && (
          <ConfirmDialog
            open={deleteOpen}
            onOpenChange={setDeleteOpen}
            title="ลบรายการสินค้า"
            description={`ยืนยันลบ "${item.raw_name.slice(0, 50)}${item.raw_name.length > 50 ? '...' : ''}" ?`}
            confirmLabel="ลบรายการ"
            variant="destructive"
            onConfirm={handleDelete}
          />
        )}
        {item.source_image_url && (
          <Dialog open={imagePreviewOpen} onOpenChange={setImagePreviewOpen}>
            <DialogContent className="max-h-[92vh] max-w-[min(92vw,960px)] overflow-hidden p-0">
              <DialogHeader className="sr-only">
                <DialogTitle>รูปสินค้า</DialogTitle>
                <DialogDescription>{sourceItemDescription}</DialogDescription>
              </DialogHeader>
              <div className="flex max-h-[92vh] min-h-[280px] items-center justify-center bg-muted/40 p-3 sm:p-4">
                <img
                  src={item.source_image_url}
                  alt={sourceItemDescription}
                  className="max-h-[86vh] max-w-full rounded-md object-contain"
                  referrerPolicy="no-referrer"
                />
              </div>
            </DialogContent>
          </Dialog>
        )}
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
          onPick={(code, unit, picked) => {
            setDraft((d) => ({ ...d, item_code: code, unit_code: unit || '' }))
            setPickedMatch(picked ?? null)
          }}
          onClose={() => setShowMapModal(false)}
        />
      )}
      <TableRow className="bg-muted/20 hover:bg-muted/20">
        <TableCell colSpan={tableColumnCount} className="p-3">
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
                    {editMatchInfo.itemName && (
                      <span className="truncate text-sm font-normal text-muted-foreground">
                        {editMatchInfo.itemName}
                      </span>
                    )}
                  </Button>
                </div>
                <div className="flex items-center gap-2">
                  <MatchBadge score={editMatchInfo.score} />
                  <span className="text-xs text-muted-foreground">
                    ระบบจะจดจำคู่จับคู่นี้หลังบันทึก
                  </span>
                </div>
              </div>

              <div className={cn('grid gap-3', showDiscountColumn ? 'grid-cols-4' : 'grid-cols-3')}>
                <label className="space-y-1.5">
                  <span className="text-xs font-medium text-muted-foreground">จำนวน</span>
                  {lockSourceAmounts ? (
                    <div className="flex h-10 items-center justify-end rounded-md border border-border bg-muted/40 px-3 text-sm tabular-nums text-foreground">
                      {item.qty}
                    </div>
                  ) : (
                    <Input
                      type="number"
                      step="any"
                      value={draft.qty}
                      onChange={(e) => setDraft((d) => ({ ...d, qty: e.target.value }))}
                      className="h-10 text-right"
                    />
                  )}
                  {lockSourceAmounts && (
                    <span className="block text-[11px] text-muted-foreground">ดึงจากอีเมล</span>
                  )}
                </label>
                <div className="space-y-1.5">
                  <span className="text-xs font-medium text-muted-foreground">หน่วย</span>
                  <UnitSelect
                    value={draft.unit_code}
                    productCode={draft.item_code}
                    onValueChange={(unit_code) => setDraft((d) => ({ ...d, unit_code }))}
                    disabled={!draft.item_code}
                    autoSelectSingle
                  />
                </div>
                <label className="space-y-1.5">
                  <span className="text-xs font-medium text-muted-foreground">ราคา</span>
                  {lockSourceAmounts ? (
                    <div className="flex h-10 items-center justify-end rounded-md border border-border bg-muted/40 px-3 text-sm tabular-nums text-foreground">
                      ฿{(item.price ?? 0).toLocaleString()}
                    </div>
                  ) : (
                    <Input
                      type="number"
                      step="any"
                      value={draft.price}
                      onChange={(e) => setDraft((d) => ({ ...d, price: e.target.value }))}
                      className="h-10 text-right"
                    />
                  )}
                  {lockSourceAmounts && (
                    <span className="block text-[11px] text-muted-foreground">ดึงจากอีเมล</span>
                  )}
                </label>
                {showDiscountColumn && (
                  <div className="space-y-1.5">
                    <span className="text-xs font-medium text-muted-foreground">ส่วนลด</span>
                    <div className="flex h-10 items-center justify-end rounded-md border border-border bg-muted/40 px-3 text-sm tabular-nums text-muted-foreground">
                      {discountAmount > 0 ? `-฿${discountAmount.toLocaleString()}` : '—'}
                    </div>
                  </div>
                )}
                <div className={cn('flex items-center justify-between rounded-md bg-muted/50 px-3 py-2', showDiscountColumn ? 'col-span-4' : 'col-span-3')}>
                  <span className="text-xs font-medium text-muted-foreground">รวมรายการนี้</span>
                  <span className="tabular-nums text-sm font-semibold text-foreground">
                    ฿{Math.max(editQty * editPrice - discountAmount, 0).toLocaleString()}
                  </span>
                </div>
                <div className={cn('flex justify-end gap-2', showDiscountColumn ? 'col-span-4' : 'col-span-3')}>
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
