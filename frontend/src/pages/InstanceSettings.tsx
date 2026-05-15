import { useEffect, useMemo, useState } from 'react'
import { AlertCircle, AlertTriangle, Bell, Bot, Building2, CheckCircle2, Database, RotateCw, Save, Settings2 } from 'lucide-react'
import { toast } from 'sonner'

import client from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { PageHeader } from '@/components/common/PageHeader'
import { cn } from '@/lib/utils'

type SettingGroup = 'instance' | 'sml' | 'line' | 'ai' | 'automation'

type InstanceSetting = {
  key: string
  label: string
  group: SettingGroup
  type: 'text' | 'url' | 'number' | 'password'
  value: string
  source: 'database' | 'env' | 'default'
  env_key?: string
  secret?: boolean
  has_secret?: boolean
  restart_required?: boolean
  description?: string
  overridden?: boolean
  runtime_value?: string
  active?: boolean
  pending_restart?: boolean
}

type Response = {
  settings: InstanceSetting[]
  note?: string
  pending_restart?: boolean
  pending_restart_settings?: string[]
}

const GROUP_META: Record<SettingGroup, { title: string; description: string; icon: typeof Building2 }> = {
  instance: {
    title: 'ข้อมูลร้าน (ไม่บังคับ)',
    description: 'ใช้เป็นป้ายกำกับให้ทีมดูแลระบบ ไม่เกี่ยวกับการส่ง SML หรือ LINE',
    icon: Building2,
  },
  sml: {
    title: 'SML ERP',
    description: 'ข้อมูลเชื่อมต่อ SML ของร้านนี้',
    icon: Database,
  },
  line: {
    title: 'LINE แจ้งเตือนระบบ',
    description: 'Token และ userId สำหรับส่ง error/สถานะระบบไปหาแอดมิน',
    icon: Bell,
  },
  ai: {
    title: 'OpenRouter AI',
    description: 'API key และ model ที่ใช้ดึงข้อมูลจากอีเมล',
    icon: Bot,
  },
  automation: {
    title: 'Automation',
    description: 'ค่าควบคุมการทำงานอัตโนมัติ',
    icon: Settings2,
  },
}

const GROUP_ORDER: SettingGroup[] = ['instance', 'sml', 'line', 'ai', 'automation']
const PHASE = Number(import.meta.env.VITE_PHASE ?? 99)

const PHASE1_HIDDEN_KEYS = new Set([
  'sml.json_rpc_base_url',
  'ai.openrouter_audio_model',
  'automation.auto_confirm_threshold',
])

function sourceLabel(source: InstanceSetting['source']) {
  if (source === 'database') return 'ตั้งค่าแล้ว'
  return 'ยังไม่ได้ตั้งค่าในระบบ'
}

