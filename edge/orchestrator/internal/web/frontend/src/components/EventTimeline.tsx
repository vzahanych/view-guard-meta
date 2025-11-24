import { useState, useEffect } from 'react'
import EventCard from './EventCard'
import EventDetailModal from './EventDetailModal'
import Loading from './Loading'
import Button from './Button'
import Select from './Select'
import Input from './Input'
import { api } from '../utils/api'
import { ChevronLeft, ChevronRight, Filter, X } from 'lucide-react'

interface Event {
  id: string
  camera_id: string
  event_type: string
  timestamp: string
  confidence: number
  snapshot_path?: string
  clip_path?: string
}

interface EventDetail extends Event {
  metadata?: Record<string, any>
  bounding_box?: {
    x1: number
    y1: number
    x2: number
    y2: number
    class_id?: number
    class_name?: string
    confidence?: number
  }
}

interface EventTimelineProps {
  cameras?: Array<{ id: string; name: string }>
}

export default function EventTimeline({ cameras = [] }: EventTimelineProps) {
  const [events, setEvents] = useState<Event[]>([])
  const [selectedEvent, setSelectedEvent] = useState<EventDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [limit] = useState(20)

  // Filters
  const [showFilters, setShowFilters] = useState(false)
  const [cameraFilter, setCameraFilter] = useState<string>('')
  const [eventTypeFilter, setEventTypeFilter] = useState<string>('')
  const [startDate, setStartDate] = useState<string>('')
  const [endDate, setEndDate] = useState<string>('')

  const fetchEvents = async () => {
    try {
      setLoading(true)
      setError(null)

      const params = new URLSearchParams()
      params.append('limit', limit.toString())
      params.append('offset', ((page - 1) * limit).toString())
      params.append('order_by', 'timestamp DESC')

      if (cameraFilter) {
        params.append('camera_id', cameraFilter)
      }
      if (eventTypeFilter) {
        params.append('event_type', eventTypeFilter)
      }
      if (startDate) {
        params.append('start_time', new Date(startDate).toISOString())
      }
      if (endDate) {
        params.append('end_time', new Date(endDate).toISOString())
      }

      const response = await api.get<{
        events: Event[]
        count: number
        total: number
        limit: number
        offset: number
      }>(`/events?${params.toString()}`)

      setEvents(response.events)
      setTotal(response.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load events')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchEvents()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, cameraFilter, eventTypeFilter, startDate, endDate, limit])

  const handleViewDetails = async (eventId: string) => {
    try {
      const event = await api.get<EventDetail>(`/events/${eventId}`)
      setSelectedEvent(event)
    } catch (err) {
      console.error('Failed to load event details:', err)
    }
  }

  const handleClearFilters = () => {
    setCameraFilter('')
    setEventTypeFilter('')
    setStartDate('')
    setEndDate('')
    setPage(1)
  }

  const totalPages = Math.ceil(total / limit)
  const hasFilters = !!(cameraFilter || eventTypeFilter || startDate || endDate)

  const eventTypeOptions = [
    { value: '', label: 'All Types' },
    { value: 'person_detected', label: 'Person Detected' },
    { value: 'motion_detected', label: 'Motion Detected' },
    { value: 'object_detected', label: 'Object Detected' },
    { value: 'alert', label: 'Alert' },
  ]

  const cameraOptions = [
    { value: '', label: 'All Cameras' },
    ...cameras.map((cam) => ({ value: cam.id, label: cam.name })),
  ]

  return (
    <div className="space-y-4">
      {/* Filters */}
      <div className="card">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold text-gray-900">Filters</h3>
          <div className="flex items-center space-x-2">
            {hasFilters && (
              <Button size="sm" variant="secondary" onClick={handleClearFilters}>
                <X className="h-4 w-4 mr-1" />
                Clear
              </Button>
            )}
            <Button
              size="sm"
              variant={showFilters ? 'primary' : 'secondary'}
              onClick={() => setShowFilters(!showFilters)}
            >
              <Filter className="h-4 w-4 mr-1" />
              {showFilters ? 'Hide' : 'Show'} Filters
            </Button>
          </div>
        </div>

        {showFilters && (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <Select
              label="Camera"
              value={cameraFilter}
              onChange={(e) => {
                setCameraFilter(e.target.value)
                setPage(1)
              }}
              options={cameraOptions}
            />
            <Select
              label="Event Type"
              value={eventTypeFilter}
              onChange={(e) => {
                setEventTypeFilter(e.target.value)
                setPage(1)
              }}
              options={eventTypeOptions}
            />
            <Input
              label="Start Date"
              type="datetime-local"
              value={startDate}
              onChange={(e) => {
                setStartDate(e.target.value)
                setPage(1)
              }}
            />
            <Input
              label="End Date"
              type="datetime-local"
              value={endDate}
              onChange={(e) => {
                setEndDate(e.target.value)
                setPage(1)
              }}
            />
          </div>
        )}
      </div>

      {/* Events List */}
      {loading ? (
        <Loading text="Loading events..." />
      ) : error ? (
        <div className="card">
          <p className="text-red-600">{error}</p>
          <Button onClick={fetchEvents} className="mt-4">
            Retry
          </Button>
        </div>
      ) : events.length === 0 ? (
        <div className="card">
          <p className="text-gray-600">No events found</p>
        </div>
      ) : (
        <>
          <div className="space-y-4">
            {events.map((event) => (
              <EventCard
                key={event.id}
                event={event}
                onViewDetails={handleViewDetails}
              />
            ))}
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between card">
              <div className="text-sm text-gray-600">
                Showing {(page - 1) * limit + 1} to {Math.min(page * limit, total)} of {total}{' '}
                events
              </div>
              <div className="flex items-center space-x-2">
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => setPage(page - 1)}
                  disabled={page === 1}
                >
                  <ChevronLeft className="h-4 w-4" />
                  Previous
                </Button>
                <span className="text-sm text-gray-600">
                  Page {page} of {totalPages}
                </span>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => setPage(page + 1)}
                  disabled={page >= totalPages}
                >
                  Next
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </>
      )}

      {/* Event Detail Modal */}
      {selectedEvent && (
        <EventDetailModal
          event={selectedEvent}
          onClose={() => setSelectedEvent(null)}
        />
      )}
    </div>
  )
}

