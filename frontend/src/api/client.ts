import axios from 'axios'
import { toast } from 'sonner'
import { useAuthStore } from '../store/auth'

const client = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '',
  timeout: 30000,
})

// Attach JWT token to every request
client.interceptors.request.use((config) => {
  const token = useAuthStore.getState().token
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// On 401, clear auth, show toast, then redirect to login
client.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      useAuthStore.getState().logout()
      toast.error('Session หมดอายุ กรุณาเข้าสู่ระบบใหม่', { id: 'session-expired' })
      setTimeout(() => { window.location.href = '/login' }, 1500)
    }
    return Promise.reject(err)
  },
)

export default client
