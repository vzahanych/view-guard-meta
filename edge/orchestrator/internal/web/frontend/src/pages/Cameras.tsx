import { useState, useEffect } from 'react'
import CameraViewer from '../components/CameraViewer'
import CameraGrid from '../components/CameraGrid'
import Select from '../components/Select'
import Card from '../components/Card'
import Loading from '../components/Loading'
import Button from '../components/Button'
import { api } from '../utils/api'
import { Grid, List } from 'lucide-react'

interface DatasetStatus {
  labeled_snapshot_count: number
  required_snapshot_count: number
  snapshot_required: boolean
  label_counts?: Record<string, number>
}

interface Camera {
  id: string
  name: string
  type: string
  enabled: boolean
  status: string
  last_seen?: string
  dataset_status?: DatasetStatus
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
              <div className="mt-4 space-y-4">
                <div className="text-sm text-gray-600">
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
                {/* Dataset Progress Display */}
                <div className="pt-4 border-t border-gray-200">
                  <h3 className="text-sm font-semibold text-gray-900 mb-3">Dataset Progress</h3>
                  {selectedCamera.dataset_status ? (
                    (() => {
                      const status = selectedCamera.dataset_status!
                      // Fix progress calculation - handle division by zero (Substep 2.2.2.3.1)
                      const progress = status.required_snapshot_count > 0
                        ? Math.min(100, Math.max(0, (status.labeled_snapshot_count / status.required_snapshot_count) * 100))
                        : 0
                      const remaining = Math.max(0, status.required_snapshot_count - status.labeled_snapshot_count)
                      const isComplete = !status.snapshot_required

                      return (
                        <div className="space-y-3">
                          <div>
                            <div className="flex items-center justify-between mb-1">
                              <span className="text-sm text-gray-700">Normal Snapshots</span>
                              <span className="text-sm font-medium text-gray-900">
                                {status.labeled_snapshot_count} / {status.required_snapshot_count}
                              </span>
                            </div>
                            <div className="w-full bg-gray-200 rounded-full h-3">
                              <div
                                className={`h-3 rounded-full transition-all duration-500 ease-out ${
                                  isComplete ? 'bg-green-500' : 'bg-blue-500'
                                }`}
                                style={{ width: `${Math.min(progress, 100)}%` }}
                              />
                            </div>
                            {!isComplete && (
                              <p className="text-xs text-gray-600 mt-1">
                                {remaining} more normal snapshots needed for training
                              </p>
                            )}
                            {isComplete && (
                              <p className="text-xs text-green-600 mt-1 font-medium">
                                ✓ Ready for training
                              </p>
                            )}
                          </div>
                          {/* Snapshot count by label display (Substep 2.2.2.3.2) */}
                          {status.label_counts && Object.keys(status.label_counts).length > 0 && (
                            <div className="mt-3 pt-3 border-t border-gray-200">
                              <div className="text-xs font-medium text-gray-700 mb-2">Snapshot Counts by Label:</div>
                              <div className="flex flex-wrap gap-2">
                                {Object.entries(status.label_counts).map(([label, count]) => {
                                  const getLabelColor = (l: string) => {
                                    switch (l.toLowerCase()) {
                                      case 'normal':
                                        return 'bg-green-100 text-green-800 border-green-200'
                                      case 'threat':
                                        return 'bg-red-100 text-red-800 border-red-200'
                                      case 'abnormal':
                                        return 'bg-yellow-100 text-yellow-800 border-yellow-200'
                                      default:
                                        return 'bg-gray-100 text-gray-800 border-gray-200'
                                    }
                                  }
                                  const getLabelIcon = (l: string) => {
                                    switch (l.toLowerCase()) {
                                      case 'normal':
                                        return '✓'
                                      case 'threat':
                                        return '⚠'
                                      case 'abnormal':
                                        return '!'
                                      default:
                                        return '•'
                                    }
                                  }
                                  return (
                                    <div
                                      key={label}
                                      className={`px-2 py-1 rounded-md border text-xs font-medium ${getLabelColor(label)}`}
                                    >
                                      <span className="mr-1">{getLabelIcon(label)}</span>
                                      {label}: {count}
                                    </div>
                                  )
                                })}
                              </div>
                            </div>
                          )}

                          <div className="grid grid-cols-2 gap-4 text-xs mt-3">
                            <div>
                              <span className="text-gray-600">Label Coverage:</span>
                              <span className="ml-2 font-medium text-gray-900">
                                {Object.keys(status.label_counts || {}).length} labels
                              </span>
                            </div>
                            <div>
                              <span className="text-gray-600">Dataset Health:</span>
                              <span
                                className={`ml-2 font-medium ${
                                  isComplete ? 'text-green-600' : 'text-yellow-600'
                                }`}
                              >
                                {isComplete ? 'Ready' : 'In Progress'}
                              </span>
                            </div>
                          </div>
                          {status.snapshot_required && (
                            <div className="mt-3 pt-3 border-t border-gray-200">
                              <p className="text-xs text-yellow-700 mb-2">
                                ⚠️ This camera needs more labeled snapshots for training
                              </p>
                              <Button
                                size="sm"
                                onClick={() => {
                                  window.location.href = '/screenshots'
                                }}
                              >
                                Go to Screenshots
                              </Button>
                            </div>
                          )}
                        </div>
                      )
                    })()
                  ) : (
                    <div className="text-sm text-gray-500 italic">
                      Calculating dataset status...
                    </div>
                  )}
                </div>
              </div>
            )}
          </Card>

          {selectedCameraId ? (
            <CameraViewer
              onScreenshotSaved={() => {
                // Refresh cameras to get updated dataset status (Substep 2.2.2.2.4)
                const fetchCameras = async () => {
                  try {
                    const response = await api.get<{ cameras: Camera[]; count: number }>('/cameras')
                    const enabledCameras = response.cameras.filter((cam) => cam.enabled)
                    setCameras(enabledCameras)
                  } catch (err) {
                    console.error('Failed to refresh cameras:', err)
                  }
                }
                fetchCameras()
              }}
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
        <div className="space-y-4">
          <CameraGrid maxColumns={2} showSelector={true} />
          {/* Show dataset status badges for cameras in grid view */}
          {cameras.some((cam) => cam.dataset_status) && (
            <Card>
              <h3 className="text-sm font-semibold text-gray-900 mb-3">Dataset Status</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {cameras
                  .filter((cam) => cam.dataset_status)
                  .map((cam) => {
                    const status = cam.dataset_status!
                    // Fix progress calculation - handle division by zero (Substep 2.2.2.3.1)
                    const progress = status.required_snapshot_count > 0
                      ? Math.min(100, Math.max(0, (status.labeled_snapshot_count / status.required_snapshot_count) * 100))
                      : 0
                    const isComplete = !status.snapshot_required

                    return (
                      <div
                        key={cam.id}
                        className="p-3 bg-gray-50 rounded-md border border-gray-200"
                      >
                        <div className="flex items-center justify-between mb-2">
                          <span className="text-sm font-medium text-gray-900">{cam.name}</span>
                          {isComplete ? (
                            <span className="text-xs px-2 py-1 bg-green-100 text-green-800 rounded-full">
                              Ready
                            </span>
                          ) : (
                            <span className="text-xs px-2 py-1 bg-yellow-100 text-yellow-800 rounded-full">
                              Needs Snapshots
                            </span>
                          )}
                        </div>
                        <div className="w-full bg-gray-200 rounded-full h-2 mb-1">
                          <div
                            className={`h-2 rounded-full transition-all duration-500 ease-out ${
                              isComplete ? 'bg-green-500' : 'bg-yellow-500'
                            }`}
                            style={{ width: `${Math.min(progress, 100)}%` }}
                          />
                        </div>
                        <p className="text-xs text-gray-600">
                          {status.labeled_snapshot_count} / {status.required_snapshot_count} normal snapshots
                        </p>
                      </div>
                    )
                  })}
              </div>
            </Card>
          )}
        </div>
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

