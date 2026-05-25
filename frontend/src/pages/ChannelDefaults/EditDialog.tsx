import { useEffect, useState } from 'react'
import { PackageSearch } from 'lucide-react'
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
import { Switch } from '@/components/ui/switch'
import { UnitSelect } from '@/components/common/UnitSelect'
import client from '@/api/client'
import type { CatalogMatch } from '@/types'

interface SmlDocFormat {
  code: string
  name_1: string
  name_2: string
  format: string
  screen_code: string
}

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
import { MapItemModal } from '../BillDetail/components/MapItemModal'

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
  const [selectedDocFormatCode, setSelectedDocFormatCode] = useState('')
  const [docFormats, setDocFormats] = useState<SmlDocFormat[]>([])
  const [docFormatsLoading, setDocFormatsLoading] = useState(false)

  // prefix = doc_format code (e.g. "POL"), running format = format field from SML stripped of leading "@"
  // SML uses "@" to mean "prefix with the doc_format code" — BillFlow already does that via doc_prefix
  const parseSmlFormat = (code: string, format: string): { prefix: string; runningFormat: string } => {
    return { prefix: code, runningFormat: format.replace(/^@/, '') }
  }
  const [shippingEnabled, setShippingEnabled] = useState(false)
  const [shippingItemCode, setShippingItemCode] = useState('')
  const [shippingItemUnitCode, setShippingItemUnitCode] = useState('')
  const [shippingItemName, setShippingItemName] = useState('')
  const [shippingPickerOpen, setShippingPickerOpen] = useState(false)
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
    setSelectedDocFormatCode(row.doc_format_code || destination?.docFormatCode || '')
    setShippingEnabled(Boolean(row.shipping_item_enabled))
    setShippingItemCode(row.shipping_item_code || '')
    setShippingItemUnitCode(row.shipping_item_unit_code || '')
    setShippingItemName('')
  }, [open, row])

  // Fetch doc formats from SML when destination changes; auto-fill prefix + running format from selected format
  useEffect(() => {
    if (!open) return
    const screenCodeMap: Record<EndpointKind, string> = {
      saleorder: 'SR',
      saleinvoice: 'SI',
      purchaseorder: 'PO',
    }
    const screenCode = screenCodeMap[selectedDestination]
    if (!screenCode) return
    setDocFormatsLoading(true)
    client.get(`/api/sml/doc-formats?screen_code=${screenCode}`)
      .then((res) => {
        const formats: SmlDocFormat[] = res.data?.data ?? []
        setDocFormats(formats)
        if (formats.length === 0) return
        // Keep current selection if still in list; otherwise default to first
        const current = formats.find((f) => f.code === selectedDocFormatCode)
        const chosen = current ?? formats[0]
        setSelectedDocFormatCode(chosen.code)
        const { prefix, runningFormat } = parseSmlFormat(chosen.code, chosen.format)
        if (prefix) setDocPrefix(prefix)
        if (runningFormat) setDocRunningFormat(runningFormat)
      })
      .catch(() => {
        setDocFormats([])
      })
      .finally(() => setDocFormatsLoading(false))
  }, [open, selectedDestination])

  if (!row) return null

  const isPurchase = row.bill_type === 'purchase'
  const isShopeePurchase = row.channel === 'shopee_shipped' && row.bill_type === 'purchase'
  const channelLabel = isShopeePurchase
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
  const shippingItemCodeTrimmed = shippingItemCode.trim()
  const shippingItemUnitCodeTrimmed = shippingItemUnitCode.trim()
  const docWarning = docNoPatternWarning(docPrefixTrimmed, docRunningFormatTrimmed)
  const canSave =
    !!selectedDestinationMeta &&
    docPrefixTrimmed !== '' &&
    docRunningFormatTrimmed !== '' &&
    docRunningFormatTrimmed.includes('#') &&
    (!isShopeePurchase || !shippingEnabled || shippingItemCodeTrimmed !== '') &&
    !docWarning &&
    !saving

  const handleDestinationChange = (value: EndpointKind) => {
    const destination = destinationOptions.find((option) => option.value === value)
    setSelectedDestination(value)
    setSelectedDocFormatCode('') // reset — useEffect will re-fetch and select first
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
    if (!docPrefixTrimmed || !docRunningFormatTrimmed || !docRunningFormatTrimmed.includes('#')) {
      toast.error('เลือกรูปแบบเอกสารก่อน ระบบจะดึง prefix และรูปแบบเลขรันจาก SML ให้อัตโนมัติ')
      return
    }
    if (docWarning) {
      toast.error('แก้รูปแบบเลขเอกสารตามคำเตือนก่อนบันทึก')
      return
    }
    if (isShopeePurchase && shippingEnabled && !shippingItemCodeTrimmed) {
      toast.error('กรุณาเลือกสินค้า SML สำหรับค่าขนส่งก่อนเปิดใช้งาน')
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
        doc_format_code: selectedDocFormatCode || selectedDestinationMeta.docFormatCode,
        endpoint: selectedDestinationMeta.apiPath,
        doc_prefix: docPrefixTrimmed,
        doc_running_format: docRunningFormatTrimmed,
        branch_code: '',
        sale_code: '',
        unit_code: '',
        doc_time: '',
        shipping_item_enabled: isShopeePurchase ? shippingEnabled : false,
        shipping_item_code: isShopeePurchase ? shippingItemCodeTrimmed : '',
        shipping_item_unit_code: isShopeePurchase ? shippingItemUnitCodeTrimmed : '',
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

  const handleShippingPick = (code: string, unitCode: string, picked?: CatalogMatch) => {
    setShippingItemCode(code)
    setShippingItemUnitCode(unitCode || '')
    setShippingItemName(picked?.item_name || '')
    setShippingPickerOpen(false)
  }

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(v) => {
          if (!v) setShippingPickerOpen(false)
          onOpenChange(v)
        }}
      >
        <DialogContent className="grid max-h-[90vh] max-w-xl grid-rows-[auto_minmax(0,1fr)_auto]">
          <DialogHeader>
            <DialogTitle>
              ตั้งค่าเส้นทาง SML สำหรับ {channelLabel} ({billTypeLabel})
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
              <Label>รูปแบบเอกสาร</Label>
              {docFormatsLoading ? (
                <div className="rounded-md border border-border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
                  กำลังโหลด...
                </div>
              ) : docFormats.length > 0 ? (
                <Select
                  value={selectedDocFormatCode}
                  onValueChange={(code) => {
                    setSelectedDocFormatCode(code)
                    const fmt = docFormats.find((f) => f.code === code)
                    if (fmt) {
                      const { prefix, runningFormat } = parseSmlFormat(fmt.code, fmt.format)
                      if (prefix) setDocPrefix(prefix)
                      if (runningFormat) setDocRunningFormat(runningFormat)
                    }
                  }}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="เลือกรูปแบบเอกสาร" />
                  </SelectTrigger>
                  <SelectContent>
                    {docFormats.map((fmt) => (
                      <SelectItem key={fmt.code} value={fmt.code}>
                        <span className="font-mono font-semibold">{fmt.code}</span>
                        <span className="ml-2 text-muted-foreground">— {fmt.name_1}</span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <div className="rounded-md border border-border bg-muted/30 px-3 py-2 font-mono text-sm text-foreground">
                  {selectedDocFormatCode || selectedDestinationMeta?.docFormatCode || '-'}
                </div>
              )}
              <p className="text-xs text-muted-foreground">
                {docFormats.length > 0
                  ? `ดึงจาก erp_doc_format ใน SML (${docFormats.length} รายการ)`
                  : 'ค่า default จากปลายทาง SML ที่เลือกไว้'}
              </p>
            </div>

            <div className="space-y-3 rounded-md border border-border bg-muted/20 p-3">
              <div className="flex items-center justify-between">
                <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  เลขเอกสาร (doc_no)
                </div>
                <span className="text-[10px] text-muted-foreground">ดึงจากรูปแบบเอกสารที่เลือก</span>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <Label className="text-xs text-muted-foreground">รหัสขึ้นต้น (prefix)</Label>
                  <div className="rounded-md border border-dashed border-border bg-muted/40 px-3 py-2 font-mono text-sm text-foreground">
                    {docPrefixTrimmed || <span className="text-muted-foreground">—</span>}
                  </div>
                </div>
                <div className="space-y-1">
                  <Label className="text-xs text-muted-foreground">รูปแบบเลขรัน</Label>
                  <div className="rounded-md border border-dashed border-border bg-muted/40 px-3 py-2 font-mono text-sm text-foreground">
                    {docRunningFormatTrimmed || <span className="text-muted-foreground">—</span>}
                  </div>
                </div>
              </div>
              <div className="text-xs text-muted-foreground">
                <b>ตัวอย่างถัดไป:</b>{' '}
                <code className="rounded bg-background px-1.5 py-0.5 font-mono text-foreground">
                  {previewDocNo(docPrefixTrimmed || 'BF', docRunningFormatTrimmed || 'YYMM####')}
                </code>
              </div>
            </div>

            {isShopeePurchase && (
              <div className="space-y-3 rounded-md border border-border bg-muted/20 p-3">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      ค่าขนส่งจาก Shopee
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      ถ้าเปิดใช้ ระบบจะเพิ่มค่าส่งจากเมล Shopee เป็นรายการสินค้าในบิลซื้อใหม่.
                      ถ้าปิดไว้ จะไม่เพิ่มรายการค่าส่งใด ๆ
                    </p>
                  </div>
                  <Switch
                    checked={shippingEnabled}
                    onCheckedChange={setShippingEnabled}
                    aria-label="เพิ่มค่าขนส่งเป็นรายการสินค้า"
                  />
                </div>

                <div className={shippingEnabled ? 'space-y-3' : 'space-y-3 opacity-60'}>
                  <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto]">
                    <div className="space-y-1">
                      <Label className="text-xs">สินค้า SML สำหรับค่าส่ง</Label>
                      <div className="rounded-md border border-border bg-background px-3 py-2">
                        {shippingItemCodeTrimmed ? (
                          <div className="min-w-0">
                            <code className="font-mono text-sm font-semibold text-foreground">
                              {shippingItemCodeTrimmed}
                            </code>
                            <div className="mt-0.5 truncate text-xs text-muted-foreground">
                              {shippingItemName || 'เลือกไว้แล้ว ระบบจะใช้ชื่อสินค้าจาก SML ตอนแสดงในบิล'}
                            </div>
                          </div>
                        ) : (
                          <span className="text-sm text-muted-foreground">ยังไม่ได้เลือกสินค้า</span>
                        )}
                      </div>
                    </div>
                    <div className="flex items-end">
                      <Button
                        type="button"
                        variant="outline"
                        className="gap-2"
                        onClick={() => setShippingPickerOpen(true)}
                        disabled={!shippingEnabled}
                      >
                        <PackageSearch className="h-4 w-4" />
                        เลือกสินค้า
                      </Button>
                    </div>
                  </div>

                  <div className="space-y-1.5">
                    <Label className="text-xs">หน่วย</Label>
                    <UnitSelect
                      value={shippingItemUnitCode}
                      onValueChange={setShippingItemUnitCode}
                      productCode={shippingItemCodeTrimmed}
                      disabled={!shippingEnabled || !shippingItemCodeTrimmed}
                      autoSelectSingle
                    />
                  </div>
                </div>

                {shippingEnabled && !shippingItemCodeTrimmed && (
                  <div className="rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">
                    ต้องเลือกสินค้า SML ก่อนบันทึก เช่น สินค้าบริการที่ร้านตั้งไว้สำหรับค่าขนส่ง
                  </div>
                )}
              </div>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              ยกเลิก
            </Button>
            <Button onClick={handleSave} disabled={!canSave}>
              {saving ? 'กำลังบันทึก...' : 'บันทึก'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {isShopeePurchase && (
        <MapItemModal
          open={open && shippingPickerOpen}
          rawName="ค่าขนส่งสินค้า"
          currentCode={shippingItemCode}
          currentUnit={shippingItemUnitCode}
          currentPrice={0}
          rawNameLabel="รายการค่าส่งจาก Shopee"
          onPick={handleShippingPick}
          onClose={() => setShippingPickerOpen(false)}
        />
      )}
    </>
  )
}
