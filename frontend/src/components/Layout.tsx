import { Outlet } from 'react-router-dom'
import Sidebar from '@/components/layout/Sidebar'
import Topbar from '@/components/layout/Topbar'
import { BreadcrumbProvider } from '@/lib/breadcrumbs'

export default function Layout() {
  return (
    <BreadcrumbProvider>
      <div className="flex h-screen w-full overflow-hidden bg-background text-foreground">
        <Sidebar />
        <div className="flex min-w-0 flex-1 flex-col">
          <Topbar />
          <main className="flex-1 overflow-y-auto">
            <div className="mx-auto w-full max-w-7xl p-6">
              <Outlet />
            </div>
          </main>
        </div>
      </div>
    </BreadcrumbProvider>
  )
}
