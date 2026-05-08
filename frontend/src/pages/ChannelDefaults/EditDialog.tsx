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
import { PartyPicker, type Party } from '@/pages/ChannelDefaults/PartyPicker'

import {
  CHANNEL_LABELS,
  channelHelp,
  destinationFor,
  docNoPatternWarning,
  endpointFor,
  ENDPOINT_OPTIONS,
  phase1DestinationOptions,
  previewDocNo,
  resolveEndpointKind,
  type ChannelDefaultRow,
  type ChannelKey,
  type EndpointKind,
} from './labels'

const PHASE = Number(import.meta.env.VITE_PHASE ?? 99)

interface Props {
  open: boolean
  onOpenChange: (v: boolean) => void
  row: ChannelDefaultRow | null
  onSaved: () => void
}

export function EditDialog({ open, onOpenChange, row, onSaved }: Props) {
  const [party, setParty] = useState<Party | null>(null)
  const [docFormat, setDocFormat] = useState('')
  const [endpointOverride, setEndpointOverride] = useState<string>('')
  const [selectedDestination, setSelectedDestination] = useState<Exclude<EndpointKind, ''>>('purchaseorder')
  const [docPrefix, setDocPrefix] = useState('')
  const [docRunningFormat, setDocRunningFormat] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open || !row) return
    if (row.party_code) {
      setParty({
        code: row.party_code,
        name: row.party_name,
        tax_id: row.party_tax_id,
        telephone: row.party_phone,
        address: row.party_address,
      })
    } else {
      setParty(null)
    }
    const detectedDestination = destinationFor(
      row.channel as ChannelKey,
      row.bill_type,
      row.endpoint ?? '',
      row.doc_format_code ?? '',
    )
    const defaultDestination = phase1DestinationOptions(row.bill_type)[0]
    const destination = detectedDestination ?? defaultDestination

    setSelectedDestination(destination?.value ?? 'purchaseorder')
    setDocFormat(row.doc_format_code || destination?.docFormatCode || '')
    setEndpointOverride(row.endpoint || destination?.apiPath || '')
    setDocPrefix(row.doc_prefix || destination?.docPrefix || '')
    setDocRunningFormat(row.doc_running_format || destination?.docRunningFormat || '')
  }, [open, row])

  if (!row) return null

  const phase1 = PHASE < 2
  const isPurchase = row.bill_type === 'purchase'
  const channelLabel = CHANNEL_LABELS[row.channel as ChannelKey] ?? row.channel
  const billTypeLabel = isPurchase ? 'บิลซื้อ' : 'บิลขาย'
  const destinationOptions = phase1DestinationOptions(row.bill_type)
  const selectedDestinationMeta =
    destinationOptions.find((option) => option.value === selectedDestination) ??
    destinationFor(row.channel as ChannelKey, row.bill_type, endpointOverride, docFormat)
  const effectiveEndpoint = phase1
    ? selectedDestinationMeta?.apiPath ?? endpointOverride
    : endpointOverride
  const effectiveDocFormat = phase1
    ? selectedDestinationMeta?.docFormatCode ?? docFormat
    : docFormat
  const endpoint = endpointFor(row.channel as ChannelKey, row.bill_type, effectiveEndpoint)
  const autoKind = resolveEndpointKind('', row.channel as ChannelKey, row.bill_type)
  const isOverridden = !phase1 && endpointOverride && endpointOverride !== autoKind

  const handleDestinationChange = (value: Exclude<EndpointKind, ''>) => {
    const destination = destinationOptions.find((option) => option.value === value)
    setSelectedDestination(value)
    if (!destination) return
    setEndpointOverride(destination.apiPath)
    setDocFormat(destination.docFormatCode)
    setDocPrefix(destination.docPrefix)
    setDocRunningFormat(destination.docRunningFormat)
  }

  const handleSave = async () => {
    if (!phase1 && !party) {
      toast.error('กรุณาเลือก' + (isPurchase ? 'ผู้ขาย' : 'ลูกค้า') + 'ก่อน')
      return
    }
    setSaving(true)
    try {
      await client.put('/api/settings/channel-defaults', {
        channel: row.channel,
        bill_type: row.bill_type,
        party_code: party?.code ?? row.party_code ?? '',
        party_name: party?.name ?? row.party_name ?? '',
        party_phone: party?.telephone ?? row.party_phone ?? '',
        party_address: party?.address ?? row.party_address ?? '',
        party_tax_id: party?.tax_id ?? row.party_tax_id ?? '',
        doc_format_code: endpoint.takesDocFormat ? effectiveDocFormat.trim() : '',
        endpoint: effectiveEndpoint,
        doc_prefix: docPrefix.trim(),
        doc_running_format: docRunningFormat.trim(),
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
            {phase1 ? 'ตั้งค่าเส้นทาง SML สำหรับ' : `ตั้งค่า${isPurchase ? 'ผู้ขาย' : 'ลูกค้า'} —`}{' '}
            {channelLabel} ({billTypeLabel})
          </DialogTitle>
        </DialogHeader>

        <div className="-mx-6 space-y-4 overflow-y-auto px-6 py-2">
          <div className="space-y-1.5">
            <Label>{phase1 ? 'ปลายทาง SML' : 'API ที่ส่งเข้า SML (URL หรือ path)'}</Label>
            {phase1 ? (
              <>
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
                      {selectedDestinationMeta?.label ?? endpoint.label}
                    </span>
                  </div>
                  <code className="mt-1 block text-[10px] text-muted-foreground">
                    POST {selectedDestinationMeta?.apiPath ?? endpoint.apiPath}
                  </code>
                  <p className="mt-1 text-[11px] text-muted-foreground">
                    {selectedDestinationMeta?.description}
                  </p>
                </div>
              </>
            ) : (
              <>
                <Input
                  value={endpointOverride}
                  onChange={(e) => setEndpointOverride(e.target.value)}
                  placeholder={`ถ้าไม่ระบุ ระบบจะใช้ — ${
                    ENDPOINT_OPTIONS.find((o) => o.value === autoKind)?.apiPath ?? ''
                  }`}
                  className="font-mono text-xs"
                />
                <div className="rounded-md border border-border bg-muted/30 px-3 py-2 text-xs">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
                      ปลายทางที่ตรวจจับได้
                    </span>
                    <span className="font-medium text-foreground">{endpoint.label}</span>
                    {isOverridden && (
                      <span className="rounded bg-info/10 px-1.5 py-0.5 text-[9px] font-medium uppercase text-info">
                        ตั้งเอง
                      </span>
                    )}
                  </div>
                  <code className="mt-0.5 block text-[10px] text-muted-foreground">
                    POST {endpointOverride || endpoint.apiPath}
                  </code>
                  <p className="mt-1 text-[11px] text-muted-foreground">
                    {ENDPOINT_OPTIONS.find((o) => o.value === resolveEndpointKind(endpointOverride, row.channel as ChannelKey, row.bill_type))?.description}
                  </p>
                </div>
              </>
            )}
          </div>

          {!phase1 && (
            <div className="space-y-1.5">
              <Label>{isPurchase ? 'ผู้ขาย' : 'ลูกค้า'} จาก SML</Label>
              <PartyPicker
                billType={row.bill_type}
                value={party}
                onChange={setParty}
              />
              <p className="text-xs text-muted-foreground">
                {channelHelp(row.channel as ChannelKey, isPurchase)}
              </p>
            </div>
          )}

          {endpoint.takesDocFormat && !phase1 && (
            <div className="space-y-1.5">
              <Label>doc_format_code</Label>
              <Input
                value={docFormat}
                onChange={(e) => setDocFormat(e.target.value)}
                placeholder={`เช่น ${endpoint.docFormatHint}`}
                className="font-mono uppercase"
              />
              <p className="text-xs text-muted-foreground">
                รหัส doc format ที่ SML ใช้แยกประเภทเอกสาร — บิลที่ส่งเข้า{' '}
                {endpoint.label} จะถูกบันทึกด้วย code นี้
                {endpoint.docFormatHint && ` (แนะนำ: ${endpoint.docFormatHint})`}
              </p>
            </div>
          )}

          {endpoint.takesDocFormat && phase1 && (
            <div className="space-y-1.5">
              <Label>doc_format_code</Label>
              <div className="rounded-md border border-border bg-muted/30 px-3 py-2 font-mono text-sm text-foreground">
                {effectiveDocFormat || 'PO'}
              </div>
              <p className="text-xs text-muted-foreground">
                ค่านี้มากับปลายทาง SML ที่เลือกไว้ จึงไม่ต้องกรอกเองใน Phase 1
              </p>
            </div>
          )}

          {endpoint.takesDocFormat && (
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
                    onChange={(e) => setDocRunningFormat(e.target.value)}
                    placeholder="YYMM####"
                    className="font-mono"
                  />
                </div>
              </div>
              <div className="space-y-1 text-xs text-muted-foreground">
                <div>
                  <b>ตัวอย่างถัดไป:</b>{' '}
                  <code className="rounded bg-background px-1.5 py-0.5 font-mono text-foreground">
                    {previewDocNo(docPrefix || 'BF', docRunningFormat || 'YYMM####')}
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
              {docNoPatternWarning(docPrefix, docRunningFormat) && (
                <div className="rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">
                  ⚠️ {docNoPatternWarning(docPrefix, docRunningFormat)}
                </div>
              )}
            </div>
          )}

          {!phase1 && party && (
            <div className="rounded-md border border-border bg-muted/30 px-3 py-2 text-sm">
              <div className="flex items-center gap-2">
                <span className="font-mono text-xs text-muted-foreground">{party.code}</span>
                <span className="font-medium">{party.name}</span>
              </div>
              {(party.tax_id || party.telephone || party.address) && (
                <div className="mt-1 text-xs text-muted-foreground">
                  {party.tax_id && <div>เลขผู้เสียภาษี: {party.tax_id}</div>}
                  {party.telephone && <div>เบอร์โทร: {party.telephone}</div>}
                  {party.address && <div>ที่อยู่: {party.address}</div>}
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            ยกเลิก
          </Button>
          <Button onClick={handleSave} disabled={saving || (!phase1 && !party)}>
            {saving ? 'กำลังบันทึก…' : 'บันทึก'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
