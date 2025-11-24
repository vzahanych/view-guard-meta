import { useState, useRef, useEffect } from 'react'
import { Play, Pause, Maximize2, Minimize2, RefreshCw, Camera } from 'lucide-react'
import Button from './Button'

interface CameraViewerProps {
  cameraId: string
  cameraName?: string
  className?: string
  onError?: (error: string) => void
}

export default function CameraViewer({
  cameraId,
  cameraName,
  className = '',
  onError,
}: CameraViewerProps) {
  const [isPlaying, setIsPlaying] = useState(true)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [snapshotUrl, setSnapshotUrl] = useState<string | null>(null)
  const [isCapturingSnapshot, setIsCapturingSnapshot] = useState(false)
  const imgRef = useRef<HTMLImageElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  const streamUrl = `/api/cameras/${cameraId}/stream`

  useEffect(() => {
    if (!isPlaying) return

    const img = imgRef.current
    if (!img) return

    setIsLoading(true)
    setError(null)

    // Add timestamp to prevent caching
    const urlWithTimestamp = `${streamUrl}?t=${Date.now()}`
    img.src = urlWithTimestamp

    const handleLoad = () => {
      setIsLoading(false)
      setError(null)
    }

    const handleError = () => {
      setIsLoading(false)
      const errorMsg = `Failed to load stream from camera ${cameraId}`
      setError(errorMsg)
      if (onError) {
        onError(errorMsg)
      }
    }

    img.addEventListener('load', handleLoad)
    img.addEventListener('error', handleError)

    return () => {
      img.removeEventListener('load', handleLoad)
      img.removeEventListener('error', handleError)
    }
  }, [cameraId, streamUrl, isPlaying, onError])

  const togglePlayPause = () => {
    setIsPlaying(!isPlaying)
    if (imgRef.current) {
      if (!isPlaying) {
        // Resume stream
        imgRef.current.src = `${streamUrl}?t=${Date.now()}`
      } else {
        // Pause stream
        imgRef.current.src = ''
      }
    }
  }

  const toggleFullscreen = () => {
    if (!containerRef.current) return

    if (!isFullscreen) {
      if (containerRef.current.requestFullscreen) {
        containerRef.current.requestFullscreen()
      }
    } else {
      if (document.exitFullscreen) {
        document.exitFullscreen()
      }
    }
  }

  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement)
    }

    document.addEventListener('fullscreenchange', handleFullscreenChange)
    return () => {
      document.removeEventListener('fullscreenchange', handleFullscreenChange)
    }
  }, [])

  const refreshStream = () => {
    if (imgRef.current && isPlaying) {
      imgRef.current.src = `${streamUrl}?t=${Date.now()}`
    }
  }

  const captureSnapshot = async () => {
    setIsCapturingSnapshot(true)
    try {
      // Add timestamp to prevent caching
      const snapshotUrl = `/api/cameras/${cameraId}/snapshot?t=${Date.now()}`
      setSnapshotUrl(snapshotUrl)
    } catch (err) {
      const errorMsg = `Failed to capture snapshot: ${err instanceof Error ? err.message : 'Unknown error'}`
      setError(errorMsg)
      if (onError) {
        onError(errorMsg)
      }
    } finally {
      setIsCapturingSnapshot(false)
    }
  }

  return (
    <div
      ref={containerRef}
      className={`relative bg-black rounded-lg overflow-hidden ${className}`}
    >
      {/* Stream Image */}
      <div className="relative w-full h-full flex items-center justify-center min-h-[400px]">
        {isLoading && isPlaying && (
          <div className="absolute inset-0 flex items-center justify-center bg-gray-900">
            <div className="text-white">
              <RefreshCw className="h-8 w-8 animate-spin mx-auto mb-2" />
              <p className="text-sm">Loading stream...</p>
            </div>
          </div>
        )}
        {error && (
          <div className="absolute inset-0 flex items-center justify-center bg-gray-900">
            <div className="text-white text-center p-4">
              <p className="text-red-400 mb-2">{error}</p>
              <Button size="sm" onClick={refreshStream}>
                Retry
              </Button>
            </div>
          </div>
        )}
        {!isPlaying && (
          <div className="absolute inset-0 flex items-center justify-center bg-gray-900">
            <div className="text-white text-center">
              <Pause className="h-12 w-12 mx-auto mb-2 opacity-50" />
              <p className="text-sm">Stream paused</p>
            </div>
          </div>
        )}
        <img
          ref={imgRef}
          alt={cameraName || `Camera ${cameraId}`}
          className={`w-full h-full object-contain ${
            !isPlaying || isLoading || error ? 'hidden' : ''
          }`}
          style={{ display: !isPlaying || isLoading || error ? 'none' : 'block' }}
        />
        {snapshotUrl && (
          <div className="absolute top-4 right-4 bg-black/80 rounded-lg p-2 max-w-xs">
            <div className="flex items-center justify-between mb-2">
              <span className="text-white text-xs font-medium">Snapshot</span>
              <Button
                size="sm"
                variant="secondary"
                onClick={() => setSnapshotUrl(null)}
                className="bg-white/20 hover:bg-white/30 text-white border-0 h-6 px-2"
              >
                ×
              </Button>
            </div>
            <img
              src={snapshotUrl}
              alt="Camera snapshot"
              className="w-full rounded"
              onError={() => {
                setError('Failed to load snapshot')
                setSnapshotUrl(null)
              }}
            />
          </div>
        )}
      </div>

      {/* Controls Overlay */}
      <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/80 to-transparent p-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <Button
              size="sm"
              variant="secondary"
              onClick={togglePlayPause}
              className="bg-white/20 hover:bg-white/30 text-white border-0"
            >
              {isPlaying ? (
                <Pause className="h-4 w-4" />
              ) : (
                <Play className="h-4 w-4" />
              )}
            </Button>
            <Button
              size="sm"
              variant="secondary"
              onClick={refreshStream}
              className="bg-white/20 hover:bg-white/30 text-white border-0"
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
            <Button
              size="sm"
              variant="secondary"
              onClick={captureSnapshot}
              disabled={isCapturingSnapshot}
              className="bg-white/20 hover:bg-white/30 text-white border-0"
              title="Capture snapshot"
            >
              <Camera className="h-4 w-4" />
            </Button>
            {cameraName && (
              <span className="text-white text-sm font-medium ml-2">
                {cameraName}
              </span>
            )}
          </div>
          <Button
            size="sm"
            variant="secondary"
            onClick={toggleFullscreen}
            className="bg-white/20 hover:bg-white/30 text-white border-0"
          >
            {isFullscreen ? (
              <Minimize2 className="h-4 w-4" />
            ) : (
              <Maximize2 className="h-4 w-4" />
            )}
          </Button>
        </div>
      </div>
    </div>
  )
}

