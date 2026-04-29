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
  docNoPatternWarning,
  endpointFor,
  ENDPOINT_OPTIONS,
  previewDocNo,
  resolveEndpointKind,
  type ChannelDefaultRow,
  type ChannelKey,
} from './labels'

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
  const [docPrefix, setDocPrefix] = useState('')
  const [docRunningFormat, setDocRunningFormat] = useState('')
  const [whCode, setWhCode] = useState('')
  const [shelfCode, setShelfCode] = useState('')
  // VAT inputs are strings so empty = "use server default" (-1 sentinel on save).
  // Number-typed state would coerce '' → 0 and silently lock the channel to 0%.
  const [vatTypeStr, setVatTypeStr] = useState<string>('default')
  const [vatRateStr, setVatRateStr] = useState<string>('')
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
    setDocFormat(row.doc_format_code ?? '')
    setEndpointOverride(row.endpoint ?? '')
    setDocPrefix(row.doc_prefix ?? '')
    setDocRunningFormat(row.doc_running_format ?? '')
    setWhCode(row.wh_code ?? '')
    setShelfCode(row.shelf_code ?? '')
    setVatTypeStr(row.vat_type >= 0 ? String(row.vat_type) : 'default')
    setVatRateStr(row.vat_rate >= 0 ? String(row.vat_rate) : '')
  }, [open, row])

  if (!row) return null

  const isPurchase = row.bill_type === 'purchase'
  const channelLabel = CHANNEL_LABELS[row.channel as ChannelKey] ?? row.channel
  const billTypeLabel = isPurchase ? 'บิลซื้อ' : 'บิลขาย'
  const endpoint = endpointFor(row.channel as ChannelKey, row.bill_type, endpointOverride)
  const autoKind = resolveEndpointKind('', row.channel as ChannelKey, row.bill_type)
  const isOverridden = endpointOverride && endpointOverride !== autoKind

  const handleSave = async () => {
    if (!party) {
      toast.error('กรุณาเลือก' + (isPurchase ? 'ผู้ขาย' : 'ลูกค้า') + 'ก่อน')
      return
    }
    setSaving(true)
    try {
      const vatTypeNum = vatTypeStr === 'default' ? -1 : Number(vatTypeStr)
      const vatRateNum = vatRateStr.trim() === '' ? -1 : Number(vatRateStr)
      await client.put('/api/settings/channel-defaults', {
        channel: row.channel,
        bill_type: row.bill_type,
        party_code: party.code,
        party_name: party.name,
        party_phone: party.telephone ?? '',
        party_address: party.address ?? '',
        party_tax_id: party.tax_id ?? '',
        doc_format_code: endpoint.takesDocFormat ? docFormat.trim() : '',
        endpoint: endpointOverride,
        doc_prefix: docPrefix.trim(),
        doc_running_format: docRunningFormat.trim(),
        wh_code: whCode.trim(),
        shelf_code: shelfCode.trim(),
        vat_type: vatTypeNum,
        vat_rate: Number.isFinite(vatRateNum) ? vatRateNum : -1,
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
      <DialogContent className="grid max-h-[90vh] max-w-lg grid-rows-[auto_minmax(0,1fr)_auto]">
        <DialogHeader>
          <DialogTitle>
            ตั้งค่า{isPurchase ? 'ผู้ขาย' : 'ลูกค้า'} default — {channelLabel} ({billTypeLabel})
          </DialogTitle>
        </DialogHeader>

        <div className="-mx-6 space-y-4 overflow-y-auto px-6 py-2">
          <div className="space-y-1.5">
            <Label>API ที่ส่งเข้า SML (URL หรือ path)</Label>
            <Input
              value={endpointOverride}
              onChange={(e) => setEndpointOverride(e.target.value)}
              placeholder={`เว้นว่างเพื่อใช้ค่าเริ่มต้น — ${
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
              <p className="mt-1 text-[11px] text-muted-foreground">
                ระบบเลือก client โดยจับคำใน URL — มีคำว่า{' '}
                <code>saleorder</code> / <code>saleinvoice</code> /{' '}
                <code>purchaseorder</code> / <code>sale_reserve</code> →
                ใช้ client ตัวนั้น. ใส่ได้ทั้ง path (<code>/SMLJavaRESTService/v3/api/saleorder</code>)
                หรือ URL เต็ม (<code>http://192.168.2.248:8080/...</code>)
              </p>
            </div>
          </div>

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

          {endpoint.takesDocFormat && (
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

          {endpoint.takesDocFormat && (
            <div className="space-y-3 rounded-md border border-border bg-muted/20 p-3">
              <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                คลัง / ภาษี (override จาก server .env)
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <Label className="text-xs">รหัสคลัง (wh_code)</Label>
                  <Input
                    value={whCode}
                    onChange={(e) => setWhCode(e.target.value)}
                    placeholder="เช่น WH-01"
                    className="font-mono"
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">รหัสพื้นที่เก็บ (shelf_code)</Label>
                  <Input
                    value={shelfCode}
                    onChange={(e) => setShelfCode(e.target.value)}
                    placeholder="เช่น SH-01"
                    className="font-mono"
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">VAT Type</Label>
                  <Select value={vatTypeStr} onValueChange={setVatTypeStr}>
                    <SelectTrigger className="h-9 text-sm">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="default">ใช้ค่าจาก server</SelectItem>
                      <SelectItem value="0">0 — แยกนอก</SelectItem>
                      <SelectItem value="1">1 — รวมใน</SelectItem>
                      <SelectItem value="2">2 — ศูนย์%</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">VAT Rate (%)</Label>
                  <Input
                    type="number"
                    step="0.001"
                    value={vatRateStr}
                    onChange={(e) => setVatRateStr(e.target.value)}
                    placeholder="ใช้ค่าจาก server"
                    className="font-mono"
                  />
                </div>
              </div>
              <p className="text-[11px] text-muted-foreground">
                ทุกฟิลด์ปล่อยว่าง / "ใช้ค่าจาก server" = ใช้ค่าใน server <code>.env</code>{' '}
                (<code>SHOPEE_SML_WH_CODE</code>, <code>SHOPEE_SML_SHELF_CODE</code>,{' '}
                <code>SHOPEE_SML_VAT_TYPE</code>, <code>SHOPEE_SML_VAT_RATE</code>) —
                ตั้งเฉพาะเมื่ออยาก override per channel
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

          {party && (
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
          <Button onClick={handleSave} disabled={saving || !party}>
            {saving ? 'กำลังบันทึก…' : 'บันทึก'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
