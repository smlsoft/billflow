import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  AlertOctagon,
  ArrowUpRight,
  Bot,
  CheckCircle2,
  Database,
  Mail,
  MessageSquare,
  Sparkles,
  type LucideIcon,
} from 'lucide-react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PageHeader } from '@/components/common/PageHeader'
import client from '@/api/client'
import { cn } from '@/lib/utils'
import { PAGE_TITLE } from '@/lib/labels'

// Live multi-account aware status — returned by GET /api/settings/status.
// LINE/IMAP fields are optional because they only exist when those repos
// are wired (always true in production).
type SystemStatus = {
  sml_configured: boolean
  ai_configured: boolean
  auto_confirm_threshold: number
  line_oa_total?: number
  line_oa_enabled?: number
  imap_total?: number
  imap_enabled?: number
  imap_failing?: number
}

// SubsystemRow is a single subsystem on the system-health card. Each row is
// a click-through to the manage page so /settings stays read-only — no
// "view a stat then go figure out where to fix it" handoff.
interface SubsystemRowProps {
  icon: LucideIcon
  label: string
  // Right-aligned status: a quick glanceable summary.
  status: string
  // Tone drives the dot + (when urgent) the row tint.
  tone: 'ok' | 'warn' | 'danger' | 'unknown'
  // Multi-line detail under the status (count breakdowns, expiring tokens, etc.)
  detail?: string
  // Where clicking takes you. Omit for read-only rows (e.g. SML/AI from env).
  to?: string
}

function SubsystemRow({ icon: Icon, label, status, tone, detail, to }: SubsystemRowProps) {
  const dotCls =
    tone === 'ok'
      ? 'bg-success'
      : tone === 'warn'
        ? 'bg-warning'
        : tone === 'danger'
          ? 'bg-destructive animate-pulse'
          : 'bg-muted-foreground/40'

  const inner = (
    <div
      className={cn(
        'flex items-center gap-3 rounded-md px-3 py-2.5 transition-colors',
        to && 'group hover:bg-accent/40',
        tone === 'danger' && 'bg-destructive/[0.04]',
      )}
    >
      <span
        className={cn(
          'flex h-7 w-7 shrink-0 items-center justify-center rounded-md',
          tone === 'danger' ? 'bg-destructive/10 text-destructive' : 'bg-muted text-muted-foreground',
        )}
      >
        <Icon className="h-3.5 w-3.5" strokeWidth={2.25} />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-2">
          <span className="truncate text-sm font-medium text-foreground">{label}</span>
          <span className="flex shrink-0 items-center gap-1.5 text-[11px] tabular-nums text-muted-foreground">
            <span className={cn('inline-block h-1.5 w-1.5 rounded-full', dotCls)} />
            {status}
          </span>
        </div>
        {detail && (
          <p className={cn(
            'mt-0.5 truncate text-[11px]',
            tone === 'danger' ? 'text-destructive' : 'text-muted-foreground',
          )}>
            {detail}
          </p>
        )}
      </div>
      {to && (
        <ArrowUpRight
          className="h-3.5 w-3.5 shrink-0 text-muted-foreground/40 transition-all group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-foreground"
        />
      )}
    </div>
  )

  return to ? <Link to={to}>{inner}</Link> : inner
}

