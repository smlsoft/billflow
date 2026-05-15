import { useEffect, useState } from 'react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import client from '@/api/client'

import {
  CHANNEL_LABELS,
  destinationFor,
  destinationOptionsFor,
  docNoPatternWarning,
  previewDocNo,
  type ChannelDefaultRow,
  type ChannelKey,
  type EndpointKind,
} from './labels'

interface Props {
  open: boolean
  onOpenChange: (v: boolean) => void
  row: ChannelDefaultRow | null
  onSaved: () => void
}

export function EditDialog({ open, onOpenChange, row, onSaved }: Props) {
  const [selectedDestination, setSelectedDestination] = useState<EndpointKind>('purchaseorder')
  const [docPrefix, setDocPrefix] = useState('')
  const [docRunningFormat, setDocRunningFormat] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open || !row) return
    const detectedDestination = destinationFor(
      row.channel as ChannelKey,
      row.bill_type,
      row.endpoint ?? '',
      row.doc_format_code ?? '',
    )
    const defaultDestination = destinationOptionsFor(row.bill_type)[0]
    const destination = detectedDestination ?? defaultDestination

    setSelectedDestination(destination?.value ?? 'purchaseorder')
    setDocPrefix(row.doc_prefix || destination?.docPrefix || '')
    setDocRunningFormat(row.doc_running_format || destination?.docRunningFormat || '')
  }, [open, row])

  if (!row) return null

  const isPurchase = row.bill_type === 'purchase'
  const channelLabel =
    row.channel === 'shopee_shipped' && row.bill_type === 'purchase'
      ? 'Email บิลซื้อ Shopee'
      : CHANNEL_LABELS[row.channel as ChannelKey] ?? row.channel
  const billTypeLabel = isPurchase ? 'บิลซื้อ' : 'บิลขาย'
  const destinationOptions = destinationOptionsFor(row.bill_type)
  const selectedDestinationMeta =
    destinationOptions.find((option) => option.value === selectedDestination) ??
    destinationFor(row.channel as ChannelKey, row.bill_type, row.endpoint ?? '', row.doc_format_code ?? '') ??
    destinationOptions[0]
  const docPrefixTrimmed = docPrefix.trim()
  const docRunningFormatTrimmed = docRunningFormat.trim().toUpperCase()
  const docWarning = docNoPatternWarning(docPrefixTrimmed, docRunningFormatTrimmed)
  const canSave =
    !!selectedDestinationMeta &&
    docPrefixTrimmed !== '' &&
    docRunningFormatTrimmed !== '' &&
    docRunningFormatTrimmed.includes('#') &&
    !docWarning &&
    !saving

  const handleDestinationChange = (value: EndpointKind) => {
    const destination = destinationOptions.find((option) => option.value === value)
    setSelectedDestination(value)
    if (!destination) return
    setDocPrefix(destination.docPrefix)
    setDocRunningFormat(destination.docRunningFormat)
  }

  const handleSave = async () => {
    if (saving) return
    if (!selectedDestinationMeta) {
      toast.error('กรุณาเลือกปลายทาง SML ก่อน')
      return
    }
    if (!docPrefixTrimmed) {
      toast.error('กรุณากรอก prefix ของเลขเอกสาร')
      return
    }
    if (!docRunningFormatTrimmed || !docRunningFormatTrimmed.includes('#')) {
      toast.error('รูปแบบเลขรันต้องมี # อย่างน้อย 1 ตัว')
      return
    }
    if (docWarning) {
      toast.error('แก้รูปแบบเลขเอกสารตามคำเตือนก่อนบันทึก')
      return
    }
    setSaving(true)
    try {
      await client.put('/api/settings/channel-defaults', {
        channel: row.channel,
        bill_type: row.bill_type,
        party_code: row.party_code ?? '',
        party_name: row.party_name ?? '',
        party_phone: row.party_phone ?? '',
        party_address: row.party_address ?? '',
        party_tax_id: row.party_tax_id ?? '',
        doc_format_code: selectedDestinationMeta.docFormatCode,
        endpoint: selectedDestinationMeta.apiPath,
        doc_prefix: docPrefixTrimmed,
        doc_running_format: docRunningFormatTrimmed,
        branch_code: '',
        sale_code: '',
        unit_code: '',
        doc_time: '',
        wh_code: '',
        shelf_code: '',
        vat_type: -1,
        vat_rate: -1,
      })
      toast.success('บันทึกสำเร็จ')
      onSaved()
      onOpenChange(false)
    } catch (e: any) {
      toast.error('บันทึกล้มเหลว: ' + (e?.response?.data?.error ?? e?.message ?? 'unknown'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* max-h + grid-rows[auto, scrollable, auto] so the body scrolls but
          header/footer stay pinned. minmax(0,1fr) is required — plain 1fr
          won't shrink below content height and overflow-y-auto never fires. */}
      <DialogContent className="grid max-h-[90vh] max-w-xl grid-rows-[auto_minmax(0,1fr)_auto]">
        <DialogHeader>
          <DialogTitle>
            ตั้งค่าเส้นทาง SML สำหรับ{' '}
            {channelLabel} ({billTypeLabel})
          </DialogTitle>
        </DialogHeader>

        <div className="-mx-6 space-y-4 overflow-y-auto px-6 py-2">
          <div className="space-y-1.5">
            <Label>ปลายทาง SML</Label>
            <Select value={selectedDestination} onValueChange={handleDestinationChange}>
              <SelectTrigger>
                <SelectValue placeholder="เลือกปลายทาง SML" />
              </SelectTrigger>
              <SelectContent>
                {destinationOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div className="rounded-md border border-success/30 bg-success/5 px-3 py-2 text-xs">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium text-foreground">
                  {selectedDestinationMeta?.label}
                </span>
                {selectedDestinationMeta?.statusLabel && (
                  <span className="rounded bg-success/10 px-1.5 py-0.5 text-[9px] font-medium text-success">
                    {selectedDestinationMeta.statusLabel}
                  </span>
                )}
              </div>
              <code className="mt-1 block text-[10px] text-muted-foreground">
                POST {selectedDestinationMeta?.apiPath}
              </code>
              <p className="mt-1 text-[11px] text-muted-foreground">
                {selectedDestinationMeta?.description}
              </p>
            </div>
          </div>

          <div className="space-y-1.5">
            <Label>doc_format_code</Label>
            <div className="rounded-md border border-border bg-muted/30 px-3 py-2 font-mono text-sm text-foreground">
              {selectedDestinationMeta?.docFormatCode || '-'}
            </div>
            <p className="text-xs text-muted-foreground">
              ค่านี้มาจากปลายทาง SML ที่เลือกไว้
            </p>
          </div>

          <div className="space-y-3 rounded-md border border-border bg-muted/20 p-3">
            <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              เลขเอกสาร (doc_no)
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <Label className="text-xs">รหัสขึ้นต้น (prefix)</Label>
                <Input
                  value={docPrefix}
                  onChange={(e) => setDocPrefix(e.target.value)}
                  placeholder="BF-SO"
                  className="font-mono"
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">รูปแบบเลขรัน</Label>
                <Input
                  value={docRunningFormat}
                  onChange={(e) => setDocRunningFormat(e.target.value.toUpperCase())}
                  placeholder="YYMM####"
                  className="font-mono"
                />
              </div>
            </div>
            <div className="space-y-1 text-xs text-muted-foreground">
              <div>
                <b>ตัวอย่างถัดไป:</b>{' '}
                <code className="rounded bg-background px-1.5 py-0.5 font-mono text-foreground">
                  {previewDocNo(docPrefixTrimmed || 'BF', docRunningFormatTrimmed || 'YYMM####')}
                </code>
              </div>
              <div>
                Token: <code>YYYY</code> = ปีเต็ม 4 หลัก, <code>YY</code> = 2 หลัก,{' '}
                <code>MM</code> = เดือน, <code>DD</code> = วัน, <code>####</code> =
                เลขรัน (จำนวน <code>#</code> = หลักของเลข — reset ตามช่วงที่ใช้:
                มี <code>DD</code> = รายวัน, <code>MM</code> = รายเดือน,{' '}
                <code>YY</code> = รายปี)
              </div>
            </div>
            {docWarning && (
              <div className="rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">
                ⚠️ {docWarning}
              </div>
            )}
            {docRunningFormatTrimmed && !docRunningFormatTrimmed.includes('#') && (
              <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
                รูปแบบเลขรันต้องมี # เพื่อให้ระบบออกเลขเอกสารต่อเนื่องได้
              </div>
            )}
          </div>

        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            ยกเลิก
          </Button>
          <Button onClick={handleSave} disabled={!canSave}>
            {saving ? 'กำลังบันทึก…' : 'บันทึก'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
