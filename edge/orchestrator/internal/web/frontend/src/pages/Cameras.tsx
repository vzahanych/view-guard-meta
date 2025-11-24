import { useState, useEffect } from 'react'
import CameraViewer from '../components/CameraViewer'
import CameraGrid from '../components/CameraGrid'
import Select from '../components/Select'
import Card from '../components/Card'
import Loading from '../components/Loading'
import Button from '../components/Button'
import { api } from '../utils/api'
import { Grid, List } from 'lucide-react'

interface Camera {
  id: string
  name: string
  type: string
  enabled: boolean
  status: string
  last_seen?: string
}

export default function Cameras() {
  const [cameras, setCameras] = useState<Camera[]>([])
  const [selectedCameraId, setSelectedCameraId] = useState<string>('')
  const [viewMode, setViewMode] = useState<'single' | 'grid'>('single')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const fetchCameras = async () => {
      try {
        setLoading(true)
        const response = await api.get<{ cameras: Camera[]; count: number }>('/cameras')
        const enabledCameras = response.cameras.filter((cam) => cam.enabled)
        setCameras(enabledCameras)
        // Auto-select first camera if available
        if (enabledCameras.length > 0 && !selectedCameraId) {
          setSelectedCameraId(enabledCameras[0].id)
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load cameras')
      } finally {
        setLoading(false)
      }
    }

    fetchCameras()
  }, [])

  const selectedCamera = cameras.find((cam) => cam.id === selectedCameraId)

  if (loading) {
    return <Loading text="Loading cameras..." />
  }

  if (error) {
    return (
      <div className="card">
        <p className="text-red-600">{error}</p>
        <Button onClick={() => window.location.reload()} className="mt-4">
          Retry
        </Button>
      </div>
    )
  }

  const cameraOptions = cameras.map((cam) => ({
    value: cam.id,
    label: `${cam.name} (${cam.status})`,
  }))

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Cameras</h1>
          <p className="mt-2 text-gray-600">View live camera streams</p>
        </div>
        <div className="flex items-center space-x-2">
          <Button
            variant={viewMode === 'single' ? 'primary' : 'secondary'}
            size="sm"
            onClick={() => setViewMode('single')}
          >
            <List className="h-4 w-4 mr-2" />
            Single
          </Button>
          <Button
            variant={viewMode === 'grid' ? 'primary' : 'secondary'}
            size="sm"
            onClick={() => setViewMode('grid')}
          >
            <Grid className="h-4 w-4 mr-2" />
            Grid
          </Button>
        </div>
      </div>

      {viewMode === 'single' ? (
        <div className="space-y-4">
          <Card>
            <Select
              label="Select Camera"
              value={selectedCameraId}
              onChange={(e) => setSelectedCameraId(e.target.value)}
              options={cameraOptions}
            />
            {selectedCamera && (
              <div className="mt-4 text-sm text-gray-600">
                <p>
                  <span className="font-medium">Type:</span> {selectedCamera.type}
                </p>
                <p>
                  <span className="font-medium">Status:</span>{' '}
                  <span
                    className={`capitalize ${
                      selectedCamera.status === 'online'
                        ? 'text-green-600'
                        : 'text-red-600'
                    }`}
                  >
                    {selectedCamera.status}
                  </span>
                </p>
                {selectedCamera.last_seen && (
                  <p>
                    <span className="font-medium">Last Seen:</span>{' '}
                    {new Date(selectedCamera.last_seen).toLocaleString()}
                  </p>
                )}
              </div>
            )}
          </Card>

          {selectedCameraId ? (
            <CameraViewer
              cameraId={selectedCameraId}
              cameraName={selectedCamera?.name}
              className="w-full"
            />
          ) : (
            <Card>
              <p className="text-gray-600">Please select a camera to view</p>
            </Card>
          )}
        </div>
      ) : (
        <CameraGrid maxColumns={2} showSelector={true} />
      )}

      {cameras.length === 0 && (
        <Card>
          <p className="text-gray-600">No enabled cameras available</p>
          <p className="text-sm text-gray-500 mt-2">
            Enable cameras in the configuration to view streams
          </p>
        </Card>
      )}
    </div>
  )
}

