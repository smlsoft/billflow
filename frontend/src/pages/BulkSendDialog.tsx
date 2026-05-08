import { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, CheckCircle2, Loader2, Send } from 'lucide-react'

import client from '@/api/client'
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
import { PartyPicker, type Party } from '@/pages/ChannelDefaults/PartyPicker'
import { ShelfPicker, WarehousePicker } from '@/pages/BillDetail/components/WarehousePicker'
import { getBill, retryBill, type RetryBillPayload } from '@/hooks/useBills'
import type { Bill } from '@/types'
import { validateForSML, issueLabel } from '@/pages/BillDetail/utils/validation'

function currentTimeHHMM() {
  const now = new Date()
  return `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`
}

type Candidate = {
  bill: Bill
  ready: boolean
  issues: string[]
  result?: 'sent' | 'failed' | 'skipped'
  message?: string
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  billType: 'purchase' | 'sale'
  filters: {
    source: string
    bill_type: 'purchase' | 'sale'
    document_route?: string
  }
  onDone?: () => void
}

export function BulkSendDialog({
  open,
  onOpenChange,
  title,
  billType,
  filters,
  onDone,
}: Props) {
  const [loading, setLoading] = useState(false)
  const [sending, setSending] = useState(false)
  const [party, setParty] = useState<Party | null>(null)
  const [docTime, setDocTime] = useState(currentTimeHHMM())
  const [whCode, setWhCode] = useState('')
  const [shelfCode, setShelfCode] = useState('')
  const [manualWarehouse, setManualWarehouse] = useState(false)
  const [vatTypeStr, setVatTypeStr] = useState('')
  const [vatRateStr, setVatRateStr] = useState('7')
  const [branchCode, setBranchCode] = useState('')
  const [saleCode, setSaleCode] = useState('')
  const [remark, setRemark] = useState('')
  const [candidates, setCandidates] = useState<Candidate[]>([])
  const [totalPending, setTotalPending] = useState(0)

  const readyCount = candidates.filter((c) => c.ready).length
  const skippedCount = candidates.length - readyCount
  const sentCount = candidates.filter((c) => c.result === 'sent').length
  const failedCount = candidates.filter((c) => c.result === 'failed').length
  const vatRateNum = Number(vatRateStr)
  const canSend =
    readyCount > 0 &&
    !!party?.code &&
    whCode.trim() !== '' &&
    shelfCode.trim() !== '' &&
    vatTypeStr !== '' &&
    vatRateStr.trim() !== '' &&
    Number.isFinite(vatRateNum) &&
    docTime.trim() !== '' &&
    !sending

  const destination = useMemo(() => {
    if (filters.document_route === 'saleinvoice') return 'ขาย -> ขายสินค้าและบริการ'
    if (filters.document_route === 'saleorder') return 'ขาย -> ใบสั่งขาย'
    return 'ซื้อ -> ใบสั่งซื้อ'
  }, [filters.document_route])

  useEffect(() => {
    if (!open) return
    let alive = true
    setLoading(true)
    setSending(false)
    setParty(null)
    setDocTime(currentTimeHHMM())
    setWhCode('')
    setShelfCode('')
    setManualWarehouse(false)
    setVatTypeStr('')
    setVatRateStr('7')
    setBranchCode('')
    setSaleCode('')
    setRemark('')
    setCandidates([])
    setTotalPending(0)

    async function load() {
      try {
        const params = new URLSearchParams({
          source: filters.source,
          bill_type: filters.bill_type,
          status: 'pending',
          page: '1',
          per_page: '100',
        })
        if (filters.document_route) params.set('document_route', filters.document_route)
        const res = await client.get<{ data: Bill[]; total: number }>(`/api/bills?${params}`)
        const list = res.data.data ?? []
        const details = await Promise.all(list.map((b) => getBill(b.id)))
        const rows = details.map((bill) => {
          const validation = validateForSML(bill)
          return {
            bill,
            ready: validation.canSend,
            issues: validation.issues.map((issue) => `${issue.count} รายการ${issueLabel(issue.kind)}`),
          }
        })
        if (!alive) return
        setTotalPending(res.data.total ?? rows.length)
        setCandidates(rows)
      } catch (err) {
        if (!alive) return
        setCandidates([{
          bill: {
            id: 'load-error',
            bill_type: billType,
            source: filters.source,
            status: 'failed',
            created_at: new Date().toISOString(),
          } as Bill,
          ready: false,
          issues: [err instanceof Error ? err.message : 'โหลดรายการไม่สำเร็จ'],
        }])
      } finally {
        if (alive) setLoading(false)
      }
    }
    load()
    return () => {
      alive = false
    }
  }, [open, filters.source, filters.bill_type, filters.document_route, billType])

  const payload = (): RetryBillPayload => ({
    party_code: party?.code,
    party_name: party?.name,
    remark: remark.trim() || undefined,
    branch_code: branchCode.trim() || undefined,
    sale_code: saleCode.trim() || undefined,
    doc_time: docTime.trim(),
    wh_code: whCode.trim(),
    shelf_code: shelfCode.trim(),
    vat_type: Number(vatTypeStr),
    vat_rate: vatRateNum,
  })

  const handleSend = async () => {
    if (!canSend) return
    setSending(true)
    const body = payload()
    for (const row of candidates) {
      if (!row.ready) {
        setCandidates((prev) =>
          prev.map((c) => c.bill.id === row.bill.id ? { ...c, result: 'skipped', message: 'ยังไม่พร้อมส่ง' } : c),
        )
        continue
      }
      try {
        await retryBill(row.bill.id, body)
        setCandidates((prev) =>
          prev.map((c) => c.bill.id === row.bill.id ? { ...c, result: 'sent', message: 'ส่งสำเร็จ' } : c),
        )
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'ส่งไม่สำเร็จ'
        setCandidates((prev) =>
          prev.map((c) => c.bill.id === row.bill.id ? { ...c, result: 'failed', message: msg } : c),
        )
      }
    }
    setSending(false)
    onDone?.()
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!sending) onOpenChange(v) }}>
      <DialogContent className="grid max-h-[92vh] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>ส่ง SML ทั้งหมด: {title}</DialogTitle>
        </DialogHeader>

        <div className="-mx-6 space-y-4 overflow-y-auto px-6 py-2">
          <div className="rounded-md border border-info/25 bg-info/[0.04] px-3 py-2 text-xs text-muted-foreground">
            <div className="font-medium text-foreground">ปลายทาง SML: {destination}</div>
            <div className="mt-0.5">
              ระบบจะส่งเฉพาะเอกสารสถานะพร้อมส่ง และใช้ค่าชุดนี้ร่วมกันทุกเอกสารในรอบนี้
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5 sm:col-span-2">
              <Label>{billType === 'sale' ? 'ลูกค้า' : 'ผู้ขาย'} <span className="text-destructive">*</span></Label>
              <PartyPicker billType={billType} value={party} onChange={setParty} />
            </div>

            <div className="space-y-1">
              <Label className="text-xs">เวลาเอกสาร <span className="text-destructive">*</span></Label>
              <Input value={docTime} onChange={(e) => setDocTime(e.target.value)} className="font-mono" />
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

            <div className="space-y-1">
              <div className="flex items-center justify-between gap-2">
                <Label className="text-xs">คลัง <span className="text-destructive">*</span></Label>
                <Button type="button" variant="ghost" size="sm" className="h-6 px-1.5 text-[11px]" onClick={() => setManualWarehouse((v) => !v)}>
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
                <ShelfPicker warehouseCode={whCode} value={shelfCode} onChange={(shelf) => setShelfCode(shelf.code)} />
              )}
            </div>

            <div className="space-y-1">
              <Label className="text-xs">อัตราภาษี (%) <span className="text-destructive">*</span></Label>
              <Input type="number" step="0.001" value={vatRateStr} onChange={(e) => setVatRateStr(e.target.value)} className="font-mono" />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">Branch code</Label>
              <Input value={branchCode} onChange={(e) => setBranchCode(e.target.value)} className="font-mono" placeholder="ไม่บังคับ" />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">Sale code</Label>
              <Input value={saleCode} onChange={(e) => setSaleCode(e.target.value)} className="font-mono" placeholder="ไม่บังคับ" />
            </div>
            <div className="space-y-1 sm:col-span-2">
              <Label className="text-xs">หมายเหตุ</Label>
              <textarea
                value={remark}
                onChange={(e) => setRemark(e.target.value)}
                rows={2}
                className="w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                placeholder="หมายเหตุสำหรับ SML (ถ้ามี)"
              />
            </div>
          </div>

          <div className="rounded-md border border-border">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-3 py-2 text-xs">
              <div className="font-medium text-foreground">ตรวจรายการพร้อมส่ง</div>
              <div className="flex gap-2 text-muted-foreground">
                <span>พร้อมส่ง {readyCount}</span>
                <span>ต้องข้าม {skippedCount}</span>
                {totalPending > candidates.length && <span>โหลด 100/{totalPending}</span>}
              </div>
            </div>
            <div className="max-h-56 overflow-y-auto divide-y divide-border">
              {loading ? (
                <div className="flex items-center gap-2 px-3 py-4 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  กำลังตรวจเอกสารพร้อมส่ง…
                </div>
              ) : candidates.length === 0 ? (
                <div className="px-3 py-4 text-sm text-muted-foreground">ไม่มีเอกสารสถานะพร้อมส่งในเมนูนี้</div>
              ) : (
                candidates.map((row) => (
                  <div key={row.bill.id} className="grid gap-2 px-3 py-2 text-xs sm:grid-cols-[1fr_auto]">
                    <div className="min-w-0">
                      <div className="font-mono text-foreground">{row.bill.sml_doc_no || row.bill.id.slice(0, 8)}</div>
                      <div className="truncate text-muted-foreground">
                        {row.ready ? 'ผ่าน validation' : row.issues.join(' · ')}
                      </div>
                    </div>
                    <div className="flex items-center gap-1 justify-end">
                      {row.result === 'sent' ? (
                        <span className="inline-flex items-center gap-1 text-success"><CheckCircle2 className="h-3.5 w-3.5" />สำเร็จ</span>
                      ) : row.result === 'failed' ? (
                        <span className="inline-flex items-center gap-1 text-destructive"><AlertTriangle className="h-3.5 w-3.5" />ไม่สำเร็จ</span>
                      ) : row.ready ? (
                        <span className="text-success">พร้อม</span>
                      ) : (
                        <span className="text-warning">ข้าม</span>
                      )}
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>

        <DialogFooter className="items-center gap-2 sm:justify-between">
          <div className="text-xs text-muted-foreground">
            {sending ? `ส่งแล้ว ${sentCount} · ไม่สำเร็จ ${failedCount}` : 'doc_no จะ running แยกต่อใบตอนส่งจริง'}
          </div>
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={sending}>
              ปิด
            </Button>
            <Button type="button" onClick={handleSend} disabled={!canSend} className="gap-2">
              {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
              ส่ง SML {readyCount} รายการ
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
