import { useEffect, useState } from 'react'
import Card from '../components/Card'
import Loading from '../components/Loading'
import MetricCard from '../components/MetricCard'
import MetricChart from '../components/MetricChart'
import Button from '../components/Button'
import { api } from '../utils/api'
import { RefreshCw, Activity, CheckCircle2, AlertCircle } from 'lucide-react'

interface SystemStatus {
  status: string
  uptime: string
  version: string
}

interface SystemMetrics {
  system: {
    cpu_usage_percent: number
    memory_used_bytes: number
    memory_total_bytes: number
    disk_used_bytes: number
    disk_total_bytes: number
    disk_usage_percent: number
  }
}

interface AppMetrics {
  application: {
    event_queue_length: number
    active_cameras: number
    total_cameras?: number
    enabled_cameras?: number
    online_cameras?: number
  }
}

export default function Dashboard() {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [systemMetrics, setSystemMetrics] = useState<SystemMetrics | null>(null)
  const [appMetrics, setAppMetrics] = useState<AppMetrics | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [metricHistory, setMetricHistory] = useState<{
    cpu: Array<{ time: string; value: number }>
    memory: Array<{ time: string; value: number }>
    disk: Array<{ time: string; value: number }>
  }>({
    cpu: [],
    memory: [],
    disk: [],
  })

  const fetchData = async (isManualRefresh = false) => {
    try {
      if (isManualRefresh) {
        setRefreshing(true)
      } else {
        setLoading(true)
      }

      const [statusData, systemData, appData] = await Promise.all([
        api.get<SystemStatus>('/status'),
        api.get<SystemMetrics>('/metrics'),
        api.get<AppMetrics>('/metrics/app'),
      ])
      setStatus(statusData)
      setSystemMetrics(systemData)
      setAppMetrics(appData)

      // Update metric history (keep last 20 data points)
      if (systemData) {
        const now = new Date().toISOString()
        setMetricHistory((prev) => ({
          cpu: [
            ...prev.cpu.slice(-19),
            { time: now, value: systemData.system.cpu_usage_percent },
          ],
          memory: [
            ...prev.memory.slice(-19),
            {
              time: now,
              value: (systemData.system.memory_used_bytes / systemData.system.memory_total_bytes) * 100,
            },
          ],
          disk: [
            ...prev.disk.slice(-19),
            { time: now, value: systemData.system.disk_usage_percent },
          ],
        }))
      }
    } catch (error) {
      console.error('Failed to fetch dashboard data:', error)
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => {
    fetchData()
    const interval = setInterval(() => fetchData(), 30000) // Refresh every 30 seconds
    return () => clearInterval(interval)
  }, [])

  if (loading) {
    return <Loading text="Loading dashboard..." />
  }

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i]
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Dashboard</h1>
          <p className="mt-2 text-gray-600">System overview and metrics</p>
        </div>
        <Button
          size="sm"
          variant="secondary"
          onClick={() => fetchData(true)}
          disabled={refreshing}
        >
          <RefreshCw className={`h-4 w-4 mr-2 ${refreshing ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {/* System Status */}
      {status && (
        <Card>
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-gray-900">System Status</h3>
            <div className="flex items-center space-x-2">
              {status.status === 'healthy' ? (
                <CheckCircle2 className="h-5 w-5 text-green-600" />
              ) : (
                <AlertCircle className="h-5 w-5 text-red-600" />
              )}
              <span
                className={`px-3 py-1 rounded-full text-sm font-medium ${
                  status.status === 'healthy'
                    ? 'bg-green-100 text-green-800'
                    : 'bg-red-100 text-red-800'
                }`}
              >
                {status.status.toUpperCase()}
              </span>
            </div>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <p className="text-sm text-gray-600">Uptime</p>
              <p className="text-lg font-semibold text-gray-900">{status.uptime}</p>
            </div>
            <div>
              <p className="text-sm text-gray-600">Version</p>
              <p className="text-lg font-semibold text-gray-900">v{status.version}</p>
            </div>
            <div>
              <p className="text-sm text-gray-600">Last Updated</p>
              <p className="text-lg font-semibold text-gray-900">
                {new Date().toLocaleTimeString()}
              </p>
            </div>
          </div>
        </Card>
      )}

      {/* System Metrics */}
      {systemMetrics && (
        <>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            <MetricCard
              title="CPU Usage"
              value={`${systemMetrics.system.cpu_usage_percent.toFixed(1)}%`}
              percentage={systemMetrics.system.cpu_usage_percent}
            />
            <MetricCard
              title="Memory Usage"
              value={formatBytes(systemMetrics.system.memory_used_bytes)}
              subtitle={`of ${formatBytes(systemMetrics.system.memory_total_bytes)}`}
              percentage={
                (systemMetrics.system.memory_used_bytes /
                  systemMetrics.system.memory_total_bytes) *
                100
              }
            />
            <MetricCard
              title="Disk Usage"
              value={`${systemMetrics.system.disk_usage_percent.toFixed(1)}%`}
              subtitle={`${formatBytes(systemMetrics.system.disk_used_bytes)} of ${formatBytes(
                systemMetrics.system.disk_total_bytes
              )}`}
              percentage={systemMetrics.system.disk_usage_percent}
            />
          </div>

          {/* Metric Charts */}
          {metricHistory.cpu.length > 0 && (
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <MetricChart
                title="CPU Usage Over Time"
                data={metricHistory.cpu}
                type="line"
                color="#3b82f6"
                unit="%"
              />
              <MetricChart
                title="Memory Usage Over Time"
                data={metricHistory.memory}
                type="line"
                color="#10b981"
                unit="%"
              />
              <MetricChart
                title="Disk Usage Over Time"
                data={metricHistory.disk}
                type="line"
                color="#f59e0b"
                unit="%"
              />
            </div>
          )}
        </>
      )}

      {/* Application Metrics */}
      {appMetrics && (
        <Card>
          <div className="flex items-center mb-4">
            <Activity className="h-5 w-5 text-gray-400 mr-2" />
            <h3 className="text-lg font-semibold text-gray-900">Application Metrics</h3>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <MetricCard
              title="Event Queue"
              value={appMetrics.application.event_queue_length}
              className="border-l-4 border-blue-500"
            />
            <MetricCard
              title="Active Cameras"
              value={appMetrics.application.active_cameras}
              className="border-l-4 border-green-500"
            />
            {appMetrics.application.total_cameras !== undefined && (
              <>
                <MetricCard
                  title="Total Cameras"
                  value={appMetrics.application.total_cameras}
                  className="border-l-4 border-purple-500"
                />
                <MetricCard
                  title="Online Cameras"
                  value={appMetrics.application.online_cameras || 0}
                  subtitle={`of ${appMetrics.application.total_cameras}`}
                  percentage={
                    appMetrics.application.total_cameras > 0
                      ? ((appMetrics.application.online_cameras || 0) /
                          appMetrics.application.total_cameras) *
                        100
                      : 0
                  }
                  className="border-l-4 border-indigo-500"
                />
              </>
            )}
          </div>
        </Card>
      )}
    </div>
  )
}

