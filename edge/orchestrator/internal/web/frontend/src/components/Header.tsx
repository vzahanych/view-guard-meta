import { useState, useEffect } from 'react'
import { AlertCircle, CheckCircle2 } from 'lucide-react'
import { api } from '../utils/api'

interface SystemStatus {
  status: string
  uptime: string
  version: string
}

export default function Header() {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchStatus = async () => {
      try {
        const data = await api.get<SystemStatus>('/status')
        setStatus(data)
      } catch (error) {
        console.error('Failed to fetch status:', error)
      } finally {
        setLoading(false)
      }
    }

    fetchStatus()
    const interval = setInterval(fetchStatus, 30000) // Refresh every 30 seconds
    return () => clearInterval(interval)
  }, [])

  return (
    <header className="bg-white border-b border-gray-200 h-16 flex items-center justify-between px-6">
      <div className="flex items-center space-x-4">
        {loading ? (
          <div className="h-4 w-4 border-2 border-gray-300 border-t-primary-600 rounded-full animate-spin" />
        ) : status ? (
          <div className="flex items-center space-x-2">
            {status.status === 'healthy' ? (
              <CheckCircle2 className="h-5 w-5 text-green-600" />
            ) : (
              <AlertCircle className="h-5 w-5 text-red-600" />
            )}
            <span className="text-sm text-gray-600">
              {status.status === 'healthy' ? 'System Healthy' : 'System Unhealthy'}
            </span>
            <span className="text-xs text-gray-400">•</span>
            <span className="text-xs text-gray-500">v{status.version}</span>
          </div>
        ) : null}
      </div>
    </header>
  )
}

