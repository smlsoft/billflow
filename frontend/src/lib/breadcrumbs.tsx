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
  { pattern: '/dashboard', crumbs: [{ label: 'Dashboard' }] },
  { pattern: '/bills', crumbs: [{ label: 'บิลทั้งหมด' }] },
  {
    pattern: '/bills/:id',
    crumbs: [{ label: 'บิลทั้งหมด', href: '/bills' }, { label: ':id', dynamic: true }],
  },
  {
    pattern: '/import',
    crumbs: [{ label: 'นำเข้า', href: '/import' }, { label: 'Lazada' }],
  },
  {
    pattern: '/import/shopee',
    crumbs: [{ label: 'นำเข้า', href: '/import' }, { label: 'Shopee' }],
  },
  { pattern: '/mappings', crumbs: [{ label: 'Mapping สินค้า' }] },
  { pattern: '/settings', crumbs: [{ label: 'ตั้งค่า' }] },
  {
    pattern: '/settings/catalog',
    crumbs: [{ label: 'ตั้งค่า', href: '/settings' }, { label: 'Catalog SML' }],
  },
  { pattern: '/logs', crumbs: [{ label: 'Activity Log' }] },
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