export default function Settings() {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    client
      .get<SystemStatus>('/api/settings/status')
      .then((r) => setStatus(r.data))
      .catch(() => setStatus(null))
      .finally(() => setLoading(false))
  }, [])

  // Derive each subsystem's tone from its live state. Falls back to 'unknown'
  // when the API didn't return that field (e.g. repo not wired in dev).
  const lineOA = (() => {
    if (status?.line_oa_total == null) return null
    const total = status.line_oa_total
    const enabled = status.line_oa_enabled ?? 0
    if (total === 0) {
      return { status: 'ยังไม่มี OA', tone: 'warn' as const, detail: 'เพิ่ม LINE OA เพื่อรับข้อความจากลูกค้า' }
    }
    return {
      status: `${enabled} / ${total} เปิดใช้งาน`,
      tone: enabled > 0 ? ('ok' as const) : ('warn' as const),
      detail: enabled === total ? undefined : `${total - enabled} OA ถูกปิด`,
    }
  })()

  const imap = (() => {
    if (status?.imap_total == null) return null
    const total = status.imap_total
    const enabled = status.imap_enabled ?? 0
    const failing = status.imap_failing ?? 0
    if (total === 0) {
      return { status: 'ยังไม่มี inbox', tone: 'warn' as const, detail: 'เพิ่ม email inbox เพื่อรับบิลทาง email' }
    }
    if (failing > 0) {
      return {
        status: `${failing} มีปัญหา`,
        tone: 'danger' as const,
        detail: `จาก ${enabled} inbox ที่เปิดใช้งาน — ตรวจ password / 2FA`,
      }
    }
    return {
      status: `${enabled} / ${total} เปิดใช้งาน`,
      tone: enabled > 0 ? ('ok' as const) : ('warn' as const),
    }
  })()

  return (
    <div className="space-y-5">
      <PageHeader
        title={PAGE_TITLE.settings}
        description="สถานะการเชื่อมต่อระบบภายนอก · กดที่แต่ละแถวเพื่อจัดการ"
      />

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-semibold">การเชื่อมต่อภายนอก</CardTitle>
        </CardHeader>
        <CardContent className="space-y-1 px-2 pb-3 pt-0">
          {/* LINE OA — multi-account aware. Click-through to /settings/line-oa */}
          {lineOA ? (
            <SubsystemRow
              icon={MessageSquare}
              label="LINE OA"
              status={lineOA.status}
              tone={lineOA.tone}
              detail={lineOA.detail}
              to="/settings/line-oa"
            />
          ) : loading ? (
            <SubsystemRowSkeleton icon={MessageSquare} label="LINE OA" />
          ) : (
            <SubsystemRow icon={MessageSquare} label="LINE OA" status="—" tone="unknown" />
          )}

          {/* Email inboxes — multi-account aware. Failing count surfaces here. */}
          {imap ? (
            <SubsystemRow
              icon={Mail}
              label="Email Inbox"
              status={imap.status}
              tone={imap.tone}
              detail={imap.detail}
              to="/settings/email"
            />
          ) : loading ? (
            <SubsystemRowSkeleton icon={Mail} label="Email Inbox" />
          ) : (
            <SubsystemRow icon={Mail} label="Email Inbox" status="—" tone="unknown" />
          )}

          {/* SML — env config only; not multi-account, no click-through. */}
          <SubsystemRow
            icon={Database}
            label="SML ERP"
            status={status?.sml_configured ? 'พร้อมใช้งาน' : 'ยังไม่ได้ตั้งค่า'}
            tone={status?.sml_configured ? 'ok' : 'danger'}
            detail={status?.sml_configured ? undefined : 'ตรวจ SML_BASE_URL ใน .env'}
          />

          {/* AI — env config only. */}
          <SubsystemRow
            icon={Bot}
            label="OpenRouter AI"
            status={status?.ai_configured ? 'พร้อมใช้งาน' : 'ยังไม่ได้ตั้งค่า'}
            tone={status?.ai_configured ? 'ok' : 'danger'}
            detail={status?.ai_configured ? undefined : 'ตรวจ OPENROUTER_API_KEY ใน .env'}
          />
        </CardContent>
      </Card>

      {/* Auto-confirm threshold — small "config snapshot" card for transparency.
          Lives in env, not editable here; surfacing the value avoids
          "what's our current threshold?" trips into the codebase. */}
      {status && (
        <Card>
          <CardContent className="flex items-center justify-between p-4">
            <div className="flex items-center gap-2.5">
              <Sparkles className="h-4 w-4 text-primary" strokeWidth={2.25} />
              <div>
                <p className="text-sm font-medium">Auto-confirm Threshold</p>
                <p className="text-[11px] text-muted-foreground">
                  AI confidence ≥ ค่านี้ → ผ่าน auto-confirm · ตั้งใน <code className="font-mono">.env</code> AUTO_CONFIRM_THRESHOLD
                </p>
              </div>
            </div>
            <span className="font-mono text-xl font-semibold tabular-nums text-primary">
              {(status.auto_confirm_threshold * 100).toFixed(0)}%
            </span>
          </CardContent>
        </Card>
      )}

      {/* Pre-deploy notice — let admin know /settings shows live state, not config */}
      <p className="flex items-center justify-center gap-1.5 text-center text-xs text-muted-foreground">
        <CheckCircle2 className="h-3 w-3" />
        BillFlow v0.2.0 · ดู status สด · ตั้งค่าจริงในแต่ละหน้าย่อย (LINE OA / Email / Channels / Catalog)
      </p>
    </div>
  )
}

// Loading placeholder that mirrors the SubsystemRow layout so the page
// doesn't "jump" when the API returns. Reuses the icon prop so the row
// already feels recognizable while loading.
function SubsystemRowSkeleton({ icon: Icon, label }: { icon: LucideIcon; label: string }) {
  return (
    <div className="flex items-center gap-3 rounded-md px-3 py-2.5">
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground/40">
        <Icon className="h-3.5 w-3.5" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-2">
          <span className="text-sm text-muted-foreground/40">{label}</span>
          <span className="text-[11px] text-muted-foreground/40">กำลังโหลด…</span>
        </div>
      </div>
      {/* Use AlertOctagon as a hidden anchor so layout matches non-skeleton row */}
      <AlertOctagon className="h-3.5 w-3.5 opacity-0" />
    </div>
  )
}
