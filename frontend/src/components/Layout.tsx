import { useState } from 'react'
import { Outlet, useNavigate } from 'react-router-dom'
import { Toaster } from '@/components/ui/sonner'
import Sidebar from '@/components/layout/Sidebar'
import Topbar from '@/components/layout/Topbar'
import { CommandPalette } from '@/components/CommandPalette'
import { BreadcrumbProvider } from '@/lib/breadcrumbs'
import { useUIStore } from '@/lib/ui-store'
import { useChordHotkeys, useHotkeys } from '@/hooks/useHotkeys'
import { useAuth } from '@/hooks/useAuth'

export default function Layout() {
  const navigate = useNavigate()
  const [paletteOpen, setPaletteOpen] = useState(false)
  const toggleSidebar = useUIStore((s) => s.toggleSidebar)
  const { logout } = useAuth()

  useHotkeys([
    {
      key: 'k',
      mod: true,
      preventDefault: true,
      description: 'เปิด Command Palette',
      action: () => setPaletteOpen((o) => !o),
    },
  ])

  useChordHotkeys({
    'g d': () => navigate('/dashboard'),
    'g b': () => navigate('/bills'),
    'g i': () => navigate('/import'),
    'g s': () => navigate('/import/shopee'),
    'g m': () => navigate('/mappings'),
    'g l': () => navigate('/logs'),
    'g c': () => navigate('/settings/catalog'),
    'g x': () => {
      logout()
      navigate('/login')
    },
  })

  return (
    <BreadcrumbProvider>
      <div className="flex h-screen w-full overflow-hidden bg-background text-foreground">
        <Sidebar />
        <div className="flex min-w-0 flex-1 flex-col">
          <Topbar onOpenPalette={() => setPaletteOpen(true)} />
          <main className="flex-1 overflow-y-auto">
            <div className="mx-auto w-full max-w-7xl p-6">
              <Outlet />
            </div>
          </main>
        </div>
        <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
        <Toaster />
      </div>
    </BreadcrumbProvider>
  )
}
