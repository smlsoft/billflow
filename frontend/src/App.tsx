import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useAuthStore } from './store/auth'
import Layout from './components/Layout'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Bills from './pages/Bills'
import BillDetail from './pages/BillDetail'
import Import from './pages/Import'
import ShopeeImport from './pages/ShopeeImport'
import Mappings from './pages/Mappings'
import Settings from './pages/Settings'
import Logs from './pages/Logs'
import CatalogSettings from './pages/CatalogSettings'
import EmailAccounts from './pages/EmailAccounts'
import ChannelDefaults from './pages/ChannelDefaults'
import LineOA from './pages/LineOA'
import Messages from './pages/Messages'
import QuickReplies from './pages/QuickReplies'
import Showcase from './pages/Showcase'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token)
  if (!token) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        {import.meta.env.DEV && (
          <Route path="/dev/showcase" element={<Showcase />} />
        )}
        <Route path="/login" element={<Login />} />
        <Route
          path="/"
          element={
            <RequireAuth>
              <Layout />
            </RequireAuth>
          }
        >
          <Route index element={<Navigate to="/dashboard" replace />} />
          <Route path="dashboard" element={<Dashboard />} />
          <Route path="bills" element={<Bills />} />
          <Route path="bills/:id" element={<BillDetail />} />
          <Route path="messages" element={<Messages />} />
          <Route path="import" element={<Import />} />
          <Route path="import/shopee" element={<ShopeeImport />} />
          <Route path="mappings" element={<Mappings />} />
          <Route path="settings" element={<Settings />} />
          <Route path="logs" element={<Logs />} />
          <Route path="settings/catalog" element={<CatalogSettings />} />
          <Route path="settings/email" element={<EmailAccounts />} />
          <Route path="settings/channels" element={<ChannelDefaults />} />
          <Route path="settings/line-oa" element={<LineOA />} />
          <Route path="settings/quick-replies" element={<QuickReplies />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
