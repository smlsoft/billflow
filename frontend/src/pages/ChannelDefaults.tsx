import { useEffect, useMemo, useState } from 'react'
import {
  AlertCircle,
  ChevronDown,
  Info,
  Pencil,
  RefreshCw,
  Sparkles,
  Trash2,
} from 'lucide-react'
import dayjs from 'dayjs'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { DataTable } from '@/components/common/DataTable'
import { PageHeader } from '@/components/common/PageHeader'
import client from '@/api/client'
import { cn } from '@/lib/utils'

import { EditDialog } from './ChannelDefaults/EditDialog'
import {
  CHANNEL_LABELS,
  CHANNEL_SLOTS,
  endpointFor,
  type ChannelDefaultRow,
  type ChannelKey,
} from './ChannelDefaults/labels'

function HelpBanner() {
  const [open, setOpen] = useState(true)
  return (
    <Card className="border-info/30 bg-info/5">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger className="flex w-full items-center gap-2 px-4 py-3 text-left text-sm font-medium text-foreground hover:bg-info/10">
          <Info className="h-4 w-4 text-info" />
          <span>หน้านี้ใช้ทำอะไร — อ่านก่อนตั้งค่า</span>
          <ChevronDown
            className={cn(
              'ml-auto h-4 w-4 text-muted-foreground transition-transform',
              open && 'rotate-180',
            )}
          />
        </CollapsibleTrigger>
        <CollapsibleContent>
          <CardContent className="space-y-3 border-t border-info/20 px-4 pt-3 text-sm">
            <p className="text-muted-foreground">
              บิลทุกใบที่ระบบรับเข้ามา (LINE / Email / Shopee / Lazada) สุดท้ายต้องส่งเข้า{' '}
              <b>SML ERP</b> เพื่อบันทึก. หน้านี้กำหนดว่า <b>"แต่ละช่องทาง"</b> จะ:
            </p>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div className="rounded-md border border-border bg-card p-3">
                <div className="mb-1 text-sm font-semibold">1. ส่งเข้า API ตัวไหน</div>
                <p className="text-xs text-muted-foreground">
                  เลือกระหว่าง <code>ใบสั่งขาย</code>, <code>ใบกำกับภาษี</code>,{' '}
                  <code>ใบสั่งซื้อ/สั่งจอง</code>, <code>ใบจอง</code> — แต่ละตัวเก็บที่
                  เมนูคนละจุดใน SML
                </p>
              </div>
              <div className="rounded-md border border-border bg-card p-3">
                <div className="mb-1 text-sm font-semibold">2. doc_format_code</div>
                <p className="text-xs text-muted-foreground">
                  รหัสประเภทเอกสารใน SML — เช่น <code>SR</code> สำหรับใบสั่งขาย,{' '}
                  <code>PO</code> สำหรับใบสั่งซื้อ. ตั้ง code ที่ตรงกับที่ SML
                  ของลูกค้าใช้
                </p>
              </div>
              <div className="rounded-md border border-border bg-card p-3">
                <div className="mb-1 text-sm font-semibold">3. ลูกค้า / ผู้ขาย</div>
                <p className="text-xs text-muted-foreground">
                  เลือกรหัสลูกค้า (AR-xxx) หรือผู้ขาย (V-xxx) จาก SML —
                  ทุกบิลที่มาจาก channel นี้จะ link ไปที่รหัสที่ตั้งไว้
                </p>
              </div>
            </div>
            <p className="rounded-md bg-warning/10 px-3 py-2 text-xs text-warning">
              <b>⚠️ ถ้าตั้งไม่ครบ:</b> บิลที่ retry/auto-confirm จะ <b>fail</b>{' '}
              ทันทีพร้อม error "ยังไม่ได้ตั้งค่าลูกค้า default" — ใช้ปุ่ม "ตั้งค่าอัตโนมัติ"
              ให้ระบบ pair LINE/Email/Shopee กับ AR00001-04 ใน SML เป็น default แรก
              แล้วค่อย edit แต่ละแถวต่อ
            </p>
          </CardContent>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  )
}

interface QuickSetupResult {
  channel: string
  bill_type: string
  applied: boolean
  party_code?: string
  party_name?: string
  reason?: string
}

interface SyncStatus {
  customers: number
  suppliers: number
  last_sync: string | null
}

