import { useState } from 'react'
import CameraList from '../components/CameraList'
import CameraForm from '../components/CameraForm'
import CameraDiscovery from '../components/CameraDiscovery'
import Button from '../components/Button'
import { api } from '../utils/api'
import { Plus, Search, Video } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

export default function CameraManagement() {
  const [showAddForm, setShowAddForm] = useState(false)
  const [editingCameraId, setEditingCameraId] = useState<string | null>(null)
  const [showDiscovery, setShowDiscovery] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)
  const navigate = useNavigate()

  const handleEdit = (cameraId: string) => {
    setEditingCameraId(cameraId)
    setShowAddForm(true)
  }

  const handleDelete = async (cameraId: string) => {
    if (!confirm(`Are you sure you want to delete camera "${cameraId}"?`)) {
      return
    }

    try {
      await api.delete(`/cameras/${cameraId}`)
      setRefreshKey((prev) => prev + 1)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to delete camera')
    }
  }

  const handleTest = async (cameraId: string) => {
    try {
      await api.post(`/cameras/${cameraId}/test`)
      setRefreshKey((prev) => prev + 1)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to test camera')
    }
  }

  const handleToggleEnabled = async (cameraId: string, enabled: boolean) => {
    try {
      await api.put(`/cameras/${cameraId}`, { enabled })
      setRefreshKey((prev) => prev + 1)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to update camera')
    }
  }

  const handleSave = () => {
    setShowAddForm(false)
    setEditingCameraId(null)
    setRefreshKey((prev) => prev + 1)
  }

  const handleAddFromDiscovery = () => {
    setShowDiscovery(false)
    setRefreshKey((prev) => prev + 1)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Camera Management</h1>
          <p className="mt-2 text-gray-600">Manage your cameras and view live streams</p>
        </div>
        <div className="flex space-x-2">
          <Button variant="secondary" onClick={() => navigate('/cameras')}>
            <Video className="h-4 w-4 mr-2" />
            View Streams
          </Button>
          <Button variant="secondary" onClick={() => setShowDiscovery(!showDiscovery)}>
            <Search className="h-4 w-4 mr-2" />
            {showDiscovery ? 'Hide' : 'Show'} Discovery
          </Button>
          <Button onClick={() => setShowAddForm(true)}>
            <Plus className="h-4 w-4 mr-2" />
            Add Camera
          </Button>
        </div>
      </div>

      {showDiscovery && (
        <CameraDiscovery onAddCamera={handleAddFromDiscovery} />
      )}

      {showAddForm && (
        <CameraForm
          cameraId={editingCameraId || undefined}
          onClose={() => {
            setShowAddForm(false)
            setEditingCameraId(null)
          }}
          onSave={handleSave}
        />
      )}

      <CameraList
        key={refreshKey}
        onEdit={handleEdit}
        onDelete={handleDelete}
        onTest={handleTest}
        onToggleEnabled={handleToggleEnabled}
      />
    </div>
  )
}

