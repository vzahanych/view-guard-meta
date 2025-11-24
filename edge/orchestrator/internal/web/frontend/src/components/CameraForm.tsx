import { useState, useEffect } from 'react'
import Input from './Input'
import Select from './Select'
import Button from './Button'
import Card from './Card'
import Loading from './Loading'
import { api } from '../utils/api'
import { X, Save } from 'lucide-react'

interface CameraFormProps {
  cameraId?: string
  onClose: () => void
  onSave: () => void
}

export default function CameraForm({ cameraId, onClose, onSave }: CameraFormProps) {
  const [loading, setLoading] = useState(!!cameraId)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [formData, setFormData] = useState({
    id: '',
    name: '',
    type: 'rtsp',
    enabled: true,
    rtsp_urls: [] as string[],
    device_path: '',
    ip_address: '',
    onvif_endpoint: '',
    manufacturer: '',
    model: '',
    recording_enabled: true,
    motion_detection: true,
    quality: '',
    frame_rate: 30,
    resolution: '',
  })

  useEffect(() => {
    if (cameraId) {
      const fetchCamera = async () => {
        try {
          setLoading(true)
        const camera = await api.get<any>(`/cameras/${cameraId}`)
          setFormData({
            id: camera.id,
            name: camera.name,
            type: camera.type,
            enabled: camera.enabled,
            rtsp_urls: camera.rtsp_urls || [],
            device_path: camera.device_path || '',
            ip_address: camera.ip_address || '',
            onvif_endpoint: camera.onvif_endpoint || '',
            manufacturer: camera.manufacturer || '',
            model: camera.model || '',
            recording_enabled: camera.config?.recording_enabled ?? true,
            motion_detection: camera.config?.motion_detection ?? true,
            quality: camera.config?.quality || '',
            frame_rate: camera.config?.frame_rate || 30,
            resolution: camera.config?.resolution || '',
          })
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Failed to load camera')
        } finally {
          setLoading(false)
        }
      }
      fetchCamera()
    }
  }, [cameraId])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError(null)

    try {
      const payload: any = {
        id: formData.id || `camera-${Date.now()}`,
        name: formData.name,
        type: formData.type,
        enabled: formData.enabled,
        config: {
          recording_enabled: formData.recording_enabled,
          motion_detection: formData.motion_detection,
          quality: formData.quality || undefined,
          frame_rate: formData.frame_rate || undefined,
          resolution: formData.resolution || undefined,
        },
      }

      if (formData.type === 'rtsp') {
        payload.rtsp_urls = formData.rtsp_urls.filter((url) => url.trim() !== '')
        if (payload.rtsp_urls.length === 0) {
          throw new Error('RTSP cameras require at least one RTSP URL')
        }
      } else if (formData.type === 'usb') {
        payload.device_path = formData.device_path
        if (!payload.device_path) {
          throw new Error('USB cameras require a device path')
        }
      } else if (formData.type === 'onvif') {
        payload.ip_address = formData.ip_address
        payload.onvif_endpoint = formData.onvif_endpoint
      }

      if (formData.manufacturer) payload.manufacturer = formData.manufacturer
      if (formData.model) payload.model = formData.model

      if (cameraId) {
        // Update existing camera
        await api.put(`/cameras/${cameraId}`, payload)
      } else {
        // Add new camera
        await api.post('/cameras', payload)
      }

      onSave()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save camera')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <Card>
        <Loading text="Loading camera..." />
      </Card>
    )
  }

  return (
    <Card>
      <div className="flex items-center justify-between mb-6">
        <h3 className="text-lg font-semibold text-gray-900">
          {cameraId ? 'Edit Camera' : 'Add Camera'}
        </h3>
        <button onClick={onClose} className="text-gray-400 hover:text-gray-600">
          <X className="h-6 w-6" />
        </button>
      </div>

      {error && (
        <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-600">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input
            label="Camera ID"
            value={formData.id}
            onChange={(e) => setFormData({ ...formData, id: e.target.value })}
            placeholder="camera-1"
            required
            disabled={!!cameraId}
          />
          <Input
            label="Name"
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            placeholder="Front Door Camera"
            required
          />
        </div>

        <Select
          label="Camera Type"
          value={formData.type}
          onChange={(e) => setFormData({ ...formData, type: e.target.value })}
          options={[
            { value: 'rtsp', label: 'RTSP' },
            { value: 'onvif', label: 'ONVIF' },
            { value: 'usb', label: 'USB' },
          ]}
        />

        {formData.type === 'rtsp' && (
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              RTSP URLs (one per line)
            </label>
            <textarea
              value={formData.rtsp_urls.join('\n')}
              onChange={(e) =>
                setFormData({
                  ...formData,
                  rtsp_urls: e.target.value.split('\n').filter((url) => url.trim() !== ''),
                })
              }
              className="input"
              placeholder="rtsp://192.168.1.100:554/stream"
              rows={3}
              required
            />
          </div>
        )}

        {formData.type === 'usb' && (
          <Input
            label="Device Path"
            value={formData.device_path}
            onChange={(e) => setFormData({ ...formData, device_path: e.target.value })}
            placeholder="/dev/video0"
            required
          />
        )}

        {formData.type === 'onvif' && (
          <>
            <Input
              label="IP Address"
              value={formData.ip_address}
              onChange={(e) => setFormData({ ...formData, ip_address: e.target.value })}
              placeholder="192.168.1.100"
            />
            <Input
              label="ONVIF Endpoint"
              value={formData.onvif_endpoint}
              onChange={(e) => setFormData({ ...formData, onvif_endpoint: e.target.value })}
              placeholder="http://192.168.1.100/onvif/device_service"
            />
          </>
        )}

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input
            label="Manufacturer"
            value={formData.manufacturer}
            onChange={(e) => setFormData({ ...formData, manufacturer: e.target.value })}
            placeholder="Optional"
          />
          <Input
            label="Model"
            value={formData.model}
            onChange={(e) => setFormData({ ...formData, model: e.target.value })}
            placeholder="Optional"
          />
        </div>

        <div className="border-t border-gray-200 pt-4">
          <h4 className="text-sm font-medium text-gray-900 mb-4">Camera Settings</h4>
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <label className="block text-sm font-medium text-gray-700">Enabled</label>
                <p className="text-xs text-gray-500">Enable this camera</p>
              </div>
              <label className="relative inline-flex items-center cursor-pointer">
                <input
                  type="checkbox"
                  checked={formData.enabled}
                  onChange={(e) => setFormData({ ...formData, enabled: e.target.checked })}
                  className="sr-only peer"
                />
                <div className="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
              </label>
            </div>
            <div className="flex items-center justify-between">
              <div>
                <label className="block text-sm font-medium text-gray-700">Recording Enabled</label>
                <p className="text-xs text-gray-500">Record video clips for this camera</p>
              </div>
              <label className="relative inline-flex items-center cursor-pointer">
                <input
                  type="checkbox"
                  checked={formData.recording_enabled}
                  onChange={(e) =>
                    setFormData({ ...formData, recording_enabled: e.target.checked })
                  }
                  className="sr-only peer"
                />
                <div className="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
              </label>
            </div>
            <div className="flex items-center justify-between">
              <div>
                <label className="block text-sm font-medium text-gray-700">Motion Detection</label>
                <p className="text-xs text-gray-500">Enable motion detection for this camera</p>
              </div>
              <label className="relative inline-flex items-center cursor-pointer">
                <input
                  type="checkbox"
                  checked={formData.motion_detection}
                  onChange={(e) =>
                    setFormData({ ...formData, motion_detection: e.target.checked })
                  }
                  className="sr-only peer"
                />
                <div className="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
              </label>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <Input
                label="Quality"
                value={formData.quality}
                onChange={(e) => setFormData({ ...formData, quality: e.target.value })}
                placeholder="high"
              />
              <Input
                label="Frame Rate"
                type="number"
                value={formData.frame_rate}
                onChange={(e) =>
                  setFormData({ ...formData, frame_rate: parseInt(e.target.value, 10) || 30 })
                }
                min={1}
                max={60}
              />
              <Input
                label="Resolution"
                value={formData.resolution}
                onChange={(e) => setFormData({ ...formData, resolution: e.target.value })}
                placeholder="1920x1080"
              />
            </div>
          </div>
        </div>

        <div className="flex justify-end space-x-2 pt-4 border-t border-gray-200">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={saving}>
            {saving ? (
              <>
                <div className="h-4 w-4 border-2 border-white border-t-transparent rounded-full animate-spin mr-2" />
                Saving...
              </>
            ) : (
              <>
                <Save className="h-4 w-4 mr-2" />
                {cameraId ? 'Update' : 'Add'} Camera
              </>
            )}
          </Button>
        </div>
      </form>
    </Card>
  )
}

