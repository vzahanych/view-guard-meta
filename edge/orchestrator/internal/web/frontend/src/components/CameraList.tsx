import { useState, useEffect } from 'react'
import Card from './Card'
import Button from './Button'
import Loading from './Loading'
import { api } from '../utils/api'
import { Camera, Edit, Trash2, Power, PowerOff, TestTube, CheckCircle2, XCircle, Clock } from 'lucide-react'

interface CameraListItem {
  id: string
  name: string
  type: string
  enabled: boolean
  status: string
  last_seen?: string
  manufacturer?: string
  model?: string
  ip_address?: string
  device_path?: string
}

interface CameraListProps {
  onEdit: (cameraId: string) => void
  onDelete: (cameraId: string) => void
  onTest: (cameraId: string) => void
  onToggleEnabled: (cameraId: string, enabled: boolean) => void
}

export default function CameraList({
  onEdit,
  onDelete,
  onTest,
  onToggleEnabled,
}: CameraListProps) {
  const [cameras, setCameras] = useState<CameraListItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [testing, setTesting] = useState<Set<string>>(new Set())

  const fetchCameras = async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await api.get<{ cameras: CameraListItem[]; count: number }>('/cameras')
      setCameras(response.cameras)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load cameras')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchCameras()
  }, [])

  const handleTest = async (cameraId: string) => {
    setTesting((prev) => new Set(prev).add(cameraId))
    try {
      await api.post(`/cameras/${cameraId}/test`)
      // Refresh cameras to get updated status
      await fetchCameras()
    } catch (err) {
      console.error('Test failed:', err)
    } finally {
      setTesting((prev) => {
        const next = new Set(prev)
        next.delete(cameraId)
        return next
      })
    }
  }

  const getStatusColor = (status: string) => {
    switch (status.toLowerCase()) {
      case 'online':
        return 'text-green-600 bg-green-100'
      case 'offline':
        return 'text-red-600 bg-red-100'
      default:
        return 'text-gray-600 bg-gray-100'
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status.toLowerCase()) {
      case 'online':
        return <CheckCircle2 className="h-4 w-4" />
      case 'offline':
        return <XCircle className="h-4 w-4" />
      default:
        return <Clock className="h-4 w-4" />
    }
  }

  if (loading) {
    return <Loading text="Loading cameras..." />
  }

  if (error) {
    return (
      <Card>
        <p className="text-red-600">{error}</p>
        <Button onClick={fetchCameras} className="mt-4">
          Retry
        </Button>
      </Card>
    )
  }

  if (cameras.length === 0) {
    return (
      <Card>
        <p className="text-gray-600">No cameras found</p>
        <p className="text-sm text-gray-500 mt-2">Add a camera or discover cameras on your network</p>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      {cameras.map((camera) => (
        <Card key={camera.id}>
          <div className="flex items-start justify-between">
            <div className="flex-1">
              <div className="flex items-center space-x-3 mb-2">
                <Camera className="h-5 w-5 text-gray-400" />
                <h3 className="text-lg font-semibold text-gray-900">{camera.name}</h3>
                <span
                  className={`px-2 py-1 rounded text-xs font-medium flex items-center space-x-1 ${getStatusColor(
                    camera.status
                  )}`}
                >
                  {getStatusIcon(camera.status)}
                  <span className="capitalize">{camera.status}</span>
                </span>
                {!camera.enabled && (
                  <span className="px-2 py-1 rounded text-xs font-medium bg-gray-100 text-gray-600">
                    Disabled
                  </span>
                )}
              </div>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-sm text-gray-600">
                <div>
                  <span className="font-medium">Type:</span> {camera.type.toUpperCase()}
                </div>
                {camera.manufacturer && (
                  <div>
                    <span className="font-medium">Manufacturer:</span> {camera.manufacturer}
                    {camera.model && ` ${camera.model}`}
                  </div>
                )}
                {camera.ip_address && (
                  <div>
                    <span className="font-medium">IP:</span> {camera.ip_address}
                  </div>
                )}
                {camera.device_path && (
                  <div>
                    <span className="font-medium">Device:</span> {camera.device_path}
                  </div>
                )}
                {camera.last_seen && (
                  <div>
                    <span className="font-medium">Last Seen:</span>{' '}
                    {new Date(camera.last_seen).toLocaleString()}
                  </div>
                )}
              </div>
            </div>

            <div className="ml-4 flex flex-col space-y-2">
              <div className="flex space-x-2">
                <Button
                  size="sm"
                  variant={camera.enabled ? 'secondary' : 'primary'}
                  onClick={() => onToggleEnabled(camera.id, !camera.enabled)}
                >
                  {camera.enabled ? (
                    <>
                      <PowerOff className="h-4 w-4 mr-1" />
                      Disable
                    </>
                  ) : (
                    <>
                      <Power className="h-4 w-4 mr-1" />
                      Enable
                    </>
                  )}
                </Button>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => handleTest(camera.id)}
                  disabled={testing.has(camera.id)}
                >
                  <TestTube className="h-4 w-4 mr-1" />
                  {testing.has(camera.id) ? 'Testing...' : 'Test'}
                </Button>
                <Button size="sm" variant="secondary" onClick={() => onEdit(camera.id)}>
                  <Edit className="h-4 w-4 mr-1" />
                  Edit
                </Button>
                <Button size="sm" variant="danger" onClick={() => onDelete(camera.id)}>
                  <Trash2 className="h-4 w-4 mr-1" />
                  Delete
                </Button>
              </div>
            </div>
          </div>
        </Card>
      ))}
    </div>
  )
}

