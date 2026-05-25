import { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, RefreshCw, Send } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { RetryBillPayload } from '@/hooks/useBills'
import type { Bill } from '@/types'
import { REMARK2_NONE, SML_REMARK2_OPTIONS, normalizeRemark2, remark2PayloadValue } from '@/lib/smlRemark2'
import { PartyPicker, type Party } from '@/pages/ChannelDefaults/PartyPicker'
import { ShelfPicker, WarehousePicker } from './WarehousePicker'

function currentTimeHHMM() {
  const now = new Date()
  return `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`
}

function payloadString(payload: Record<string, unknown> | null | undefined, key: string) {
  const value = payload?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function payloadNumber(payload: Record<string, unknown> | null | undefined, key: string) {
  const value = payload?.[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function firstPayloadLine(payload: Record<string, unknown> | null | undefined) {
  const details = payload?.details
  if (Array.isArray(details) && typeof details[0] === 'object' && details[0] !== null) {
    return details[0] as Record<string, unknown>
  }
  const items = payload?.items
  if (Array.isArray(items) && typeof items[0] === 'object' && items[0] !== null) {
    return items[0] as Record<string, unknown>
  }
  return null
}

function routeDestination(route?: string, isSale = false) {
  if (route === 'saleinvoice') {
    return { label: 'ขาย -> ขายสินค้าและบริการ', code: 'SI' }
  }
  if (route === 'saleorder' || isSale) {
    return { label: 'ขาย -> ใบสั่งขาย', code: 'SO' }
  }
  return { label: 'ซื้อ -> ใบสั่งซื้อ', code: 'PO' }
}

const PURCHASE_INQUIRY_TYPE_OPTIONS = [
  { value: '0', label: '0 — ซื้อสินค้าเงินเชื่อ' },
  { value: '1', label: '1 — ซื้อสินค้าเงินสด' },
  { value: '3', label: '3 — ซื้อสินค้าเงินสด (สินค้าบริการ)' },
  { value: '4', label: '4 — ซื้อสินค้าเงินเชื่อ (สินค้าบริการ)' },
]

function rawString(payload: Record<string, unknown> | null | undefined, key: string) {
  const value = payload?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function rawNumber(payload: Record<string, unknown> | null | undefined, key: string) {
  const value = payload?.[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function rawBool(payload: Record<string, unknown> | null | undefined, key: string) {
  const value = payload?.[key]
  return typeof value === 'boolean' ? value : false
}

function rawObject(payload: Record<string, unknown> | null | undefined, key: string) {
  const value = payload?.[key]
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

function formatBaht(value: number | null) {
  if (value == null) return ''
  return `฿${value.toLocaleString('th-TH', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

function orderIDFromRaw(raw: Record<string, unknown> | null | undefined) {
  return rawString(raw, 'order_id') || rawString(raw, 'shopee_order_id') || rawString(raw, 'order_no')
}

interface Props {
  open: boolean
  bill: Bill
  onConfirm: (body: RetryBillPayload) => void
  onCancel: () => void
  onRegenerateDocNo?: () => Promise<string | null> | string | null | void
  regeneratingDocNo?: boolean
}

export function SendPurchaseDialog({
  open,
  bill,
  onConfirm,
  onCancel,
  onRegenerateDocNo,
  regeneratingDocNo = false,
}: Props) {
  const billType = bill.bill_type === 'sale' ? 'sale' : 'purchase'
  const isSale = billType === 'sale'
  const isPurchaseOrder = !isSale
  const isShopeePurchaseEmail = bill.source === 'shopee_shipped' && bill.bill_type === 'purchase'
  const destination = routeDestination(bill.preview?.route, isSale)
  const documentName = bill.preview?.route === 'saleinvoice'
    ? 'ขายสินค้าและบริการ'
    : isSale
      ? 'ใบสั่งขาย'
      : 'ใบสั่งซื้อ'
  const defaults = bill.preview?.sml_defaults
  const [party, setParty] = useState<Party | null>(null)
  const [docNo, setDocNo] = useState('')
  const [remark, setRemark] = useState('')
  const [branchCode, setBranchCode] = useState('')
  const [saleCode, setSaleCode] = useState('')
  const [docTime, setDocTime] = useState('')
  const [whCode, setWhCode] = useState('')
  const [shelfCode, setShelfCode] = useState('')
  const [manualWarehouse, setManualWarehouse] = useState(false)
  const [vatTypeStr, setVatTypeStr] = useState('')
  const [vatRateStr, setVatRateStr] = useState('7')
  const [inquiryTypeStr, setInquiryTypeStr] = useState('')
  const [remark2Str, setRemark2Str] = useState(REMARK2_NONE)

  const effectivePartyCode = party?.code ?? ''
  const parsedVatRate = Number(vatRateStr)
  const vatRateNum = Number.isFinite(parsedVatRate) ? parsedVatRate : 7
  const paymentSummary = rawObject(bill.raw_data, 'payment_summary')
  const paymentMethod = rawString(paymentSummary, 'payment_method')
  const paymentPaidAmount = rawNumber(paymentSummary, 'payment_paid_amount')
  const paymentDocRefAmount = rawString(paymentSummary, 'doc_ref_amount')
  const paymentIsCard = rawBool(paymentSummary, 'is_credit_debit_card')
  const sellerFromEmail = rawString(bill.raw_data, 'seller_name')
  const orderID = orderIDFromRaw(bill.raw_data)
  const canConfirm =
    !!effectivePartyCode &&
    whCode.trim() !== '' &&
    shelfCode.trim() !== '' &&
    vatTypeStr !== '' &&
    (!isPurchaseOrder || inquiryTypeStr !== '') &&
    docTime.trim() !== ''
  const missingFields = useMemo(() => [
    !effectivePartyCode ? (isSale ? 'ลูกค้า' : 'ผู้ขาย') : '',
    whCode.trim() === '' ? 'คลัง' : '',
    shelfCode.trim() === '' ? 'พื้นที่เก็บ' : '',
    vatTypeStr === '' ? 'ประเภทภาษี' : '',
    isPurchaseOrder && inquiryTypeStr === '' ? 'ประเภทรายการ' : '',
    docTime.trim() === '' ? 'เวลาเอกสาร' : '',
  ].filter(Boolean), [docTime, effectivePartyCode, inquiryTypeStr, isPurchaseOrder, isSale, shelfCode, vatTypeStr, whCode])

  useEffect(() => {
    if (!open) return
    const payload = bill.sml_payload
    const firstLine = firstPayloadLine(payload)
    const partyCode = payloadString(payload, 'cust_code')
    const partyName = payloadString(payload, 'supplier_name') || partyCode
    setParty(partyCode ? { code: partyCode, name: partyName } : null)
    setDocNo(payloadString(payload, 'doc_no') || bill.sml_doc_no || bill.preview?.doc_no || '')
    setRemark(bill.remark ?? '')
    setBranchCode(defaults?.branch_code ?? '')
    setSaleCode(defaults?.sale_code ?? '')
    setDocTime(payloadString(payload, 'doc_time') || currentTimeHHMM())
    setWhCode(payloadString(payload, 'wh_code') || payloadString(firstLine, 'wh_code'))
    setShelfCode(payloadString(payload, 'shelf_code') || payloadString(firstLine, 'shelf_code'))
    setManualWarehouse(false)
    const vatType = payloadNumber(payload, 'vat_type')
    const vatRate = payloadNumber(payload, 'vat_rate')
    const inquiryType = payloadNumber(payload, 'inquiry_type')
    setRemark2Str(normalizeRemark2(payloadString(payload, 'remark_2')))
    setVatTypeStr(vatType != null ? String(vatType) : '')
    setVatRateStr(vatRate != null ? String(vatRate) : '7')
    setInquiryTypeStr(inquiryType != null ? String(inquiryType) : '')
  }, [open, bill.id, bill.remark, bill.sml_doc_no, bill.sml_payload, defaults])

  const handleConfirm = () => {
    if (!canConfirm) return
    onConfirm({
      party_code: effectivePartyCode,
      party_name: party?.name,
      doc_no: docNo.trim() || undefined,
      remark: isShopeePurchaseEmail ? undefined : remark.trim() || undefined,
      remark_2: remark2PayloadValue(remark2Str),
      branch_code: branchCode.trim() || undefined,
      sale_code: saleCode.trim() || undefined,
      doc_time: docTime.trim() || undefined,
      wh_code: whCode.trim(),
      shelf_code: shelfCode.trim(),
      vat_type: Number(vatTypeStr),
      vat_rate: vatRateNum,
      inquiry_type: isPurchaseOrder ? Number(inquiryTypeStr) : undefined,
    })
  }

  const handleRegenerateDocNo = async () => {
    if (!onRegenerateDocNo) return
    const nextDocNo = await onRegenerateDocNo()
    if (typeof nextDocNo === 'string' && nextDocNo.trim()) {
      setDocNo(nextDocNo.trim())
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onCancel() }}>
      <DialogContent className="grid max-h-[90vh] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            ยืนยันการส่ง {documentName} ไปยัง SML
          </DialogTitle>
        </DialogHeader>

        <div className="-mx-6 space-y-4 overflow-y-auto px-6 py-2">
          <div className="rounded-md border border-info/25 bg-info/[0.04] px-3 py-2 text-xs text-muted-foreground">
            <div className="font-medium text-foreground">
              ปลายทาง SML: {destination.label} · {bill.preview?.doc_format_code || destination.code}
            </div>
            <div className="mt-0.5">
              เลือกค่าที่จะใช้กับบิลใบนี้เท่านั้น ระบบจะไม่บันทึกค่าเหล่านี้กลับไปเป็นค่าของช่องทาง
            </div>
          </div>

          <div className="space-y-1.5">
            <Label>{isSale ? 'ลูกค้า' : 'ผู้ขาย'} <span className="text-destructive">*</span></Label>
            <PartyPicker
              billType={billType}
              value={party}
              onChange={setParty}
            />
            {!effectivePartyCode && (
              <p className="text-[11px] text-warning">
                ต้องเลือก{isSale ? 'ลูกค้า' : 'ผู้ขาย'}ก่อนส่งเข้า SML
              </p>
            )}
          </div>

          <div className="grid gap-2.5 rounded-md border border-border bg-muted/20 p-3 sm:grid-cols-2">
            <div className="space-y-1">
              <div className="flex items-center justify-between gap-2">
                <Label className="text-xs">เลขเอกสาร SML (doc_no)</Label>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-6 gap-1 px-1.5 text-[11px]"
                  onClick={handleRegenerateDocNo}
                  disabled={!onRegenerateDocNo || regeneratingDocNo}
                  title="ดึงเลข running ล่าสุดจาก SML แล้วออกเลขใหม่ให้บิลนี้"
                >
                  <RefreshCw className={`h-3 w-3 ${regeneratingDocNo ? 'animate-spin' : ''}`} />
                  ดึงเลขล่าสุด
                </Button>
              </div>
              <Input
                value={docNo}
                onChange={(e) => setDocNo(e.target.value.trim().toUpperCase())}
                placeholder="เว้นว่างเพื่อให้ระบบออกเลข running ตอนส่ง"
                className="font-mono"
              />
              <p className="text-[10px] text-muted-foreground">
                ระบบถามเลขล่าสุดจาก SML ตอนออกเลขใหม่ และยังจองเลขจริงอีกครั้งตอนกดส่ง
              </p>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">
                เวลาเอกสาร <span className="text-destructive">*</span>
              </Label>
              <Input
                value={docTime}
                onChange={(e) => setDocTime(e.target.value)}
                placeholder="เช่น 09:00"
                className="font-mono"
              />
              <p className="text-[10px] text-muted-foreground">
                ใช้เวลาปัจจุบันตอนเปิด dialog
              </p>
            </div>
            <div className="space-y-1">
              <div className="flex items-center justify-between gap-2">
                <Label className="text-xs">คลัง <span className="text-destructive">*</span></Label>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-6 px-1.5 text-[11px]"
                  onClick={() => setManualWarehouse((v) => !v)}
                >
                  {manualWarehouse ? 'เลือกจาก SML' : 'พิมพ์รหัสเอง'}
                </Button>
              </div>
              {manualWarehouse ? (
                <Input
                  value={whCode}
                  onChange={(e) => {
                    setWhCode(e.target.value.toUpperCase())
                    setShelfCode('')
                  }}
                  placeholder="เช่น WH-01"
                  className="font-mono"
                />
              ) : (
                <WarehousePicker
                  value={whCode}
                  onChange={(warehouse) => {
                    setWhCode(warehouse.code)
                    setShelfCode('')
                  }}
                />
              )}
              <p className="text-[10px] text-muted-foreground">
                เลือกจากคลังใน SML หรือพิมพ์เองถ้า service ยังไม่พร้อม
              </p>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">พื้นที่เก็บ <span className="text-destructive">*</span></Label>
              {manualWarehouse ? (
                <Input
                  value={shelfCode}
                  onChange={(e) => setShelfCode(e.target.value.toUpperCase())}
                  placeholder="เช่น SH-01"
                  className="font-mono"
                />
              ) : (
                <ShelfPicker
                  warehouseCode={whCode}
                  value={shelfCode}
                  onChange={(shelf) => setShelfCode(shelf.code)}
                />
              )}
              <p className="text-[10px] text-muted-foreground">
                พื้นที่เก็บจะถูกกรองตามคลังที่เลือก
              </p>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">ประเภทภาษี <span className="text-destructive">*</span></Label>
              <Select value={vatTypeStr} onValueChange={setVatTypeStr}>
                <SelectTrigger className="h-9 text-sm">
                  <SelectValue placeholder="เลือกประเภทภาษี" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="0">0 — แยกนอก</SelectItem>
                  <SelectItem value="1">1 — รวมใน</SelectItem>
                  <SelectItem value="2">2 — ศูนย์%</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {isPurchaseOrder && (
              <div className="space-y-1">
                <Label className="text-xs">ประเภทรายการ <span className="text-destructive">*</span></Label>
                <Select value={inquiryTypeStr} onValueChange={setInquiryTypeStr}>
                  <SelectTrigger className="h-9 text-sm">
                    <SelectValue placeholder="เลือกประเภทรายการ" />
                  </SelectTrigger>
                  <SelectContent>
                    {PURCHASE_INQUIRY_TYPE_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="space-y-1">
              <Label className="text-xs">สถานะเอกสาร</Label>
              <Select value={remark2Str} onValueChange={setRemark2Str}>
                <SelectTrigger className="h-9 text-sm">
                  <SelectValue placeholder="ไม่ระบุ" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={REMARK2_NONE}>ไม่ระบุ</SelectItem>
                  {SML_REMARK2_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="rounded-md bg-background/70 px-2.5 py-1.5 text-[11px] text-muted-foreground sm:col-span-2">
              อัตราภาษีใช้ {Number.isFinite(vatRateNum) ? `${vatRateNum}%` : '7%'} จากค่าเริ่มต้นของระบบเพื่อกัน user กรอกผิด
            </div>
            <details className="space-y-2 rounded-md border border-border bg-background px-3 py-2 sm:col-span-2">
              <summary className="cursor-pointer text-xs font-medium text-muted-foreground">
                ตัวเลือกเพิ่มเติม: Branch code / Sale code (ไม่บังคับ)
              </summary>
              <div className="mt-3 grid gap-3 sm:grid-cols-2">
                <div className="space-y-1">
                  <Label className="text-xs">Branch code</Label>
                  <Input
                    value={branchCode}
                    onChange={(e) => setBranchCode(e.target.value)}
                    placeholder="ปล่อยว่างได้"
                    className="font-mono"
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">Sale code</Label>
                  <Input
                    value={saleCode}
                    onChange={(e) => setSaleCode(e.target.value)}
                    placeholder="ปล่อยว่างได้"
                    className="font-mono"
                  />
                </div>
              </div>
            </details>
            <div className="rounded-md bg-background/70 px-2.5 py-1.5 text-[11px] text-muted-foreground sm:col-span-2">
              เลขเอกสารจะใช้ค่าที่แสดงอยู่ใน dialog นี้ ถ้า SML แจ้งเลขซ้ำ ให้กดดึงเลขล่าสุดแล้วส่งใหม่
            </div>
          </div>

          {isShopeePurchaseEmail && (
            <div className="rounded-md border border-info/25 bg-info/[0.04] px-3 py-2.5 text-xs">
              <div className="font-medium text-foreground">ข้อมูลที่จะส่งไปหัวเอกสาร SML จากอีเมล Shopee</div>
              <div className="mt-2 grid gap-2 sm:grid-cols-2">
                <div>
                  <div className="text-muted-foreground">ผู้ขาย → remark</div>
                  <div className="font-medium text-foreground">{sellerFromEmail || 'ไม่พบผู้ขายในอีเมล'}</div>
                </div>
                <div>
                  <div className="text-muted-foreground">หมายเลขคำสั่งซื้อ → remark_5</div>
                  <div className="font-mono font-medium text-foreground">{orderID || 'ไม่พบหมายเลขคำสั่งซื้อ'}</div>
                </div>
                <div>
                  <div className="text-muted-foreground">วิธีชำระเงิน</div>
                  <div className="font-medium text-foreground">{paymentMethod || 'ไม่พบรายละเอียดการชำระเงินในอีเมล'}</div>
                </div>
                <div>
                  <div className="text-muted-foreground">doc_ref</div>
                  <div className="font-medium text-foreground">
                    {paymentIsCard
                      ? paymentDocRefAmount
                        ? `${paymentDocRefAmount} (${formatBaht(paymentPaidAmount)})`
                        : 'เป็นบัตรเครดิต/เดบิต แต่ไม่พบจำนวนเงินที่จ่าย'
                      : paymentMethod
                        ? 'ไม่ใช่บัตรเครดิต/เดบิต จึงไม่ส่ง doc_ref'
                        : 'ไม่พบรายละเอียดการชำระเงินในอีเมล'}
                  </div>
                </div>
              </div>
              <div className="mt-2 text-[11px] text-muted-foreground">
                หมายเหตุ SML ของบิล Shopee ซื้อจะใช้ผู้ขายจากอีเมลอัตโนมัติ เพื่อไม่ให้ชนกับ requirement หัวเอกสาร
              </div>
            </div>
          )}

          {!isShopeePurchaseEmail && (
            <div className="space-y-1.5">
              <Label htmlFor="remark">หมายเหตุ</Label>
              <textarea
                id="remark"
                value={remark}
                onChange={(e) => setRemark(e.target.value)}
                placeholder="หมายเหตุสำหรับ SML (ถ้ามี)"
                rows={3}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
              />
            </div>
          )}
          {missingFields.length > 0 && (
            <div className="flex items-start gap-2 rounded-md border border-warning/35 bg-warning/[0.07] px-3 py-2 text-xs text-warning">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <div>ต้องกรอกเพิ่มก่อนส่ง: {missingFields.join(', ')}</div>
            </div>
          )}
        </div>

        <DialogFooter className="gap-2">
          <Button type="button" variant="outline" onClick={onCancel}>
            ยกเลิก
          </Button>
          <Button type="button" onClick={handleConfirm} disabled={!canConfirm} className="gap-2">
            <Send className="h-4 w-4" />
            ส่งไปยัง SML
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