export default function ChannelDefaults() {
  const [rows, setRows] = useState<ChannelDefaultRow[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<ChannelDefaultRow | null>(null)
  const [editOpen, setEditOpen] = useState(false)
  const [deleting, setDeleting] = useState<ChannelDefaultRow | null>(null)
  const [quickRunning, setQuickRunning] = useState(false)
  const [sync, setSync] = useState<SyncStatus | null>(null)

  const load = async () => {
    setLoading(true)
    try {
      const r = await client.get<{ data: ChannelDefaultRow[] }>(
        '/api/settings/channel-defaults',
      )
      setRows(r.data.data ?? [])
    } catch (e: any) {
      toast.error('โหลดข้อมูลไม่สำเร็จ: ' + (e?.message ?? 'unknown'))
    } finally {
      setLoading(false)
    }
  }

  const loadSync = async () => {
    try {
      const r = await client.get<SyncStatus>('/api/sml/parties/last-sync')
      setSync(r.data)
    } catch {
      // ignore — not critical
    }
  }

  useEffect(() => {
    load()
    loadSync()
  }, [])

  // Merge DB rows with the static slot list so unset channels show up too
  // (instead of "no data" — we need to nudge admin to set them).
  const tableRows = useMemo(() => {
    const byKey = new Map<string, ChannelDefaultRow>()
    for (const r of rows) {
      byKey.set(`${r.channel}/${r.bill_type}`, r)
    }
    return CHANNEL_SLOTS.map((slot) => {
      const existing = byKey.get(`${slot.channel}/${slot.bill_type}`)
      if (existing) return existing
      return {
        channel: slot.channel,
        bill_type: slot.bill_type,
        party_code: '',
        party_name: '',
        party_phone: '',
        party_address: '',
        party_tax_id: '',
        doc_format_code: '',
        endpoint: '',
        doc_prefix: '',
        doc_running_format: '',
        wh_code: '',
        shelf_code: '',
        vat_type: -1,
        vat_rate: -1,
      } satisfies ChannelDefaultRow
    })
  }, [rows])

  const unsetCount = tableRows.filter((r) => !r.party_code).length

  const handleQuickSetup = async () => {
    setQuickRunning(true)
    try {
      const r = await client.post<{
        applied: number
        results: QuickSetupResult[]
      }>('/api/settings/channel-defaults/quick-setup')
      const applied = r.data.applied
      const skipped = (r.data.results ?? []).filter((x) => !x.applied).length
      toast.success(`ตั้งค่าสำเร็จ ${applied} channel · ข้าม ${skipped}`)
      await load()
    } catch (e: any) {
      toast.error('Quick setup ล้มเหลว: ' + (e?.response?.data?.error ?? e?.message ?? 'unknown'))
    } finally {
      setQuickRunning(false)
    }
  }

  const handleRefreshParties = async () => {
    try {
      const r = await client.post<SyncStatus>('/api/sml/refresh-parties')
      setSync(r.data)
      toast.success(
        `ซิงก์เสร็จ — ${r.data.customers} ลูกค้า / ${r.data.suppliers} ผู้ขาย`,
      )
    } catch (e: any) {
      toast.error('รีเฟรชไม่สำเร็จ: ' + (e?.response?.data?.error ?? e?.message ?? 'unknown'))
    }
  }

  const handleDelete = async () => {
    if (!deleting) return
    try {
      await client.delete(
        `/api/settings/channel-defaults/${deleting.channel}/${deleting.bill_type}`,
      )
      toast.success('ลบสำเร็จ')
      setDeleting(null)
      await load()
    } catch (e: any) {
      toast.error('ลบไม่สำเร็จ: ' + (e?.response?.data?.error ?? e?.message ?? 'unknown'))
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="ลูกค้า / ผู้ขายต่อช่องทาง"
        description="กำหนดว่าแต่ละช่องทางจะส่งบิลเข้า API ของ SML ตัวไหน + บันทึกไปที่ลูกค้า (หรือผู้ขาย) รหัสไหน"
        actions={
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            onClick={handleRefreshParties}
          >
            <RefreshCw className="h-3.5 w-3.5" />
            รีเฟรชจาก SML
          </Button>
        }
      />

      <HelpBanner />

      {unsetCount > 0 && (
        <Card className="border-warning/40 bg-warning/5">
          <CardContent className="flex flex-wrap items-center gap-3 px-4 py-3">
            <Sparkles className="h-5 w-5 shrink-0 text-warning" />
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium text-foreground">
                Quick setup — ตั้งค่าอัตโนมัติจาก placeholder ใน SML
              </div>
              <div className="mt-0.5 text-xs text-muted-foreground">
                ระบบจะค้นหาลูกค้าชื่อ <span className="font-mono text-foreground">"ลูกค้า จาก AI / Line / Email / Shopee"</span>{' '}
                ใน SML 248 (ปกติ AR00001–04) แล้ว pair เข้าทุก channel ที่ยังไม่ตั้งค่า — ไม่กระทบ row ที่มีค่าอยู่แล้ว
              </div>
            </div>
            <Button
              onClick={handleQuickSetup}
              disabled={quickRunning}
              size="sm"
              title="ค้นหา AR00001–04 ใน SML แล้วผูกกับ channel ที่ยังว่าง — ปลอดภัย ไม่ทับของเดิม"
            >
              {quickRunning ? 'กำลังตั้งค่า…' : 'ตั้งค่าอัตโนมัติ'}
            </Button>
          </CardContent>
        </Card>
      )}

      <DataTable<ChannelDefaultRow>
        data={tableRows}
        loading={loading}
        empty="ยังไม่มี channel ที่ตั้งค่า"
        columns={[
          {
            key: 'channel',
            header: 'ช่องทาง',
            cell: (r) => (
              <span className="font-medium">
                {CHANNEL_LABELS[r.channel as ChannelKey] ?? r.channel}
              </span>
            ),
          },
          {
            key: 'bill_type',
            header: 'ประเภท',
            cell: (r) => (
              <Badge
                variant="secondary"
                className={cn(
                  'h-5 px-1.5 text-[10px] font-medium',
                  r.bill_type === 'purchase'
                    ? 'bg-warning/15 text-warning hover:bg-warning/20'
                    : 'bg-info/15 text-info hover:bg-info/20',
                )}
              >
                {r.bill_type === 'purchase' ? 'บิลซื้อ' : 'บิลขาย'}
              </Badge>
            ),
          },
          {
            key: 'party',
            header: 'ลูกค้า / ผู้ขาย',
            cell: (r) =>
              r.party_code ? (
                <div className="flex flex-col">
                  <span className="font-mono text-xs font-medium text-foreground">
                    {r.party_code}
                  </span>
                  <span className="text-xs text-muted-foreground">{r.party_name}</span>
                </div>
              ) : (
                <span className="flex items-center gap-1.5 text-xs text-warning">
                  <AlertCircle className="h-3.5 w-3.5" />
                  ยังไม่ตั้งค่า
                </span>
              ),
          },
          {
            key: 'endpoint',
            header: 'ส่งเข้า SML',
            cell: (r) => {
              const ep = endpointFor(r.channel as ChannelKey, r.bill_type, r.endpoint ?? '')
              const overridden = !!r.endpoint
              return (
                <div className="flex flex-col">
                  <div className="flex items-center gap-1.5">
                    <span className="text-xs font-medium text-foreground">{ep.label}</span>
                    {overridden && (
                      <span className="rounded bg-info/10 px-1 py-0.5 text-[9px] font-medium uppercase text-info">
                        ตั้งเอง
                      </span>
                    )}
                  </div>
                  <code className="text-[10px] text-muted-foreground">
                    {ep.apiPath}
                  </code>
                </div>
              )
            },
          },
          {
            key: 'doc_format',
            header: 'doc format',
            cell: (r) => {
              const ep = endpointFor(r.channel as ChannelKey, r.bill_type, r.endpoint ?? '')
              if (!ep.takesDocFormat) {
                return <span className="text-xs text-muted-foreground/60">—</span>
              }
              return r.doc_format_code ? (
                <span className="font-mono text-xs font-medium text-foreground">
                  {r.doc_format_code}
                </span>
              ) : (
                <span className="text-xs text-warning">ยังไม่ตั้ง</span>
              )
            },
          },
          {
            key: 'updated',
            header: 'อัปเดต',
            cell: (r) =>
              r.updated_at ? (
                <span className="text-xs tabular-nums text-muted-foreground">
                  {dayjs(r.updated_at).format('DD/MM/YY HH:mm')}
                </span>
              ) : (
                <span className="text-xs text-muted-foreground">—</span>
              ),
          },
          {
            key: 'actions',
            header: '',
            headerClassName: 'text-right',
            className: 'text-right',
            cell: (r) => (
              <div className="flex items-center justify-end gap-1.5">
                <Button
                  variant={r.party_code ? 'outline' : 'default'}
                  size="sm"
                  className="h-7 gap-1 px-2.5 text-xs"
                  onClick={(e) => {
                    e.stopPropagation()
                    setEditing(r)
                    setEditOpen(true)
                  }}
                  title="แก้ไข endpoint, ลูกค้า, doc format, prefix และเลขรัน"
                >
                  <Pencil className="h-3.5 w-3.5" />
                  {r.party_code ? 'แก้ไข' : 'ตั้งค่า'}
                </Button>
                {r.party_code && (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 px-2 text-destructive hover:text-destructive"
                    onClick={(e) => {
                      e.stopPropagation()
                      setDeleting(r)
                    }}
                    title="ลบการตั้งค่านี้"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                )}
              </div>
            ),
          },
        ]}
      />

      <div className="text-xs text-muted-foreground">
        {sync && (
          <>
            แคช SML: {sync.customers.toLocaleString()} ลูกค้า ·{' '}
            {sync.suppliers.toLocaleString()} ผู้ขาย
            {sync.last_sync && (
              <> · ซิงก์ล่าสุด {dayjs(sync.last_sync).format('DD/MM/YY HH:mm')}</>
            )}
          </>
        )}
      </div>

      <EditDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        row={editing}
        onSaved={load}
      />

      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(v) => !v && setDeleting(null)}
        title="ลบการตั้งค่า"
        description={
          deleting
            ? `ลบการตั้งค่าของ ${CHANNEL_LABELS[deleting.channel as ChannelKey] ?? deleting.channel} (${
                deleting.bill_type === 'purchase' ? 'บิลซื้อ' : 'บิลขาย'
              })?`
            : ''
        }
        confirmLabel="ลบ"
        variant="destructive"
        onConfirm={handleDelete}
      />
    </div>
  )
}
