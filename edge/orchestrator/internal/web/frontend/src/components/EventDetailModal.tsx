import { X, Download, Calendar, Camera, AlertCircle } from 'lucide-react'
import Button from './Button'
import ClipViewer from './ClipViewer'

interface BoundingBox {
  x1: number
  y1: number
  x2: number
  y2: number
  class_id?: number
  class_name?: string
  confidence?: number
}

interface EventDetail {
  id: string
  camera_id: string
  event_type: string
  timestamp: string
  confidence: number
  metadata?: Record<string, any>
  snapshot_path?: string
  clip_path?: string
  bounding_box?: BoundingBox
}

interface EventDetailModalProps {
  event: EventDetail | null
  onClose: () => void
}

export default function EventDetailModal({ event, onClose }: EventDetailModalProps) {
  if (!event) return null

  const formatDate = (timestamp: string) => {
    return new Date(timestamp).toLocaleString()
  }

  const getEventTypeLabel = (eventType: string) => {
    return eventType
      .split('_')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ')
  }

  const handleDownloadClip = () => {
    if (event.clip_path) {
      window.open(`/api/clips/${event.id}/download`, '_blank')
    }
  }

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4">
      <div className="bg-white rounded-lg max-w-4xl w-full max-h-[90vh] overflow-y-auto">
        {/* Header */}
        <div className="sticky top-0 bg-white border-b border-gray-200 px-6 py-4 flex items-center justify-between">
          <h2 className="text-xl font-semibold text-gray-900">
            Event Details: {getEventTypeLabel(event.event_type)}
          </h2>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 transition-colors"
          >
            <X className="h-6 w-6" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 space-y-6">
          {/* Event Info */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <div className="flex items-center text-sm text-gray-600 mb-1">
                <Calendar className="h-4 w-4 mr-2" />
                <span className="font-medium">Timestamp</span>
              </div>
              <p className="text-gray-900">{formatDate(event.timestamp)}</p>
            </div>
            <div>
              <div className="flex items-center text-sm text-gray-600 mb-1">
                <Camera className="h-4 w-4 mr-2" />
                <span className="font-medium">Camera</span>
              </div>
              <p className="text-gray-900">{event.camera_id}</p>
            </div>
            <div>
              <div className="flex items-center text-sm text-gray-600 mb-1">
                <AlertCircle className="h-4 w-4 mr-2" />
                <span className="font-medium">Confidence</span>
              </div>
              <p className="text-gray-900">{(event.confidence * 100).toFixed(1)}%</p>
            </div>
            <div>
              <div className="text-sm text-gray-600 mb-1">
                <span className="font-medium">Event Type</span>
              </div>
              <p className="text-gray-900">{getEventTypeLabel(event.event_type)}</p>
            </div>
          </div>

          {/* Bounding Box Info */}
          {event.bounding_box && (
            <div className="border-t border-gray-200 pt-4">
              <h3 className="text-sm font-medium text-gray-900 mb-2">Detection Details</h3>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                <div>
                  <span className="text-gray-600">Class: </span>
                  <span className="text-gray-900">
                    {event.bounding_box.class_name || 'Unknown'}
                  </span>
                </div>
                {event.bounding_box.confidence !== undefined && (
                  <div>
                    <span className="text-gray-600">Confidence: </span>
                    <span className="text-gray-900">
                      {(event.bounding_box.confidence * 100).toFixed(1)}%
                    </span>
                  </div>
                )}
                <div>
                  <span className="text-gray-600">Position: </span>
                  <span className="text-gray-900">
                    ({event.bounding_box.x1}, {event.bounding_box.y1}) - (
                    {event.bounding_box.x2}, {event.bounding_box.y2})
                  </span>
                </div>
              </div>
            </div>
          )}

          {/* Metadata */}
          {event.metadata && Object.keys(event.metadata).length > 0 && (
            <div className="border-t border-gray-200 pt-4">
              <h3 className="text-sm font-medium text-gray-900 mb-2">Metadata</h3>
              <pre className="bg-gray-50 p-4 rounded text-xs overflow-x-auto">
                {JSON.stringify(event.metadata, null, 2)}
              </pre>
            </div>
          )}

          {/* Snapshot */}
          {event.snapshot_path && (
            <div className="border-t border-gray-200 pt-4">
              <h3 className="text-sm font-medium text-gray-900 mb-2">Snapshot</h3>
              <img
                src={`/api/snapshots/${event.id}`}
                alt="Event snapshot"
                className="max-w-full h-auto rounded border border-gray-200"
                onError={(e) => {
                  e.currentTarget.style.display = 'none'
                  const parent = e.currentTarget.parentElement
                  if (parent) {
                    parent.innerHTML = '<p class="text-gray-500">Snapshot not available</p>'
                  }
                }}
              />
            </div>
          )}

          {/* Clip */}
          {event.clip_path && (
            <div className="border-t border-gray-200 pt-4">
              <div className="flex items-center justify-between mb-2">
                <h3 className="text-sm font-medium text-gray-900">Video Clip</h3>
                <Button size="sm" variant="secondary" onClick={handleDownloadClip}>
                  <Download className="h-4 w-4 mr-1" />
                  Download
                </Button>
              </div>
              <ClipViewer eventId={event.id} />
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="sticky bottom-0 bg-gray-50 border-t border-gray-200 px-6 py-4 flex justify-end">
          <Button variant="secondary" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    </div>
  )
}

