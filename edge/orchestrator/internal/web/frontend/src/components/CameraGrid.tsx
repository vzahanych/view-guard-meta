import { useState, useEffect } from 'react'
import CameraViewer from './CameraViewer'
import Loading from './Loading'
import { api } from '../utils/api'

interface Camera {
  id: string
  name: string
  type: string
  enabled: boolean
  status: string
}

interface CameraGridProps {
  maxColumns?: number
  showSelector?: boolean
}

export default function CameraGrid({ maxColumns = 2, showSelector = true }: CameraGridProps) {
  const [cameras, setCameras] = useState<Camera[]>([])
  const [selectedCameras, setSelectedCameras] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Fetch cameras on mount
  useEffect(() => {
    const fetchCameras = async () => {
      try {
        setLoading(true)
        const response = await api.get<{ cameras: Camera[]; count: number }>('/cameras')
        const enabledCameras = response.cameras.filter((cam) => cam.enabled)
        setCameras(enabledCameras)
        // Auto-select first camera if available
        if (enabledCameras.length > 0 && selectedCameras.length === 0) {
          setSelectedCameras([enabledCameras[0].id])
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load cameras')
      } finally {
        setLoading(false)
      }
    }

    fetchCameras()
  }, [])

  const handleCameraSelect = (cameraId: string) => {
    if (selectedCameras.includes(cameraId)) {
      setSelectedCameras(selectedCameras.filter((id) => id !== cameraId))
    } else {
      setSelectedCameras([...selectedCameras, cameraId])
    }
  }

  if (loading) {
    return <Loading text="Loading cameras..." />
  }

  if (error) {
    return (
      <div className="card">
        <p className="text-red-600">{error}</p>
      </div>
    )
  }

  if (cameras.length === 0) {
    return (
      <div className="card">
        <p className="text-gray-600">No enabled cameras available</p>
      </div>
    )
  }

  const gridCols = `grid-cols-1 ${maxColumns >= 2 ? 'md:grid-cols-2' : ''} ${maxColumns >= 3 ? 'lg:grid-cols-3' : ''} ${maxColumns >= 4 ? 'xl:grid-cols-4' : ''}`

  return (
    <div className="space-y-4">
      {showSelector && (
        <div className="card">
          <h3 className="text-lg font-semibold text-gray-900 mb-4">Select Cameras</h3>
          <div className="flex flex-wrap gap-2">
            {cameras.map((camera) => (
              <label
                key={camera.id}
                className="flex items-center space-x-2 cursor-pointer p-2 rounded-lg border border-gray-300 hover:bg-gray-50"
              >
                <input
                  type="checkbox"
                  checked={selectedCameras.includes(camera.id)}
                  onChange={() => handleCameraSelect(camera.id)}
                  className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                <span className="text-sm text-gray-700">
                  {camera.name} ({camera.status})
                </span>
              </label>
            ))}
          </div>
        </div>
      )}

      {selectedCameras.length === 0 ? (
        <div className="card">
          <p className="text-gray-600">Select at least one camera to view</p>
        </div>
      ) : (
        <div className={`grid ${gridCols} gap-4`}>
          {selectedCameras.map((cameraId) => {
            const camera = cameras.find((c) => c.id === cameraId)
            return (
              <CameraViewer
                key={cameraId}
                cameraId={cameraId}
                cameraName={camera?.name}
                className="aspect-video"
              />
            )
          })}
        </div>
      )}
    </div>
  )
}

