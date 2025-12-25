import { useState, useRef, useEffect } from 'react'
import { Play, Pause, Maximize2, Minimize2, RefreshCw, Camera, Save } from 'lucide-react'
import Button from './Button'
import Select from './Select'
import Input from './Input'
import { api } from '../utils/api'

interface CameraViewerProps {
  cameraId: string
  cameraName?: string
  className?: string
  onError?: (error: string) => void
  onScreenshotSaved?: () => void // Callback when screenshot is saved (to refresh cameras)
}

export default function CameraViewer({
  cameraId,
  cameraName,
  className = '',
  onError,
  onScreenshotSaved,
}: CameraViewerProps) {
  const [isPlaying, setIsPlaying] = useState(true)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [snapshotUrl, setSnapshotUrl] = useState<string | null>(null)
  const [isCapturingSnapshot, setIsCapturingSnapshot] = useState(false)
  const [showCaptureModal, setShowCaptureModal] = useState(false)
  const [capturedImage, setCapturedImage] = useState<string | null>(null)
  const [captureLabel, setCaptureLabel] = useState<'normal' | 'threat' | 'abnormal' | 'custom'>('normal')
  const [captureCustomLabel, setCaptureCustomLabel] = useState('')
  const [captureDescription, setCaptureDescription] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saveSuccess, setSaveSuccess] = useState<string | null>(null)
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
    setSaveError(null)
    setSaveSuccess(null)
    try {
      // Capture snapshot from camera
      const snapshotResponse = await fetch(`/api/cameras/${cameraId}/snapshot?t=${Date.now()}`)
      if (!snapshotResponse.ok) {
        throw new Error('Failed to capture snapshot')
      }

      const blob = await snapshotResponse.blob()
      const reader = new FileReader()
      reader.onloadend = () => {
        const base64data = reader.result as string
        setCapturedImage(base64data)
        setShowCaptureModal(true) // Open modal instead of showing inline (Substep 2.2.2.2.4)
      }
      reader.readAsDataURL(blob)
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

  const saveScreenshot = async () => {
    // Validation
    if (!capturedImage) {
      setSaveError('No image captured')
      return
    }

    if (captureLabel === 'custom' && !captureCustomLabel.trim()) {
      setSaveError('Custom label is required when label is "custom"')
      return
    }

    try {
      setSaving(true)
      setSaveError(null)
      setSaveSuccess(null)

      const response = await api.post<{
        id: string
        dataset_status?: {
          labeled_snapshot_count: number
          required_snapshot_count: number
          snapshot_required: boolean
        }
      }>('/screenshots', {
        camera_id: cameraId,
        label: captureLabel,
        custom_label: captureLabel === 'custom' ? captureCustomLabel : undefined,
        description: captureDescription || undefined,
      })

      // Show success message
      const updatedCount = response.dataset_status?.labeled_snapshot_count
      const requiredCount = response.dataset_status?.required_snapshot_count
      if (updatedCount !== undefined && requiredCount !== undefined) {
        setSaveSuccess(`Screenshot saved! Progress: ${updatedCount}/${requiredCount} normal snapshots.`)
      } else {
        setSaveSuccess('Screenshot saved successfully!')
      }

      // Clear capture state
      setCapturedImage(null)
      setCaptureLabel('normal')
      setCaptureCustomLabel('')
      setCaptureDescription('')

      // Call callback to refresh cameras in parent
      if (onScreenshotSaved) {
        onScreenshotSaved()
      }

      // Close modal after a short delay
      setTimeout(() => {
        setShowCaptureModal(false)
        setSaveSuccess(null)
      }, 2000)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save screenshot')
    } finally {
      setSaving(false)
    }
  }

  const cancelCapture = () => {
    setShowCaptureModal(false)
    setCapturedImage(null)
    setCaptureLabel('normal')
    setCaptureCustomLabel('')
    setCaptureDescription('')
    setSaveError(null)
    setSaveSuccess(null)
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

      {/* Snapshot Capture Modal (Substep 2.2.2.2.4) */}
      {showCaptureModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 max-w-2xl w-full mx-4 max-h-[90vh] overflow-y-auto">
            <h2 className="text-xl font-bold mb-4">Label Screenshot</h2>
            <div className="space-y-4">
              {/* Show preview of captured image */}
              {capturedImage ? (
                <div>
                  <img src={capturedImage} alt="Captured" className="w-full rounded" />
                </div>
              ) : (
                <div className="border-2 border-dashed border-gray-300 rounded-lg p-8 text-center">
                  <p className="text-gray-500">No image captured yet.</p>
                </div>
              )}

              {/* Show error message if save failed */}
              {saveError && (
                <div className="bg-red-50 border border-red-200 rounded-lg p-3">
                  <p className="text-sm text-red-800">{saveError}</p>
                </div>
              )}

              {/* Show success message after save */}
              {saveSuccess && (
                <div className="bg-green-50 border border-green-200 rounded-lg p-3">
                  <p className="text-sm text-green-800">{saveSuccess}</p>
                </div>
              )}

              {/* Labeling form - only show if image is captured */}
              {capturedImage && (
                <>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Label <span className="text-red-500">*</span>
                    </label>
                    <Select
                      value={captureLabel}
                      onChange={(e) =>
                        setCaptureLabel(e.target.value as 'normal' | 'threat' | 'abnormal' | 'custom')
                      }
                      options={[
                        { value: 'normal', label: 'Normal' },
                        { value: 'threat', label: 'Threat' },
                        { value: 'abnormal', label: 'Abnormal' },
                        { value: 'custom', label: 'Custom' },
                      ]}
                    />
                  </div>
                  {captureLabel === 'custom' && (
                    <div>
                      <Input
                        label="Custom Label"
                        value={captureCustomLabel}
                        onChange={(e) => setCaptureCustomLabel(e.target.value)}
                        placeholder="Enter custom label"
                        required
                      />
                    </div>
                  )}
                  <div>
                    <Input
                      label="Description (optional)"
                      value={captureDescription}
                      onChange={(e) => setCaptureDescription(e.target.value)}
                      placeholder="Describe what you see in this image"
                    />
                  </div>
                </>
              )}

              <div className="flex gap-2 justify-end">
                {capturedImage ? (
                  <>
                    <Button variant="secondary" onClick={cancelCapture} disabled={saving}>
                      Reject
                    </Button>
                    <Button
                      onClick={saveScreenshot}
                      disabled={saving || (captureLabel === 'custom' && !captureCustomLabel.trim())}
                    >
                      <Save className="w-4 h-4 mr-2" />
                      {saving ? 'Saving...' : 'Save Screenshot'}
                    </Button>
                  </>
                ) : (
                  <Button variant="secondary" onClick={cancelCapture}>
                    Close
                  </Button>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

