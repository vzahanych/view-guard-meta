import { useState, useEffect } from 'react'
import Card from '../components/Card'
import Button from '../components/Button'
import Select from '../components/Select'
import Input from '../components/Input'
import Loading from '../components/Loading'
import { api } from '../utils/api'
import { Camera, Save, Trash2, Tag } from 'lucide-react'

interface Camera {
  id: string
  name: string
  type: string
  enabled: boolean
}

interface Screenshot {
  id: string
  camera_id: string
  file_path: string
  label: 'normal' | 'threat' | 'abnormal' | 'custom'
  custom_label?: string
  description?: string
  created_at: string
  updated_at: string
}

export default function Screenshots() {
  const [cameras, setCameras] = useState<Camera[]>([])
  const [screenshots, setScreenshots] = useState<Screenshot[]>([])
  const [selectedCameraId, setSelectedCameraId] = useState<string>('')
  const [filterLabel, setFilterLabel] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCaptureModal, setShowCaptureModal] = useState(false)
  const [captureLabel, setCaptureLabel] = useState<'normal' | 'threat' | 'abnormal' | 'custom'>('normal')
  const [captureCustomLabel, setCaptureCustomLabel] = useState('')
  const [captureDescription, setCaptureDescription] = useState('')
  const [capturedImage, setCapturedImage] = useState<string | null>(null)

  useEffect(() => {
    fetchCameras()
    fetchScreenshots()
  }, [])

  useEffect(() => {
    fetchScreenshots()
  }, [filterLabel, selectedCameraId])

  const fetchCameras = async () => {
    try {
      const response = await api.get<{ cameras: Camera[]; count: number }>('/cameras')
      const enabledCameras = response.cameras.filter((cam) => cam.enabled)
      setCameras(enabledCameras)
      if (enabledCameras.length > 0 && !selectedCameraId) {
        setSelectedCameraId(enabledCameras[0].id)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load cameras')
    }
  }

  const fetchScreenshots = async () => {
    try {
      setLoading(true)
      const params: Record<string, string> = {}
      if (filterLabel) params.label = filterLabel
      if (selectedCameraId) params.camera_id = selectedCameraId

      const queryString = new URLSearchParams(params).toString()
      const response = await api.get<{ screenshots: Screenshot[]; count: number }>(
        `/screenshots${queryString ? `?${queryString}` : ''}`
      )
      setScreenshots(response.screenshots)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load screenshots')
    } finally {
      setLoading(false)
    }
  }

  const captureScreenshot = async () => {
    if (!selectedCameraId) {
      setError('Please select a camera')
      return
    }

    try {
      setSaving(true)
      setError(null)

      // Capture snapshot from camera
      const snapshotResponse = await fetch(`/api/cameras/${selectedCameraId}/snapshot?t=${Date.now()}`)
      if (!snapshotResponse.ok) {
        throw new Error('Failed to capture snapshot')
      }

      const blob = await snapshotResponse.blob()
      const reader = new FileReader()
      reader.onloadend = () => {
        const base64data = reader.result as string
        setCapturedImage(base64data)
        setShowCaptureModal(true)
      }
      reader.readAsDataURL(blob)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to capture screenshot')
    } finally {
      setSaving(false)
    }
  }

  const saveScreenshot = async () => {
    if (!capturedImage || !selectedCameraId) {
      setError('No image captured or camera not selected')
      return
    }

    try {
      setSaving(true)
      setError(null)

      await api.post('/screenshots', {
        camera_id: selectedCameraId,
        label: captureLabel,
        custom_label: captureLabel === 'custom' ? captureCustomLabel : undefined,
        description: captureDescription || undefined,
        image_data: capturedImage,
      })

      setShowCaptureModal(false)
      setCapturedImage(null)
      setCaptureLabel('normal')
      setCaptureCustomLabel('')
      setCaptureDescription('')
      fetchScreenshots()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save screenshot')
    } finally {
      setSaving(false)
    }
  }

  const deleteScreenshot = async (id: string) => {
    if (!confirm('Are you sure you want to delete this screenshot?')) {
      return
    }

    try {
      await api.delete(`/screenshots/${id}`)
      fetchScreenshots()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete screenshot')
    }
  }

  const exportDataset = async () => {
    try {
      setExporting(true)
      setError(null)
      const payload = {
        camera_id: selectedCameraId || undefined,
        label: filterLabel || undefined,
        include_metadata: true,
      }
      const response = await fetch('/api/screenshots/export', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(payload),
      })
      if (!response.ok) {
        const errorText = await response.text()
        throw new Error(errorText || 'Failed to export dataset')
      }
      const blob = await response.blob()
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `dataset-${Date.now()}.zip`
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.URL.revokeObjectURL(url)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to export dataset')
    } finally {
      setExporting(false)
    }
  }

  const updateScreenshotLabel = async (id: string, label: string, customLabel?: string) => {
    try {
      await api.put(`/screenshots/${id}`, {
        label,
        custom_label: customLabel,
      })
      fetchScreenshots()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update screenshot')
    }
  }

  const getLabelColor = (label: string) => {
    switch (label) {
      case 'normal':
        return 'bg-green-100 text-green-800'
      case 'threat':
        return 'bg-red-100 text-red-800'
      case 'abnormal':
        return 'bg-yellow-100 text-yellow-800'
      default:
        return 'bg-gray-100 text-gray-800'
    }
  }

  const getLabelIcon = (label: string) => {
    switch (label) {
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

  if (loading && screenshots.length === 0) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loading />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Screenshot Management</h1>
          <p className="mt-2 text-gray-600">
            Capture, label, and manage screenshots for model training
          </p>
        </div>
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded">
          {error}
        </div>
      )}

      {/* Capture Section */}
      <Card>
        <div className="flex flex-wrap items-end gap-4">
          <Select
            label="Camera"
            value={selectedCameraId}
            onChange={(e) => setSelectedCameraId(e.target.value)}
            options={cameras.map((cam) => ({ value: cam.id, label: cam.name }))}
          />
          <Button onClick={captureScreenshot} disabled={!selectedCameraId || saving}>
            <Camera className="w-4 h-4 mr-2" />
            {saving ? 'Capturing...' : 'Capture Screenshot'}
          </Button>
          <Button variant="outline" onClick={exportDataset} disabled={exporting}>
            {exporting ? 'Exporting...' : 'Export Dataset'}
          </Button>
        </div>
      </Card>

      {/* Filters */}
      <Card>
        <div className="flex items-center gap-4">
          <Select
            label="Filter by Label"
            value={filterLabel}
            onChange={(e) => setFilterLabel(e.target.value)}
            options={[
              { value: '', label: 'All Labels' },
              { value: 'normal', label: 'Normal' },
              { value: 'threat', label: 'Threat' },
              { value: 'abnormal', label: 'Abnormal' },
              { value: 'custom', label: 'Custom' },
            ]}
          />
        </div>
      </Card>

      {/* Screenshots Grid */}
      {screenshots.length === 0 ? (
        <Card>
          <div className="text-center py-12">
            <Tag className="w-12 h-12 text-gray-400 mx-auto mb-4" />
            <p className="text-gray-500">No screenshots found. Capture one to get started.</p>
          </div>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {screenshots.map((screenshot) => (
            <Card key={screenshot.id}>
              <div className="space-y-3">
                <div className="relative">
                  <img
                    src={`/api/screenshots/${screenshot.id}/image`}
                    alt={`Screenshot ${screenshot.id}`}
                    className="w-full h-48 object-cover rounded"
                  />
                  <div className="absolute top-2 right-2">
                    <span
                      className={`px-2 py-1 rounded text-xs font-semibold ${getLabelColor(
                        screenshot.label
                      )}`}
                    >
                      {getLabelIcon(screenshot.label)} {screenshot.label}
                    </span>
                  </div>
                </div>
                <div>
                  <p className="text-sm text-gray-600">
                    Camera: {cameras.find((c) => c.id === screenshot.camera_id)?.name || screenshot.camera_id}
                  </p>
                  {screenshot.custom_label && (
                    <p className="text-sm text-gray-600">Custom: {screenshot.custom_label}</p>
                  )}
                  {screenshot.description && (
                    <p className="text-sm text-gray-500 mt-1">{screenshot.description}</p>
                  )}
                  <p className="text-xs text-gray-400 mt-1">
                    {new Date(screenshot.created_at).toLocaleString()}
                  </p>
                </div>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      const newLabel = screenshot.label === 'normal' ? 'threat' : 'normal'
                      updateScreenshotLabel(screenshot.id, newLabel)
                    }}
                  >
                    <Tag className="w-3 h-3 mr-1" />
                    Re-label
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => deleteScreenshot(screenshot.id)}
                  >
                    <Trash2 className="w-3 h-3 mr-1" />
                    Delete
                  </Button>
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}

      {/* Capture Modal */}
      {showCaptureModal && capturedImage && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 max-w-2xl w-full mx-4 max-h-[90vh] overflow-y-auto">
            <h2 className="text-xl font-bold mb-4">Label Screenshot</h2>
            <div className="space-y-4">
              <div>
                <img src={capturedImage} alt="Captured" className="w-full rounded" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">Label</label>
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
              <div className="flex gap-2 justify-end">
                <Button variant="outline" onClick={() => setShowCaptureModal(false)}>
                  Cancel
                </Button>
                <Button onClick={saveScreenshot} disabled={saving}>
                  <Save className="w-4 h-4 mr-2" />
                  {saving ? 'Saving...' : 'Save Screenshot'}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

