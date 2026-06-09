import { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, ArrowRight, Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { BILL_SOURCE_LABEL } from '@/lib/labels'
import type { Bill } from '@/types'
import { PartyPicker, type Party } from '@/pages/ChannelDefaults/PartyPicker'

function payloadString(
  payload: Record<string, unknown> | null | undefined,
  key: string,
) {
  const value = payload?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function billRawString(bill: Bill | null, key: string) {
  const value = bill?.raw_data?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function currentCreditorFromBill(bill: Bill | null): Party | null {
  const code = payloadString(bill?.sml_payload, 'cust_code')
  if (!code) return null
  return {
    code,
    name: payloadString(bill?.sml_payload, 'supplier_name') || code,
  }
}

function marketplaceOrderID(bill: Bill | null) {
  return (
    billRawString(bill, 'order_id') ||
    billRawString(bill, 'shopee_order_id') ||
    billRawString(bill, 'order_no') ||
    ''
  )
}

export function UpdatePurchaseCreditorDialog({
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
  onConfirm: (party: Party) => Promise<void> | void
}) {
  const currentCreditor = useMemo(() => currentCreditorFromBill(bill), [bill])
  const [party, setParty] = useState<Party | null>(currentCreditor)

  useEffect(() => {
    if (open) setParty(currentCreditor)
  }, [open, currentCreditor])

  const sameCreditor = !!party?.code && !!currentCreditor?.code && party.code === currentCreditor.code
  const canSubmit = !!party?.code && !sameCreditor && !submitting
  const docNo = bill?.sml_doc_no || ''
  const orderID = marketplaceOrderID(bill)
  const sourceLabel = bill?.source ? BILL_SOURCE_LABEL[bill.source] || bill.source : ''

  return (
    <Dialog open={open} onOpenChange={(next) => !submitting && onOpenChange(next)}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>แก้เจ้าหนี้ใน SML</DialogTitle>
          <DialogDescription>
            อัปเดตเอกสาร SML เดิมโดยไม่ส่งใบสั่งซื้อใหม่ ใช้ได้เฉพาะผู้ดูแลระบบ
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="grid gap-2 rounded-md border border-border bg-muted/25 p-3 text-sm sm:grid-cols-2">
            <InfoLine label="เอกสาร SML" value={docNo || '-'} mono />
            <InfoLine label="คำสั่งซื้อ" value={orderID || '-'} mono />
            <InfoLine label="ช่องทาง" value={sourceLabel || '-'} />
            <InfoLine
              label="เจ้าหนี้เดิม"
              value={currentCreditor ? `${currentCreditor.code} · ${currentCreditor.name}` : 'ไม่พบใน SML payload'}
              mono={!!currentCreditor?.code}
            />
          </div>

          <div className="space-y-2">
            <Label>เลือกเจ้าหนี้ใหม่</Label>
            <PartyPicker billType="purchase" value={party} onChange={setParty} disabled={submitting} />
            {sameCreditor && (
              <p className="text-xs text-muted-foreground">
                เลือกเจ้าหนี้ใหม่ที่ต่างจากเดิมก่อนอัปเดต
              </p>
            )}
          </div>

          <div className="rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">
            <div className="flex gap-2">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <p>
                การเปลี่ยนนี้จะแก้ cust_code ใน SML ของใบสั่งซื้อเดิมและ sync กลับ BillFlow
                เฉพาะช่องเจ้าหนี้เท่านั้น ไม่แก้สินค้า ยอดเงิน ภาษี หรือหมายเหตุ
              </p>
            </div>
          </div>

          <div className="flex min-w-0 items-center gap-2 rounded-md bg-background text-sm">
            <CreditorChip party={currentCreditor} fallback="เจ้าหนี้เดิม" />
            <ArrowRight className="h-4 w-4 shrink-0 text-muted-foreground" />
            <CreditorChip party={party} fallback="ยังไม่เลือกเจ้าหนี้ใหม่" highlight />
          </div>
        </div>

        <DialogFooter className="gap-2">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
            ยกเลิก
          </Button>
          <Button
            type="button"
            onClick={() => party && onConfirm(party)}
            disabled={!canSubmit}
          >
            {submitting && <Loader2 className="h-4 w-4 animate-spin" />}
            อัปเดตเจ้าหนี้ใน SML
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

function CreditorChip({
  party,
  fallback,
  highlight,
}: {
  party: Party | null
  fallback: string
  highlight?: boolean
}) {
  const text = party?.code ? `${party.code} · ${party.name || party.code}` : fallback
  return (
    <span
      className={[
        'min-w-0 flex-1 truncate rounded-md border px-2.5 py-2 text-xs',
        highlight
          ? 'border-primary/40 bg-primary/10 text-foreground'
          : 'border-border bg-muted/30 text-muted-foreground',
      ].join(' ')}
      title={text}
    >
      {text}
    </span>
  )
}
