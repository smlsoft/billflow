import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import { useLocation, matchPath } from 'react-router-dom'

export interface Crumb {
  label: string
  href?: string
}

interface BreadcrumbDef {
  label: string
  href?: string
  dynamic?: boolean
}

const ROUTES: Array<{ pattern: string; crumbs: BreadcrumbDef[] }> = [
  { pattern: '/dashboard', crumbs: [{ label: 'ภาพรวม' }] },
  { pattern: '/setup', crumbs: [{ label: 'เริ่มต้นใช้งาน' }] },
  { pattern: '/bills', crumbs: [{ label: 'ใบสั่งซื้อ' }] },
  { pattern: '/sales-orders', crumbs: [{ label: 'ใบสั่งขาย' }] },
  { pattern: '/sale-invoices', crumbs: [{ label: 'ขายสินค้าและบริการ' }] },
  {
    pattern: '/bills/:id',
    crumbs: [{ label: 'ใบสั่งซื้อ', href: '/bills' }, { label: ':id', dynamic: true }],
  },
  {
    pattern: '/sales-orders/:id',
    crumbs: [{ label: 'ใบสั่งขาย', href: '/sales-orders' }, { label: ':id', dynamic: true }],
  },
  {
    pattern: '/sale-invoices/:id',
    crumbs: [{ label: 'ขายสินค้าและบริการ', href: '/sale-invoices' }, { label: ':id', dynamic: true }],
  },
  {
    pattern: '/import',
    crumbs: [{ label: 'ช่องทางรับข้อมูล' }, { label: 'Lazada Excel' }],
  },
  {
    pattern: '/import/lazada',
    crumbs: [{ label: 'ช่องทางรับข้อมูล' }, { label: 'Lazada Excel' }],
  },
  {
    pattern: '/import/shopee',
    crumbs: [{ label: 'ช่องทางรับข้อมูล' }, { label: 'Shopee Excel' }],
  },
  {
    pattern: '/import/tiktok',
    crumbs: [{ label: 'ช่องทางรับข้อมูล' }, { label: 'TikTok Excel' }],
  },
  { pattern: '/mappings', crumbs: [{ label: 'ตารางจับคู่สินค้า' }] },
  { pattern: '/marketplace-aliases', crumbs: [{ label: 'สินค้ารอยืนยัน' }] },
  { pattern: '/settings', crumbs: [{ label: 'ตั้งค่า' }] },
  {
    pattern: '/settings/catalog',
    crumbs: [{ label: 'ตั้งค่า', href: '/settings' }, { label: 'สินค้าใน SML' }],
  },
  {
    pattern: '/settings/channels',
    crumbs: [{ label: 'ตั้งค่า', href: '/settings' }, { label: 'เส้นทางเอกสาร SML' }],
  },
  {
    pattern: '/settings/ai-usage',
    crumbs: [{ label: 'ตั้งค่า', href: '/settings' }, { label: 'การใช้งาน AI' }],
  },
  {
    pattern: '/settings/instance',
    crumbs: [{ label: 'ตั้งค่า', href: '/settings' }, { label: 'การเชื่อมต่อระบบ' }],
  },
  {
    pattern: '/settings/email',
    crumbs: [{ label: 'ช่องทางรับข้อมูล' }, { label: 'กล่องอีเมลรับบิล' }],
  },
  { pattern: '/logs', crumbs: [{ label: 'ประวัติการทำงาน' }] },
]

interface CtxValue {
  dynamic: Record<string, string>
  setDynamicLabel: (key: string, label: string) => void
}

const Ctx = createContext<CtxValue | null>(null)

export function BreadcrumbProvider({ children }: { children: React.ReactNode }) {
  const [dynamic, setDynamic] = useState<Record<string, string>>({})
  const setDynamicLabel = (key: string, label: string) =>
    setDynamic((p) => (p[key] === label ? p : { ...p, [key]: label }))
  return (
    <Ctx.Provider value={{ dynamic, setDynamicLabel }}>{children}</Ctx.Provider>
  )
}

export function useDynamicCrumb(key: string, label: string | undefined | null) {
  const ctx = useContext(Ctx)
  useEffect(() => {
    if (label && ctx) ctx.setDynamicLabel(key, label)
  }, [ctx, key, label])
}

export function useCrumbs(): Crumb[] {
  const { pathname } = useLocation()
  const ctx = useContext(Ctx)

  return useMemo(() => {
    for (const r of ROUTES) {
      const match = matchPath(r.pattern, pathname)
      if (!match) continue
      return r.crumbs.map((c) => {
        if (!c.dynamic) return { label: c.label, href: c.href }
        const key = c.label.replace(':', '')
        const dynLabel =
          (ctx?.dynamic[key]) ?? match.params[key]?.slice(0, 8) ?? key
        return { label: dynLabel }
      })
    }
    return []
  }, [pathname, ctx])
}
