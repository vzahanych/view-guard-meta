import { useState } from 'react'
import Card from './Card'
import Button from './Button'
import Loading from './Loading'
import { api } from '../utils/api'
import { Search, Plus, CheckCircle2 } from 'lucide-react'

interface DiscoveredCamera {
  id: string
  manufacturer?: string
  model?: string
  ip_address?: string
  onvif_endpoint?: string
  rtsp_urls?: string[]
  last_seen?: string
  discovered_at: string
  capabilities?: {
    has_ptz: boolean
    has_snapshot: boolean
    has_video_streams: boolean
  }
}

interface CameraDiscoveryProps {
  onAddCamera: (camera: DiscoveredCamera) => void
}

export default function CameraDiscovery({ onAddCamera }: CameraDiscoveryProps) {
  const [discovering, setDiscovering] = useState(false)
  const [discovered, setDiscovered] = useState<DiscoveredCamera[]>([])
  const [error, setError] = useState<string | null>(null)
  const [adding, setAdding] = useState<Set<string>>(new Set())

  const handleDiscover = async () => {
    setDiscovering(true)
    setError(null)
    setDiscovered([])

    try {
      const response = await api.get<{ discovered: DiscoveredCamera[]; count: number }>(
        '/cameras/discover'
      )
      setDiscovered(response.discovered)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to discover cameras')
    } finally {
      setDiscovering(false)
    }
  }

  const handleAdd = async (camera: DiscoveredCamera) => {
    setAdding((prev) => new Set(prev).add(camera.id))
    try {
      // Generate a camera name from available info
      const name =
        camera.model || camera.manufacturer || camera.ip_address || `Camera ${camera.id}`

      const payload: any = {
        id: camera.id,
        name: name,
        type: camera.onvif_endpoint ? 'onvif' : camera.rtsp_urls?.length ? 'rtsp' : 'usb',
        enabled: true,
        manufacturer: camera.manufacturer,
        model: camera.model,
        ip_address: camera.ip_address,
        onvif_endpoint: camera.onvif_endpoint,
        rtsp_urls: camera.rtsp_urls || [],
        config: {
          recording_enabled: true,
          motion_detection: true,
        },
      }

      await api.post('/cameras', payload)
      onAddCamera(camera)
    } catch (err) {
      console.error('Failed to add camera:', err)
    } finally {
      setAdding((prev) => {
        const next = new Set(prev)
        next.delete(camera.id)
        return next
      })
    }
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-4">
        <div>
          <h3 className="text-lg font-semibold text-gray-900">Camera Discovery</h3>
          <p className="text-sm text-gray-600 mt-1">
            Scan for cameras on your network (ONVIF and USB)
          </p>
        </div>
        <Button onClick={handleDiscover} disabled={discovering}>
          <Search className="h-4 w-4 mr-2" />
          {discovering ? 'Discovering...' : 'Discover Cameras'}
        </Button>
      </div>

      {error && (
        <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-600">
          {error}
        </div>
      )}

      {discovering && (
        <div className="py-8">
          <Loading text="Scanning for cameras..." />
        </div>
      )}

      {discovered.length > 0 && (
        <div className="space-y-3">
          <p className="text-sm text-gray-600">
            Found {discovered.length} camera{discovered.length !== 1 ? 's' : ''}
          </p>
          {discovered.map((camera) => (
            <div
              key={camera.id}
              className="border border-gray-200 rounded-lg p-4 flex items-start justify-between"
            >
              <div className="flex-1">
                <div className="flex items-center space-x-2 mb-2">
                  <h4 className="font-medium text-gray-900">
                    {camera.model || camera.manufacturer || camera.id}
                  </h4>
                  {camera.manufacturer && camera.model && (
                    <span className="text-sm text-gray-500">
                      {camera.manufacturer} {camera.model}
                    </span>
                  )}
                </div>
                <div className="space-y-1 text-sm text-gray-600">
                  {camera.ip_address && (
                    <div>
                      <span className="font-medium">IP:</span> {camera.ip_address}
                    </div>
                  )}
                  {camera.onvif_endpoint && (
                    <div>
                      <span className="font-medium">ONVIF:</span> {camera.onvif_endpoint}
                    </div>
                  )}
                  {camera.rtsp_urls && camera.rtsp_urls.length > 0 && (
                    <div>
                      <span className="font-medium">RTSP:</span> {camera.rtsp_urls.join(', ')}
                    </div>
                  )}
                  {camera.capabilities && (
                    <div className="flex space-x-2 mt-2">
                      {camera.capabilities.has_ptz && (
                        <span className="px-2 py-1 bg-blue-100 text-blue-800 text-xs rounded">
                          PTZ
                        </span>
                      )}
                      {camera.capabilities.has_snapshot && (
                        <span className="px-2 py-1 bg-green-100 text-green-800 text-xs rounded">
                          Snapshot
                        </span>
                      )}
                      {camera.capabilities.has_video_streams && (
                        <span className="px-2 py-1 bg-purple-100 text-purple-800 text-xs rounded">
                          Video
                        </span>
                      )}
                    </div>
                  )}
                </div>
              </div>
              <Button
                size="sm"
                onClick={() => handleAdd(camera)}
                disabled={adding.has(camera.id)}
                className="ml-4"
              >
                {adding.has(camera.id) ? (
                  <>
                    <div className="h-4 w-4 border-2 border-white border-t-transparent rounded-full animate-spin mr-2" />
                    Adding...
                  </>
                ) : (
                  <>
                    <Plus className="h-4 w-4 mr-1" />
                    Add
                  </>
                )}
              </Button>
            </div>
          ))}
        </div>
      )}

      {!discovering && discovered.length === 0 && !error && (
        <div className="text-center py-8 text-gray-500">
          <Search className="h-12 w-12 mx-auto mb-2 opacity-50" />
          <p>Click "Discover Cameras" to scan for cameras</p>
        </div>
      )}
    </Card>
  )
}

