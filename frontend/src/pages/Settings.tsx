import { useEffect, useState } from 'react'
import { Check, Save, Sparkles } from 'lucide-react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { PageHeader } from '@/components/common/PageHeader'
import { StatusDot } from '@/components/common/StatusDot'
import client from '@/api/client'
import { useAuth } from '@/hooks/useAuth'
import type { DashboardStats, PlatformColumnMapping } from '@/types'

type ConfigStatus = {
  line_configured: boolean
  imap_configured: boolean
  sml_configured: boolean
  ai_configured: boolean
  auto_confirm_threshold: number
}

const PLATFORM_FIELDS: Array<{ key: string; label: string }> = [
  { key: 'order_id', label: 'หมายเลขออเดอร์' },
  { key: 'buyer_name', label: 'ชื่อลูกค้า' },
  { key: 'buyer_phone', label: 'เบอร์โทร' },
  { key: 'item_name', label: 'ชื่อสินค้า' },
  { key: 'sku', label: 'SKU' },
  { key: 'qty', label: 'จำนวน' },
  { key: 'price', label: 'ราคาต่อหน่วย' },
]

function StatusRow({ ok, label }: { ok: boolean; label: string }) {
  return (
    <div className="flex items-center justify-between py-1">
      <StatusDot
        variant={ok ? 'success' : 'danger'}
        label={label}
        className="text-foreground"
      />
      <Badge
        variant="secondary"
        className={
          ok
            ? 'bg-success/15 text-success hover:bg-success/20'
            : 'bg-destructive/15 text-destructive hover:bg-destructive/20'
        }
      >
        {ok ? 'พร้อมใช้งาน' : 'ยังไม่ได้ตั้งค่า'}
      </Badge>
    </div>
  )
}

