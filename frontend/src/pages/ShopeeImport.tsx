import { useState, useRef, useEffect, Fragment } from 'react'
import { Link } from 'react-router-dom'
import {
  AlertCircle,
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  FileSpreadsheet,
  Info,
  Settings as SettingsIcon,
} from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { PageHeader } from '@/components/common/PageHeader'
import client from '@/api/client'
import { cn } from '@/lib/utils'

interface ShopeeConfig {
  server_url: string
  guid: string
  provider: string
  config_file_name: string
  database_name: string
  doc_format_code: string
  cust_code: string
  sale_code: string
  branch_code: string
  wh_code: string
  shelf_code: string
  unit_code: string
  vat_type: number
  vat_rate: number
  doc_time: string
}

interface ShopeeOrderItem {
  sku: string
  product_name: string
  price: number
  qty: number
}
interface ShopeeOrder {
  order_id: string
  doc_date: string
  status: string
  items: ShopeeOrderItem[]
  item_count: number
  total_qty: number
  duplicate: boolean
}
interface PreviewResponse {
  orders: ShopeeOrder[]
  warnings: string[]
  total_orders: number
  duplicate_count: number
  skipped_count: number
  file_token?: string
}
interface ConfirmResult {
  order_id: string
  success: boolean
  bill_id?: string
  doc_no?: string
  message?: string
}

function fmt(n: number) {
  return n.toLocaleString('th-TH', { minimumFractionDigits: 2 })
}

function ConfigField({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (v: string) => void
}) {
  return (
    <div className="space-y-1">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      <Input value={value} onChange={(e) => onChange(e.target.value)} className="h-8 text-sm" />
    </div>
  )
}

