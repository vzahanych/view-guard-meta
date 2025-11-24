import { useState, useEffect } from 'react'
import EventTimeline from '../components/EventTimeline'
import { api } from '../utils/api'

interface Camera {
  id: string
  name: string
}

export default function Events() {
  const [cameras, setCameras] = useState<Camera[]>([])

  useEffect(() => {
    const fetchCameras = async () => {
      try {
        const response = await api.get<{ cameras: Camera[]; count: number }>('/cameras')
        setCameras(response.cameras)
      } catch (err) {
        console.error('Failed to load cameras:', err)
      }
    }

    fetchCameras()
  }, [])

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-gray-900">Events</h1>
        <p className="mt-2 text-gray-600">Event timeline and history</p>
      </div>
      <EventTimeline cameras={cameras} />
    </div>
  )
}