export default function InstanceSettings() {
  const [settings, setSettings] = useState<InstanceSetting[]>([])
  const [draft, setDraft] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const [pendingRestart, setPendingRestart] = useState(false)
  const [restartKeys, setRestartKeys] = useState<string[]>([])

  const load = async () => {
    setLoading(true)
    try {
      const res = await client.get<Response>('/api/settings/instance')
      setSettings(res.data.settings ?? [])
      setPendingRestart(!!res.data.pending_restart)
      setRestartKeys(res.data.pending_restart_settings ?? [])
      setDraft(
        Object.fromEntries((res.data.settings ?? []).map((s) => [s.key, s.source === 'database' ? s.value ?? '' : ''])),
      )
    } catch {
      toast.error('โหลดค่าการเชื่อมต่อไม่สำเร็จ')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const grouped = useMemo(() => {
    return GROUP_ORDER.map((group) => ({
      group,
      items: settings.filter((s) => s.group === group && !(PHASE < 2 && PHASE1_HIDDEN_KEYS.has(s.key))),
    })).filter((g) => g.items.length > 0)
  }, [settings])

  const visibleKeys = useMemo(
    () => new Set(grouped.flatMap((g) => g.items.map((s) => s.key))),
    [grouped],
  )

  const waitForBackend = async () => {
    await new Promise((resolve) => setTimeout(resolve, 1200))
    for (let i = 0; i < 24; i += 1) {
      try {
        await client.get('/health', { timeout: 2000 })
        return
      } catch {
        await new Promise((resolve) => setTimeout(resolve, 1500))
      }
    }
    throw new Error('backend restart timeout')
  }

  const save = async () => {
    if (saving || restarting || loading) return
    const changed = settings.filter((s) => visibleKeys.has(s.key) && (draft[s.key] ?? '') !== (s.source === 'database' ? s.value ?? '' : ''))
    if (changed.length > 0) {
      const important = changed.filter((s) => s.restart_required || s.group === 'sml' || s.group === 'ai' || s.secret)
      if (important.length > 0) {
        const labels = important.slice(0, 6).map((s) => s.label).join(', ')
        const more = important.length > 6 ? ` และอีก ${important.length - 6} ค่า` : ''
        if (!window.confirm(`บันทึกและรีสตาร์ท backend ตอนนี้?\n\nค่าที่มีผลต่อระบบจริง: ${labels}${more}`)) {
          return
        }
      }
    }
    setSaving(true)
    setRestarting(false)
    const toastID = toast.loading('กำลังบันทึกค่า...')
    try {
      const payload = Object.fromEntries(
        Object.entries(draft).filter(([key]) => visibleKeys.has(key)),
      )
      await client.put('/api/settings/instance', { settings: payload })
      await load()
      setRestarting(true)
      toast.loading('บันทึกแล้ว กำลังเริ่มใช้ค่าใหม่...', { id: toastID })
      await client.post('/api/settings/instance/restart', {}, { timeout: 5000 })
      await waitForBackend()
      toast.success('บันทึกค่าแล้ว ระบบพร้อมใช้ค่าใหม่', { id: toastID })
      await load()
    } catch {
      toast.error('บันทึกหรือเริ่มใช้ค่าใหม่ไม่สำเร็จ', { id: toastID })
    } finally {
      setSaving(false)
      setRestarting(false)
    }
  }

  const restartOnly = async () => {
    if (saving || restarting || loading) return
    if (!window.confirm('รีสตาร์ท backend ตอนนี้เพื่อเริ่มใช้ค่าที่บันทึกไว้?')) return
    setRestarting(true)
    const toastID = toast.loading('กำลังรีสตาร์ท backend...')
    try {
      await client.post('/api/settings/instance/restart', {}, { timeout: 5000 })
      await waitForBackend()
      toast.success('backend กลับมาแล้ว และใช้ค่าล่าสุดแล้ว', { id: toastID })
      await load()
    } catch {
      toast.error('รีสตาร์ทไม่สำเร็จหรือ backend กลับมาช้าเกินไป', { id: toastID })
    } finally {
      setRestarting(false)
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="การเชื่อมต่อระบบ"
        description={PHASE < 2
          ? 'ตั้งค่าเฉพาะที่ใช้ใน Phase 1: SML REST, LINE แจ้งเตือนระบบ และ OpenRouter'
          : 'ตั้งค่า SML ERP, OpenRouter และข้อมูลร้านที่ใช้กับ BillFlow ชุดนี้'}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            {pendingRestart && (
              <Button variant="outline" onClick={restartOnly} disabled={saving || restarting || loading}>
                <RotateCw className={cn('h-4 w-4', restarting && 'animate-spin')} />
                {restarting ? 'กำลังรีสตาร์ท...' : 'รีสตาร์ทและใช้ค่าทันที'}
              </Button>
            )}
            <Button onClick={save} disabled={saving || restarting || loading}>
              <Save className="h-4 w-4" />
              {restarting ? 'กำลังเริ่มใช้ค่าใหม่...' : saving ? 'กำลังบันทึก...' : 'บันทึกและเริ่มใช้ค่าใหม่'}
            </Button>
          </div>
        }
      />

      <div className={cn(
        'rounded-lg border p-3 text-sm',
        pendingRestart ? 'border-warning/35 bg-warning/[0.07]' : 'border-success/25 bg-success/[0.05]',
      )}>
        <div className="flex gap-2.5">
          {pendingRestart ? (
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
          ) : (
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" />
          )}
          <div>
            <p className="font-medium text-foreground">
              {pendingRestart ? 'มีค่าที่บันทึกแล้ว แต่ backend ยังไม่ได้เริ่มใช้' : 'ค่าที่ใช้งานจริงตรงกับค่าที่บันทึกแล้ว'}
            </p>
            <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
              {pendingRestart
                ? 'กด “รีสตาร์ทและใช้ค่าทันที” ก่อน Sync สินค้าหรือส่ง SML เพื่อไม่ให้ระบบใช้ headers ชุดเก่า'
                : 'หลังบันทึก ระบบจะ restart backend อัตโนมัติและรอจนกลับมาพร้อมใช้งาน ปกติใช้เวลาประมาณ 10-30 วินาที'}
            </p>
            {pendingRestart && restartKeys.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1">
                {restartKeys.map((key) => (
                  <Badge key={key} variant="outline" className="h-5 px-1.5 text-[10px]">
                    {settings.find((s) => s.key === key)?.label ?? key}
                  </Badge>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      {grouped.map(({ group, items }) => {
        const meta = GROUP_META[group]
        const Icon = meta.icon
        return (
          <Card key={group}>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-start gap-2 text-sm font-semibold">
                <Icon className="mt-0.5 h-4 w-4 text-primary" />
                <span>
                  {meta.title}
                  <span className="mt-0.5 block text-xs font-normal text-muted-foreground">
                    {meta.description}
                  </span>
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid gap-4 lg:grid-cols-2">
                {items.map((s) => (
                  <div key={s.key} className="space-y-1.5">
                    <div className="flex items-center justify-between gap-2">
                      <Label htmlFor={s.key}>{s.label}</Label>
                      <div className="flex items-center gap-1">
                        <Badge variant={s.source === 'database' ? 'default' : 'outline'} className="h-5 px-1.5 text-[10px]">
                          {sourceLabel(s.source)}
                        </Badge>
                        {s.pending_restart ? (
                          <Badge variant="destructive" className="h-5 px-1.5 text-[10px]">
                            รอรีสตาร์ท
                          </Badge>
                        ) : s.restart_required && (
                          <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
                            ใช้งานอยู่
                          </Badge>
                        )}
                      </div>
                    </div>
                    <Input
                      id={s.key}
                      type={s.type === 'password' ? 'password' : s.type}
                      value={draft[s.key] ?? ''}
                      placeholder={s.source === 'database' ? undefined : 'กรอกค่าของร้านนี้'}
                      onChange={(e) => setDraft((d) => ({ ...d, [s.key]: e.target.value }))}
                      className={s.key.includes('url') || s.key.includes('model') || s.key.includes('database') ? 'font-mono text-xs' : undefined}
                    />
                    {s.description && (
                      <p className="text-[11px] leading-relaxed text-muted-foreground">
                        {s.description}
                      </p>
                    )}
                    {s.restart_required && s.runtime_value !== undefined && (
                      <div className={cn(
                        'rounded-md border px-2 py-1.5 text-[11px]',
                        s.pending_restart ? 'border-warning/30 bg-warning/[0.06]' : 'border-border bg-muted/25',
                      )}>
                        <div className="flex items-start gap-1.5">
                          {s.pending_restart ? (
                            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-warning" />
                          ) : (
                            <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success" />
                          )}
                          <div className="min-w-0">
                            <span className="font-medium text-foreground">ค่าที่ backend ใช้อยู่ตอนนี้: </span>
                            <span className="break-all font-mono text-muted-foreground">
                              {s.runtime_value || '—'}
                            </span>
                          </div>
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