function ConfigDialog({
  config,
  open,
  onOpenChange,
  onSave,
}: {
  config: ShopeeConfig
  open: boolean
  onOpenChange: (o: boolean) => void
  onSave: (c: ShopeeConfig) => void
}) {
  const [cfg, setCfg] = useState<ShopeeConfig>(config)

  useEffect(() => {
    if (open) setCfg(config)
  }, [open, config])

  const set = (k: keyof ShopeeConfig, v: string | number) =>
    setCfg((p) => ({ ...p, [k]: v }))

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>ตั้งค่า Shopee SML</DialogTitle>
          <DialogDescription>
            ค่าทั้งหมดถูก save บน server — ทุกครั้งที่นำเข้าไฟล์จะใช้ config นี้
          </DialogDescription>
        </DialogHeader>
        <div className="grid grid-cols-2 gap-3">
          <ConfigField label="Server URL" value={cfg.server_url} onChange={(v) => set('server_url', v)} />
          <ConfigField label="Doc Format Code" value={cfg.doc_format_code} onChange={(v) => set('doc_format_code', v)} />
          <ConfigField label="GUID" value={cfg.guid} onChange={(v) => set('guid', v)} />
          <ConfigField label="Provider" value={cfg.provider} onChange={(v) => set('provider', v)} />
          <ConfigField label="Config File Name" value={cfg.config_file_name} onChange={(v) => set('config_file_name', v)} />
          <ConfigField label="Database Name" value={cfg.database_name} onChange={(v) => set('database_name', v)} />
          <ConfigField label="รหัสลูกค้า (Cust Code)" value={cfg.cust_code} onChange={(v) => set('cust_code', v)} />
          <ConfigField label="รหัสพนักงานขาย (Sale Code)" value={cfg.sale_code} onChange={(v) => set('sale_code', v)} />
          <ConfigField label="รหัสสาขา (Branch Code)" value={cfg.branch_code} onChange={(v) => set('branch_code', v)} />
          <ConfigField label="รหัสคลัง (WH Code)" value={cfg.wh_code} onChange={(v) => set('wh_code', v)} />
          <ConfigField label="รหัสชั้นวาง (Shelf Code)" value={cfg.shelf_code} onChange={(v) => set('shelf_code', v)} />
          <ConfigField label="หน่วย (Unit Code)" value={cfg.unit_code} onChange={(v) => set('unit_code', v)} />
          <ConfigField label="เวลาเอกสาร (Doc Time)" value={cfg.doc_time} onChange={(v) => set('doc_time', v)} />
          <div className="space-y-1">
            <Label className="text-xs text-muted-foreground">VAT Type</Label>
            <Select
              value={String(cfg.vat_type)}
              onValueChange={(v) => set('vat_type', Number(v))}
            >
              <SelectTrigger className="h-8 text-sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="0">0 — แยกนอก</SelectItem>
                <SelectItem value="1">1 — รวมใน</SelectItem>
                <SelectItem value="2">2 — ศูนย์%</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1">
            <Label className="text-xs text-muted-foreground">VAT Rate (%)</Label>
            <Input
              type="number"
              value={cfg.vat_rate}
              onChange={(e) => set('vat_rate', Number(e.target.value))}
              className="h-8 text-sm"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            ยกเลิก
          </Button>
          <Button onClick={() => onSave(cfg)}>บันทึก</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function SummaryCard({
  label,
  value,
  variant = 'muted',
}: {
  label: string
  value: number
  variant?: 'success' | 'danger' | 'primary' | 'muted'
}) {
  const tone: Record<typeof variant, string> = {
    success: 'border-success/30 bg-success/5 text-success',
    danger: 'border-destructive/30 bg-destructive/5 text-destructive',
    primary: 'border-primary/30 bg-primary/5 text-primary',
    muted: 'border-border bg-muted/30 text-foreground',
  }
  return (
    <Card className={cn('text-center', tone[variant])}>
      <CardContent className="p-4">
        <p className="text-3xl font-semibold tabular-nums">{value}</p>
        <p className="mt-1 text-xs font-medium text-muted-foreground">{label}</p>
      </CardContent>
    </Card>
  )
}

type Step = 'idle' | 'uploading' | 'preview' | 'confirming' | 'done'

export default function ShopeeImport() {
  const fileRef = useRef<HTMLInputElement>(null)
  const [step, setStep] = useState<Step>('idle')
  const [config, setConfig] = useState<ShopeeConfig | null>(null)
  const [showConfigDialog, setShowConfigDialog] = useState(false)
  const [preview, setPreview] = useState<PreviewResponse | null>(null)
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set())
  const [results, setResults] = useState<{
    success_count: number
    fail_count: number
    results: ConfirmResult[]
  } | null>(null)
  const [error, setError] = useState('')
  const [expandedOrders, setExpandedOrders] = useState<Set<string>>(new Set())

  useEffect(() => {
    let alive = true
    client
      .get<ShopeeConfig>('/api/settings/shopee-config')
      .then((res) => {
        if (alive) setConfig(res.data)
      })
      .catch(() => {
        if (alive) setError('โหลด config ไม่ได้')
      })
    return () => {
      alive = false
    }
  }, [])

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file || !config) return
    e.target.value = ''
    setStep('uploading')
    setError('')
    setPreview(null)
    setResults(null)
    const form = new FormData()
    form.append('file', file)
    try {
      const res = await client.post<PreviewResponse>(
        '/api/import/shopee/preview',
        form,
        { headers: { 'Content-Type': 'multipart/form-data' } },
      )
      setPreview(res.data)
      setSelectedIDs(
        new Set(res.data.orders.filter((o) => !o.duplicate).map((o) => o.order_id)),
      )
      setStep('preview')
    } catch (err: unknown) {
      setError(
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
          'อัปโหลดไฟล์ไม่ได้',
      )
      setStep('idle')
    }
  }

  const handleConfirm = async () => {
    if (!preview || !config || selectedIDs.size === 0) return
    setStep('confirming')
    setError('')
    try {
      const res = await client.post('/api/import/shopee/confirm', {
        config,
        order_ids: Array.from(selectedIDs),
        orders: preview.orders,
        file_token: preview.file_token,
      })
      setResults(res.data)
      setStep('done')
    } catch (err: unknown) {
      setError(
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
          'ส่งข้อมูลไม่ได้',
      )
      setStep('preview')
    }
  }

  const toggleOrder = (id: string) =>
    setSelectedIDs((p) => {
      const s = new Set(p)
      if (s.has(id)) s.delete(id)
      else s.add(id)
      return s
    })
  const toggleAll = () => {
    if (!preview) return
    const nonDup = preview.orders.filter((o) => !o.duplicate).map((o) => o.order_id)
    setSelectedIDs(selectedIDs.size === nonDup.length ? new Set() : new Set(nonDup))
  }
  const toggleExpand = (id: string) =>
    setExpandedOrders((p) => {
      const s = new Set(p)
      if (s.has(id)) s.delete(id)
      else s.add(id)
      return s
    })

  return (
    <div className="space-y-5">
      <PageHeader
        title="นำเข้า Shopee"
        description="อัปโหลดไฟล์ Excel จาก Shopee Seller Center → สร้างใบกำกับสินค้าใน SML"
      />

      <Alert>
        <Info className="h-4 w-4" />
        <AlertTitle>ใช้สำหรับ bulk import เท่านั้น</AlertTitle>
        <AlertDescription>
          ถ้าตั้ง email forwarding ของ Shopee แล้ว ระบบจะดึง order/shipping email อัตโนมัติทุก 5
          นาที (ดูที่{' '}
          <Link to="/bills?source=shopee_email" className="font-medium text-primary hover:underline">
            Shopee Email
          </Link>{' '}
          และ{' '}
          <Link
            to="/bills?source=shopee_shipped"
            className="font-medium text-primary hover:underline"
          >
            Shopee จัดส่งแล้ว
          </Link>
          )
        </AlertDescription>
      </Alert>

      <input
        ref={fileRef}
        type="file"
        accept=".xlsx"
        className="sr-only"
        onChange={handleFileChange}
      />

      {config && (
        <ConfigDialog
          config={config}
          open={showConfigDialog}
          onOpenChange={setShowConfigDialog}
          onSave={(c) => {
            setConfig(c)
            setShowConfigDialog(false)
          }}
        />
      )}

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {(step === 'idle' || step === 'uploading') && (
        <>
          {config && (
            <Card>
              <CardContent className="flex flex-wrap items-center justify-between gap-3 p-3">
                <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                  <span className="font-medium text-foreground">SML config:</span>
                  <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px]">
                    {config.database_name}
                  </code>
                  <span>
                    Cust:{' '}
                    <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px]">
                      {config.cust_code || '—'}
                    </code>
                  </span>
                  <span>
                    WH:{' '}
                    <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px]">
                      {config.wh_code || '—'}
                    </code>
                  </span>
                  <span>
                    Doc:{' '}
                    <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px]">
                      {config.doc_format_code}
                    </code>
                  </span>
                  <span>
                    VAT: {config.vat_rate}% (type {config.vat_type})
                  </span>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowConfigDialog(true)}
                >
                  <SettingsIcon className="h-3.5 w-3.5" />
                  แก้ config
                </Button>
              </CardContent>
            </Card>
          )}

          <div
            className={cn(
              'flex flex-col items-center justify-center rounded-lg border-2 border-dashed border-border bg-muted/20 p-10 text-center',
              step === 'uploading' && 'opacity-60',
            )}
          >
            {step === 'uploading' ? (
              <p className="text-sm text-muted-foreground">กำลังวิเคราะห์ไฟล์…</p>
            ) : (
              <>
                <FileSpreadsheet className="mb-3 h-10 w-10 text-muted-foreground" />
                <p className="text-sm font-medium text-foreground">
                  คลิกเพื่อเลือกไฟล์ Excel (.xlsx) จาก Shopee
                </p>
                <Button
                  className="mt-4"
                  onClick={() => fileRef.current?.click()}
                  disabled={!config}
                >
                  {config ? 'เลือกไฟล์ Shopee' : 'กำลังโหลด config…'}
                </Button>
              </>
            )}
          </div>
        </>
      )}

      {step === 'preview' && preview && (
        <>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <SummaryCard
              label="Orders ทั้งหมด"
              value={preview.total_orders}
              variant="primary"
            />
            <SummaryCard
              label="เลือกแล้ว"
              value={selectedIDs.size}
              variant="success"
            />
            <SummaryCard
              label="ซ้ำ (ข้ามไป)"
              value={preview.duplicate_count}
              variant="muted"
            />
          </div>

          {(preview.warnings ?? []).length > 0 && (
            <Alert>
              <AlertTriangle className="h-4 w-4" />
              <AlertTitle>คำเตือน ({preview.warnings.length} รายการ)</AlertTitle>
              <AlertDescription>
                <ul className="mt-1 list-disc pl-5 text-xs">
                  {preview.warnings.map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" onClick={toggleAll}>
              {selectedIDs.size === preview.orders.filter((o) => !o.duplicate).length
                ? 'ยกเลิกทั้งหมด'
                : 'เลือกทั้งหมด'}
            </Button>
            <Button
              size="sm"
              disabled={selectedIDs.size === 0}
              onClick={handleConfirm}
            >
              ยืนยันส่ง {selectedIDs.size} Orders → SML
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setStep('idle')
                setPreview(null)
              }}
            >
              เลือกไฟล์ใหม่
            </Button>
          </div>

          <div className="overflow-hidden rounded-lg border border-border bg-card">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/40">
                  <TableHead className="w-10">
                    <Checkbox
                      checked={
                        selectedIDs.size ===
                        preview.orders.filter((o) => !o.duplicate).length
                      }
                      onCheckedChange={toggleAll}
                      aria-label="เลือกทั้งหมด"
                    />
                  </TableHead>
                  <TableHead>Order ID</TableHead>
                  <TableHead>วันที่</TableHead>
                  <TableHead>สถานะ</TableHead>
                  <TableHead>สินค้า</TableHead>
                  <TableHead className="text-right">Qty รวม</TableHead>
                  <TableHead>หมายเหตุ</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {preview.orders.map((order) => {
                  const expanded = expandedOrders.has(order.order_id)
                  return (
                    <Fragment key={order.order_id}>
                      <TableRow
                        className={cn(
                          order.duplicate && 'bg-muted/30 text-muted-foreground',
                        )}
                      >
                        <TableCell>
                          <Checkbox
                            checked={selectedIDs.has(order.order_id)}
                            disabled={order.duplicate}
                            onCheckedChange={() => toggleOrder(order.order_id)}
                            aria-label={`เลือก order ${order.order_id}`}
                          />
                        </TableCell>
                        <TableCell>
                          <button
                            type="button"
                            className="inline-flex items-center gap-1 font-mono text-xs font-medium text-foreground hover:text-primary"
                            onClick={() => toggleExpand(order.order_id)}
                          >
                            {expanded ? (
                              <ChevronDown className="h-3 w-3" />
                            ) : (
                              <ChevronRight className="h-3 w-3" />
                            )}
                            {order.order_id}
                          </button>
                        </TableCell>
                        <TableCell className="text-xs tabular-nums text-muted-foreground">
                          {order.doc_date}
                        </TableCell>
                        <TableCell>
                          <Badge variant="secondary" className="text-xs font-normal">
                            {order.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs">
                          {order.item_count} รายการ
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {order.total_qty}
                        </TableCell>
                        <TableCell>
                          {order.duplicate && (
                            <Badge variant="secondary" className="bg-warning/15 text-warning hover:bg-warning/20">
                              มีในระบบแล้ว
                            </Badge>
                          )}
                        </TableCell>
                      </TableRow>
                      {expanded && (
                        <TableRow>
                          <TableCell colSpan={7} className="bg-muted/20 p-0">
                            <div className="overflow-hidden border-l-2 border-primary/40">
                              <Table>
                                <TableHeader>
                                  <TableRow className="bg-muted/30">
                                    <TableHead className="text-[10px] uppercase">SKU</TableHead>
                                    <TableHead className="text-[10px] uppercase">ชื่อสินค้า</TableHead>
                                    <TableHead className="text-right text-[10px] uppercase">ราคา</TableHead>
                                    <TableHead className="text-right text-[10px] uppercase">จำนวน</TableHead>
                                  </TableRow>
                                </TableHeader>
                                <TableBody>
                                  {order.items.map((item, i) => (
                                    <TableRow key={i}>
                                      <TableCell className="font-mono text-xs">
                                        {item.sku}
                                      </TableCell>
                                      <TableCell className="text-sm">
                                        {item.product_name}
                                      </TableCell>
                                      <TableCell className="text-right tabular-nums">
                                        {fmt(item.price)}
                                      </TableCell>
                                      <TableCell className="text-right tabular-nums">
                                        {item.qty}
                                      </TableCell>
                                    </TableRow>
                                  ))}
                                </TableBody>
                              </Table>
                            </div>
                          </TableCell>
                        </TableRow>
                      )}
                    </Fragment>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        </>
      )}

      {step === 'confirming' && (
        <Card>
          <CardContent className="flex items-center justify-center gap-2 p-8 text-sm text-muted-foreground">
            กำลังบันทึกบิล… กรุณารอสักครู่
          </CardContent>
        </Card>
      )}

      {step === 'done' && results && (
        <>
          <Alert>
            <CheckCircle2 className="h-4 w-4 text-success" />
            <AlertTitle>สร้างบิลแล้ว {results.success_count} รายการ</AlertTitle>
            <AlertDescription>
              ระบบ map สินค้าให้เบื้องต้น แต่ <b>ยังไม่ส่ง SML</b> — กรุณาเข้าไปตรวจสอบรายการสินค้า
              + แก้ไขให้ถูกต้อง แล้วกด "ยืนยันและส่งไปยัง SML" ในหน้าบิลแต่ละใบ
            </AlertDescription>
          </Alert>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <SummaryCard
              label="สร้างบิลสำเร็จ"
              value={results.success_count}
              variant="success"
            />
            <SummaryCard
              label="ข้าม / ล้มเหลว"
              value={results.fail_count}
              variant="danger"
            />
            <SummaryCard
              label="ทั้งหมด"
              value={results.results.length}
              variant="primary"
            />
          </div>

          <div className="flex gap-2">
            <Button asChild>
              <Link to="/bills?source=shopee">
                ไปตรวจสอบบิลที่สร้าง
                <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setStep('idle')
                setPreview(null)
                setResults(null)
              }}
            >
              นำเข้าไฟล์ใหม่
            </Button>
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="text-sm">รายละเอียดผลลัพธ์</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/40">
                    <TableHead>Order ID</TableHead>
                    <TableHead>ผล</TableHead>
                    <TableHead>หมายเหตุ</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {results.results.map((r) => (
                    <TableRow key={r.order_id}>
                      <TableCell className="font-mono text-xs">{r.order_id}</TableCell>
                      <TableCell>
                        {r.success ? (
                          r.bill_id ? (
                            <Link
                              to={`/bills/${r.bill_id}`}
                              className="inline-flex items-center gap-1 font-medium text-success hover:underline"
                            >
                              เปิดบิล
                              <ExternalLink className="h-3 w-3" />
                            </Link>
                          ) : (
                            <span className="font-medium text-success">สำเร็จ</span>
                          )
                        ) : (
                          <span className="font-medium text-destructive">
                            ข้าม / ล้มเหลว
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {r.message}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
