import { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, CreditCard, Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { BILL_SOURCE_LABEL } from '@/lib/labels'
import type { Bill } from '@/types'
import { DEFAULT_MARKETPLACE_PRINT_PAYMENT_METHODS, normalizeMarketplacePrintPolicy } from '@/pages/ChannelDefaults/labels'

function billRawString(bill: Bill | null, key: string) {
  const value = bill?.raw_data?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function marketplaceOrderID(bill: Bill | null) {
  return (
    billRawString(bill, 'order_id') ||
    billRawString(bill, 'shopee_order_id') ||
    billRawString(bill, 'order_no') ||
    ''
  )
}

export function UpdatePrintPaymentMethodDialog({
  open,
  bill,
  submitting,
  onOpenChange,
  onConfirm,
}: {
  open: boolean
  bill: Bill | null
  submitting?: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (paymentMethod: string, applyToEmailGroup: boolean) => Promise<void> | void
}) {
  const policy = useMemo(
    () => normalizeMarketplacePrintPolicy(bill?.email_group?.print_policy),
    [bill?.email_group?.print_policy],
  )
  const methods = policy.payment_methods.length > 0 ? policy.payment_methods : DEFAULT_MARKETPLACE_PRINT_PAYMENT_METHODS
  const currentMethod = (bill?.print_payment_method || bill?.effective_print_payment_method || '').trim()
  const [paymentMethod, setPaymentMethod] = useState(currentMethod)
  const [applyToEmailGroup, setApplyToEmailGroup] = useState(false)

  useEffect(() => {
    if (!open) return
    setPaymentMethod(currentMethod)
    setApplyToEmailGroup((bill?.email_group?.order_count ?? 0) > 1)
  }, [bill?.id, bill?.email_group?.order_count, currentMethod, open])

  const selectedIsTT = paymentMethod.toUpperCase().startsWith('TT')
  const sameMethod = paymentMethod === currentMethod && !applyToEmailGroup
  const canSubmit = !!paymentMethod && !sameMethod && !submitting
  const docNo = bill?.sml_doc_no || ''
  const orderID = marketplaceOrderID(bill)
  const sourceLabel = bill?.source ? BILL_SOURCE_LABEL[bill.source] || bill.source : ''
  const groupCount = bill?.email_group?.order_count ?? 0

  return (
    <Dialog open={open} onOpenChange={(next) => !submitting && onOpenChange(next)}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>วิธีชำระเงินสำหรับปริ้น</DialogTitle>
          <DialogDescription>
            เก็บใน BillFlow เท่านั้น ไม่ส่งเข้า SML และไม่แก้ payload เอกสารเดิม
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="grid gap-2 rounded-md border border-border bg-muted/25 p-3 text-sm sm:grid-cols-2">
            <InfoLine label="เอกสาร SML" value={docNo || '-'} mono />
            <InfoLine label="คำสั่งซื้อ" value={orderID || '-'} mono />
            <InfoLine label="ช่องทาง" value={sourceLabel || '-'} />
            <InfoLine label="วิธีชำระปัจจุบัน" value={currentMethod || 'ยังไม่ได้เลือก'} mono={!!currentMethod} />
          </div>

          <div className="space-y-2">
            <Label>เลือกวิธีการชำระเงิน</Label>
            <Select value={paymentMethod} onValueChange={setPaymentMethod} disabled={submitting}>
              <SelectTrigger>
                <SelectValue placeholder="เลือกวิธีการชำระเงิน" />
              </SelectTrigger>
              <SelectContent>
                {methods.map((method) => {
                  const ready = method.toUpperCase().startsWith('TT')
                  return (
                    <SelectItem key={method} value={method}>
                      {method}{ready ? '' : ' · ยังไม่พร้อมปริ้น'}
                    </SelectItem>
                  )
                })}
              </SelectContent>
            </Select>
            <p className={selectedIsTT ? 'text-xs text-emerald-600 dark:text-emerald-300' : 'text-xs text-warning'}>
              {selectedIsTT
                ? `เลือก ${paymentMethod} แล้วจะผ่านเงื่อนไขวิธีชำระเงินสำหรับปริ้น`
                : 'ค่าที่ไม่ขึ้นต้นด้วย TT บันทึกได้ แต่ยังไม่พร้อมปริ้นในรอบนี้'}
            </p>
          </div>

          {groupCount > 1 && (
            <label className="flex items-start gap-2 rounded-md border border-border bg-background px-3 py-2 text-sm">
              <Checkbox
                checked={applyToEmailGroup}
                onCheckedChange={(checked) => setApplyToEmailGroup(checked === true)}
                disabled={submitting}
                className="mt-0.5"
              />
              <span>
                ใช้วิธีชำระเงินนี้กับทุกคำสั่งซื้อในอีเมลเดียวกัน
                <span className="block text-xs text-muted-foreground">
                  {groupCount.toLocaleString('th-TH')} คำสั่งซื้อใน email group นี้
                </span>
              </span>
            </label>
          )}

          <div className="rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">
            <div className="flex gap-2">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <p>
                พร้อมปริ้นต้องมีเลข POL ครบทุก order และวิธีการชำระเงินต้องขึ้นต้นด้วย TT.
                ค่าโอนธนาคารมีไว้เตรียมอนาคต แต่ตอนนี้ยังไม่อนุญาตให้ปริ้น
              </p>
            </div>
          </div>

          {paymentMethod && (
            <div className="flex items-center gap-2 rounded-md border border-border bg-muted/20 px-3 py-2 text-sm">
              <CreditCard className="h-4 w-4 text-muted-foreground" />
              <span className="min-w-0 truncate font-medium">
                {selectedIsTT ? `ชำระด้วยบัตรเครดิต ${paymentMethod}` : `วิธีชำระเงิน: ${paymentMethod}`}
              </span>
            </div>
          )}
        </div>

        <DialogFooter className="gap-2">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
            ยกเลิก
          </Button>
          <Button
            type="button"
            onClick={() => onConfirm(paymentMethod, applyToEmailGroup)}
            disabled={!canSubmit}
          >
            {submitting && <Loader2 className="h-4 w-4 animate-spin" />}
            บันทึกวิธีชำระเงิน
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function InfoLine({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="min-w-0">
      <p className="text-[11px] font-medium text-muted-foreground">{label}</p>
      <p className={`truncate text-foreground ${mono ? 'font-mono' : ''}`} title={value}>
        {value}
      </p>
    </div>
  )
}