function ColumnMappingEditor({ platform }: { platform: 'lazada' | 'shopee' }) {
  const [mappings, setMappings] = useState<PlatformColumnMapping[]>([])
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    client
      .get<{ mappings: PlatformColumnMapping[] }>(`/api/settings/column-mappings/${platform}`)
      .then((r) => {
        const map = new Map(r.data.mappings.map((m) => [m.field_name, m]))
        setMappings(
          PLATFORM_FIELDS.map(
            (f) =>
              map.get(f.key) ?? {
                platform,
                field_name: f.key,
                column_name: '',
              },
          ),
        )
      })
      .catch(() => {
        setMappings(
          PLATFORM_FIELDS.map((f) => ({
            platform,
            field_name: f.key,
            column_name: '',
          })),
        )
      })
  }, [platform])

  const updateColumnName = (fieldName: string, value: string) => {
    setMappings((prev) =>
      prev.map((m) =>
        m.field_name === fieldName ? { ...m, column_name: value } : m,
      ),
    )
  }

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    setSaved(false)
    try {
      await client.put(`/api/settings/column-mappings/${platform}`, { mappings })
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } }
      setError(err?.response?.data?.error ?? 'บันทึกไม่สำเร็จ')
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <div className="overflow-hidden rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40">
              <TableHead className="w-1/3">Field</TableHead>
              <TableHead>ชื่อ Column ในไฟล์ Excel</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {PLATFORM_FIELDS.map((f) => {
              const m = mappings.find((x) => x.field_name === f.key)
              return (
                <TableRow key={f.key}>
                  <TableCell>
                    <div className="font-mono text-xs font-medium text-foreground">
                      {f.key}
                    </div>
                    <div className="mt-0.5 text-xs text-muted-foreground">
                      {f.label}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Input
                      value={m?.column_name ?? ''}
                      onChange={(e) => updateColumnName(f.key, e.target.value)}
                      placeholder="ชื่อ column จริงในไฟล์…"
                      className="h-8"
                    />
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
      <div className="mt-3 flex items-center gap-3">
        <Button size="sm" onClick={handleSave} disabled={saving}>
          <Save className="h-3.5 w-3.5" />
          {saving ? 'กำลังบันทึก…' : 'บันทึก'}
        </Button>
        {saved && (
          <span className="inline-flex items-center gap-1 text-xs text-success">
            <Check className="h-3 w-3" />
            บันทึกแล้ว
          </span>
        )}
        {error && <span className="text-xs text-destructive">{error}</span>}
      </div>
    </>
  )
}

export default function Settings() {
  const { user } = useAuth()
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [config, setConfig] = useState<ConfigStatus | null>(null)
  const colMapTab: 'lazada' | 'shopee' = 'lazada'

  useEffect(() => {
    client.get<DashboardStats>('/api/dashboard/stats').then((r) => setStats(r.data)).catch(() => null)
    client.get<ConfigStatus>('/api/settings/status').then((r) => setConfig(r.data)).catch(() => null)
  }, [])

  return (
    <div className="space-y-5">
      <PageHeader title="ตั้งค่า" description="ข้อมูลผู้ใช้ + สถานะการเชื่อมต่อระบบภายนอก" />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">ข้อมูลผู้ใช้</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div className="flex items-baseline justify-between">
              <Label className="text-muted-foreground">ชื่อ</Label>
              <span className="font-medium">{user?.name}</span>
            </div>
            <div className="flex items-baseline justify-between">
              <Label className="text-muted-foreground">อีเมล</Label>
              <span className="font-medium">{user?.email}</span>
            </div>
            <div className="flex items-baseline justify-between">
              <Label className="text-muted-foreground">สิทธิ์</Label>
              <Badge variant="secondary" className="font-mono text-[10px] uppercase">
                {user?.role}
              </Badge>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">สถานะการเชื่อมต่อ</CardTitle>
          </CardHeader>
          <CardContent>
            {config ? (
              <div className="space-y-1.5">
                <StatusRow ok={config.line_configured} label="LINE OA Webhook" />
                <StatusRow ok={config.imap_configured} label="Email (IMAP)" />
                <StatusRow ok={config.sml_configured} label="SML ERP API" />
                <StatusRow ok={config.ai_configured} label="OpenRouter AI" />
                <Separator className="my-2" />
                <div className="flex items-center justify-between text-sm">
                  <span className="flex items-center gap-2 text-muted-foreground">
                    <Sparkles className="h-3.5 w-3.5" />
                    Auto-confirm Threshold
                  </span>
                  <span className="font-mono font-semibold tabular-nums text-primary">
                    {(config.auto_confirm_threshold * 100).toFixed(0)}%
                  </span>
                </div>
              </div>
            ) : (
              <p className="text-sm italic text-muted-foreground">
                ไม่สามารถโหลดสถานะการเชื่อมต่อได้
              </p>
            )}
          </CardContent>
        </Card>
      </div>

      {user?.role === 'admin' && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Column Mapping — Lazada</CardTitle>
            <CardDescription>
              กำหนดชื่อ column ในไฟล์ Excel ให้ตรงกับ field ที่ระบบใช้งาน
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Alert className="mb-4">
              <AlertDescription className="text-xs">
                Shopee ใช้ column hardcoded — ปรับใน code ที่{' '}
                <code className="font-mono">backend/internal/handlers/shopee_import.go</code>
              </AlertDescription>
            </Alert>
            <ColumnMappingEditor key={colMapTab} platform={colMapTab} />
          </CardContent>
        </Card>
      )}

      {stats && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">สรุประบบ</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <div>
                <p className="text-xs text-muted-foreground">บิลทั้งหมด</p>
                <p className="mt-1 text-xl font-semibold tabular-nums">{stats.total_bills}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">SML สำเร็จ</p>
                <p className="mt-1 text-xl font-semibold tabular-nums text-success">
                  {stats.sml_success}
                </p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">รอดำเนินการ</p>
                <p className="mt-1 text-xl font-semibold tabular-nums text-warning">
                  {stats.pending}
                </p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">ล้มเหลว</p>
                <p className="mt-1 text-xl font-semibold tabular-nums text-destructive">
                  {stats.sml_failed}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      <p className="text-center text-xs text-muted-foreground">
        BillFlow v0.2.0 — AI-powered bill processing system
      </p>
    </div>
  )
}
