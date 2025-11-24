import { useRef, useEffect } from 'react'
import Loading from './Loading'

interface ClipViewerProps {
  eventId: string
  className?: string
}

export default function ClipViewer({ eventId, className = '' }: ClipViewerProps) {
  const videoRef = useRef<HTMLVideoElement>(null)

  useEffect(() => {
    const video = videoRef.current
    if (!video) return

    // Set video source
    video.src = `/api/clips/${eventId}/play`

    // Handle errors
    const handleError = () => {
      console.error('Failed to load video clip')
    }

    video.addEventListener('error', handleError)

    return () => {
      video.removeEventListener('error', handleError)
    }
  }, [eventId])

  return (
    <div className={`relative bg-black rounded-lg overflow-hidden ${className}`}>
      <video
        ref={videoRef}
        controls
        className="w-full h-auto max-h-[600px]"
        preload="metadata"
      >
        Your browser does not support the video tag.
      </video>
    </div>
  )
}

