import { Clock, Camera, AlertCircle, Download, Eye } from 'lucide-react'
import Button from './Button'
import Card from './Card'

interface EventCardProps {
  event: {
    id: string
    camera_id: string
    event_type: string
    timestamp: string
    confidence: number
    snapshot_path?: string
    clip_path?: string
  }
  onViewDetails: (eventId: string) => void
}

export default function EventCard({ event, onViewDetails }: EventCardProps) {
  const formatDate = (timestamp: string) => {
    const date = new Date(timestamp)
    return {
      date: date.toLocaleDateString(),
      time: date.toLocaleTimeString(),
      full: date.toLocaleString(),
    }
  }

  const dateInfo = formatDate(event.timestamp)
  const hasMedia = !!(event.snapshot_path || event.clip_path)

  const getEventTypeColor = (eventType: string) => {
    const colors: Record<string, string> = {
      person_detected: 'bg-blue-100 text-blue-800',
      motion_detected: 'bg-yellow-100 text-yellow-800',
      object_detected: 'bg-purple-100 text-purple-800',
      alert: 'bg-red-100 text-red-800',
    }
    return colors[eventType] || 'bg-gray-100 text-gray-800'
  }

  const getEventTypeLabel = (eventType: string) => {
    return eventType
      .split('_')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ')
  }

  return (
    <Card className="hover:shadow-md transition-shadow">
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <div className="flex items-center space-x-2 mb-2">
            <span
              className={`px-2 py-1 rounded text-xs font-medium ${getEventTypeColor(
                event.event_type
              )}`}
            >
              {getEventTypeLabel(event.event_type)}
            </span>
            <span className="text-sm text-gray-500">
              {(event.confidence * 100).toFixed(0)}% confidence
            </span>
          </div>

          <div className="space-y-2">
            <div className="flex items-center text-sm text-gray-600">
              <Clock className="h-4 w-4 mr-2" />
              <span>{dateInfo.full}</span>
            </div>
            <div className="flex items-center text-sm text-gray-600">
              <Camera className="h-4 w-4 mr-2" />
              <span>{event.camera_id}</span>
            </div>
            {hasMedia && (
              <div className="flex items-center text-sm text-gray-600">
                <AlertCircle className="h-4 w-4 mr-2" />
                <span>
                  {event.snapshot_path && 'Snapshot'}
                  {event.snapshot_path && event.clip_path && ' • '}
                  {event.clip_path && 'Clip'}
                </span>
              </div>
            )}
          </div>
        </div>

        <div className="ml-4 flex flex-col space-y-2">
          {event.snapshot_path && (
            <img
              src={`/api/snapshots/${event.id}`}
              alt="Event snapshot"
              className="w-24 h-16 object-cover rounded border border-gray-200"
              onError={(e) => {
                e.currentTarget.style.display = 'none'
              }}
            />
          )}
          <Button size="sm" onClick={() => onViewDetails(event.id)}>
            <Eye className="h-4 w-4 mr-1" />
            View
          </Button>
        </div>
      </div>
    </Card>
  )
}

