import { Camera, CheckSquare, ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Edit, Eye, Info, Maximize2, RefreshCw, RotateCcw, Save, Square, Tag, Trash2, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Button from '../components/Button'
import Card from '../components/Card'
import Input from '../components/Input'
import Select from '../components/Select'
import { ScreenshotSkeleton } from '../components/Skeleton'
import { ToastContainer, ToastType } from '../components/Toast'
import { api } from '../utils/api'

interface DatasetStatus {
  labeled_snapshot_count: number
  required_snapshot_count: number
  snapshot_required: boolean
  label_counts?: Record<string, number>
}

interface Camera {
  id: string
  name: string
  type: string
  enabled: boolean
  dataset_status?: DatasetStatus
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
  created_by?: string
  metadata?: Record<string, any>
}

// Lazy Image Component (Substep 2.2.2.5.8)
function LazyImage({
  screenshot,
  isVisible,
  onVisible,
  onClick,
  cameraNameMap,
}: {
  screenshot: Screenshot
  isVisible: boolean
  onVisible: () => void
  onClick: () => void
  cameraNameMap: Map<string, string>
}) {
  const imgRef = useRef<HTMLImageElement>(null)

  useEffect(() => {
    if (!imgRef.current || isVisible) return

    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            onVisible()
            observer.disconnect()
          }
        })
      },
      { rootMargin: '50px' } // Start loading 50px before visible
    )

    observer.observe(imgRef.current)

    return () => observer.disconnect()
  }, [isVisible, onVisible])

  const cameraName = cameraNameMap.get(screenshot.camera_id) || screenshot.camera_id
  const altText = `Screenshot ${screenshot.id} from camera ${cameraName}, labeled as ${screenshot.label}${screenshot.custom_label ? ` (${screenshot.custom_label})` : ''}`

  return (
    <img
      ref={imgRef}
      src={isVisible ? `/api/screenshots/${screenshot.id}/thumbnail` : undefined}
      data-src={`/api/screenshots/${screenshot.id}/thumbnail`}
      alt={altText}
      className="w-full h-48 object-cover rounded cursor-pointer hover:opacity-90 transition-opacity"
      loading="lazy"
      decoding="async"
      onError={(e) => {
        // Fallback to full image if thumbnail fails
        ; (e.target as HTMLImageElement).src = `/api/screenshots/${screenshot.id}/image`
      }}
      onClick={onClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onClick()
        }
      }}
      aria-label={`View details for screenshot ${screenshot.id}`}
    />
  )
}

export default function Screenshots() {
  const [cameras, setCameras] = useState<Camera[]>([])
  const [snapshotAlerts, setSnapshotAlerts] = useState<Camera[]>([])
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
  const [successMessage, setSuccessMessage] = useState<string | null>(null)
  const [refreshingStatus, setRefreshingStatus] = useState(false)
  const [selectedScreenshot, setSelectedScreenshot] = useState<Screenshot | null>(null)
  const [showDetailModal, setShowDetailModal] = useState(false)
  const [isFullscreen, setIsFullscreen] = useState(false)
  // Edit modal state (Substep 2.2.2.5.3)
  const [showEditModal, setShowEditModal] = useState(false)
  const [editLabel, setEditLabel] = useState<'normal' | 'threat' | 'abnormal' | 'custom'>('normal')
  const [editCustomLabel, setEditCustomLabel] = useState<string>('')
  const [editDescription, setEditDescription] = useState<string>('')
  const [editMetadata, setEditMetadata] = useState<string>('')
  const [editMetadataError, setEditMetadataError] = useState<string | null>(null)
  const [updating, setUpdating] = useState(false)
  // Delete confirmation modal state (Substep 2.2.2.5.4)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [screenshotToDelete, setScreenshotToDelete] = useState<Screenshot | null>(null)
  const [deleting, setDeleting] = useState(false)
  // Enhanced list view state (Substep 2.2.2.5.1)
  const [sortBy, setSortBy] = useState<string>('created_at')
  const [sortOrder, setSortOrder] = useState<string>('desc')
  const [searchDescription, setSearchDescription] = useState<string>('')
  const [currentPage, setCurrentPage] = useState<number>(1)
  const [pageSize, setPageSize] = useState<number>(12)
  const [totalCount, setTotalCount] = useState<number>(0)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [statistics, setStatistics] = useState<{
    total: number
    byLabel: Record<string, number>
    byCamera: Record<string, number>
  } | null>(null)
  // Metadata display state (Substep 2.2.2.5.5)
  const [expandedMetadata, setExpandedMetadata] = useState<Set<string>>(new Set())
  // Bulk operations modal state (Substep 2.2.2.5.6)
  const [showBulkDeleteConfirm, setShowBulkDeleteConfirm] = useState(false)
  const [showBulkLabelModal, setShowBulkLabelModal] = useState(false)
  const [bulkLabel, setBulkLabel] = useState<'normal' | 'threat' | 'abnormal' | 'custom'>('normal')
  const [bulkCustomLabel, setBulkCustomLabel] = useState<string>('')
  const [bulkDeleting, setBulkDeleting] = useState(false)
  const [bulkUpdating, setBulkUpdating] = useState(false)
  // UX improvements state (Substep 2.2.2.5.7)
  const [toasts, setToasts] = useState<Array<{ id: string; message: string; type: ToastType }>>([])
  const [deletedScreenshots, setDeletedScreenshots] = useState<Array<{ screenshot: Screenshot; deletedAt: number }>>([])
  const [failedOperations, setFailedOperations] = useState<Array<{ id: string; operation: () => Promise<void>; error: string }>>([])
  const retryTimeoutRef = useRef<number | null>(null)
  // Performance optimizations state (Substep 2.2.2.5.8)
  const [debouncedSearchDescription, setDebouncedSearchDescription] = useState<string>('')
  const [visibleScreenshots, setVisibleScreenshots] = useState<Set<string>>(new Set())
  const datasetStatusCache = useRef<Map<string, { status: DatasetStatus; timestamp: number }>>(new Map())
  const CACHE_DURATION = 30000 // 30 seconds
  const observerRef = useRef<IntersectionObserver | null>(null)
  const [syncing, setSyncing] = useState(false)

  // Toast notification helper (Substep 2.2.2.5.7)
  const showToast = useCallback((message: string, type: ToastType = 'info') => {
    const id = `toast-${Date.now()}-${Math.random()}`
    setToasts((prev) => [...prev, { id, message, type }])
  }, [])

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  // Keyboard shortcuts (Substep 2.2.2.5.7)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Escape to close modals
      if (e.key === 'Escape') {
        if (showDetailModal) {
          closeDetailModal()
          e.preventDefault()
        } else if (showEditModal) {
          closeEditModal()
          e.preventDefault()
        } else if (showDeleteConfirm) {
          closeDeleteConfirm()
          e.preventDefault()
        } else if (showBulkDeleteConfirm) {
          closeBulkDeleteConfirm()
          e.preventDefault()
        } else if (showBulkLabelModal) {
          closeBulkLabelModal()
          e.preventDefault()
        } else if (showCaptureModal) {
          cancelCapture()
          e.preventDefault()
        }
      }
      // Enter to save (when in modals with forms)
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        if (showEditModal && !updating) {
          saveScreenshotEdit()
          e.preventDefault()
        } else if (showCaptureModal && !saving && capturedImage) {
          saveScreenshot()
          e.preventDefault()
        } else if (showBulkLabelModal && !bulkUpdating) {
          bulkUpdateLabel()
          e.preventDefault()
        }
      }
      // Delete key to delete selected screenshot (when detail modal is open)
      if (e.key === 'Delete' && showDetailModal && selectedScreenshot && !deleting) {
        closeDetailModal()
        openDeleteConfirm(selectedScreenshot)
        e.preventDefault()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [
    showDetailModal,
    showEditModal,
    showDeleteConfirm,
    showBulkDeleteConfirm,
    showBulkLabelModal,
    showCaptureModal,
    selectedScreenshot,
    updating,
    saving,
    bulkUpdating,
    deleting,
  ])

  // Auto-remove deleted screenshots from undo list after 30 seconds (Substep 2.2.2.5.7)
  useEffect(() => {
    const interval = setInterval(() => {
      const now = Date.now()
      setDeletedScreenshots((prev) => prev.filter((item) => now - item.deletedAt < 30000))
    }, 1000)

    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    fetchCameras()
    fetchScreenshots()
  }, [])

  // Debounce search description (Substep 2.2.2.5.8)
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchDescription(searchDescription)
    }, 300) // 300ms debounce

    return () => clearTimeout(timer)
  }, [searchDescription])

  useEffect(() => {
    fetchScreenshots()
  }, [filterLabel, selectedCameraId, sortBy, sortOrder, debouncedSearchDescription, currentPage, pageSize])

  // Get cached dataset status or fetch new one (Substep 2.2.2.5.8)
  const getCachedDatasetStatus = useCallback((cameraId: string): DatasetStatus | null => {
    const cached = datasetStatusCache.current.get(cameraId)
    if (cached && Date.now() - cached.timestamp < CACHE_DURATION) {
      return cached.status
    }
    return null
  }, [])

  const setCachedDatasetStatus = useCallback((cameraId: string, status: DatasetStatus) => {
    datasetStatusCache.current.set(cameraId, { status, timestamp: Date.now() })
  }, [])

  const fetchCameras = async () => {
    try {
      const response = await api.get<{ cameras: Camera[]; count: number }>('/cameras')
      const enabledCameras = response.cameras.filter((cam) => cam.enabled)

      // Use cached dataset status if available (Substep 2.2.2.5.8)
      const camerasWithCachedStatus = enabledCameras.map((cam) => {
        const cachedStatus = getCachedDatasetStatus(cam.id)
        if (cachedStatus) {
          return { ...cam, dataset_status: cachedStatus }
        }
        // Cache new status if available
        if (cam.dataset_status) {
          setCachedDatasetStatus(cam.id, cam.dataset_status)
        }
        return cam
      })

      setCameras(camerasWithCachedStatus)
      const needingSnapshots = camerasWithCachedStatus.filter((cam) => cam.dataset_status?.snapshot_required)
      setSnapshotAlerts(needingSnapshots)
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
      if (debouncedSearchDescription) params.description = debouncedSearchDescription
      if (sortBy) params.sort_by = sortBy
      if (sortOrder) params.sort_order = sortOrder
      params.limit = pageSize.toString()
      params.offset = ((currentPage - 1) * pageSize).toString()

      const queryString = new URLSearchParams(params).toString()
      const response = await api.get<{ screenshots: Screenshot[]; count: number }>(
        `/screenshots${queryString ? `?${queryString}` : ''}`
      )
      setScreenshots(response.screenshots)
      setTotalCount(response.count || response.screenshots.length)

      // Calculate statistics (Substep 2.2.2.5.8)
      const stats = {
        total: response.count || response.screenshots.length,
        byLabel: {} as Record<string, number>,
        byCamera: {} as Record<string, number>,
      }
      response.screenshots.forEach((s) => {
        stats.byLabel[s.label] = (stats.byLabel[s.label] || 0) + 1
        stats.byCamera[s.camera_id] = (stats.byCamera[s.camera_id] || 0) + 1
      })
      setStatistics(stats)
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
    // Validation (Substep 2.2.2.2.3)
    if (!capturedImage || !selectedCameraId) {
      setError('No image captured or camera not selected')
      return
    }

    if (captureLabel === 'custom' && !captureCustomLabel.trim()) {
      setError('Custom label is required when label is "custom"')
      return
    }

    try {
      setSaving(true)
      setError(null)
      setSuccessMessage(null)

      const response = await api.post<{
        id: string
        camera_id: string
        dataset_status?: {
          labeled_snapshot_count: number
          required_snapshot_count: number
          snapshot_required: boolean
        }
      }>('/screenshots', {
        camera_id: selectedCameraId,
        label: captureLabel,
        custom_label: captureLabel === 'custom' ? captureCustomLabel : undefined,
        description: captureDescription || undefined,
        image_data: capturedImage,
      })

      // Get updated snapshot count from response or refresh
      const updatedCount = response.dataset_status?.labeled_snapshot_count
      const requiredCount = response.dataset_status?.required_snapshot_count

      // Show toast notification (Substep 2.2.2.5.7)
      if (updatedCount !== undefined && requiredCount !== undefined) {
        showToast(
          `Screenshot saved! Progress: ${updatedCount}/${requiredCount} normal snapshots collected.`,
          'success'
        )
      } else {
        showToast('Screenshot saved successfully!', 'success')
      }

      // Clear all capture state (Substep 2.2.2.2.1)
      setShowCaptureModal(false)
      setCapturedImage(null)
      setCaptureLabel('normal')
      setCaptureCustomLabel('')
      setCaptureDescription('')

      // Refresh screenshots list
      await fetchScreenshots()

      // Refresh cameras to get updated dataset status (Substep 2.2.2.2.2)
      setRefreshingStatus(true)
      try {
        await fetchCameras()
      } finally {
        setRefreshingStatus(false)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save screenshot')
      // Keep modal open on error so user can retry
    } finally {
      setSaving(false)
    }
  }

  const cancelCapture = () => {
    // Clear all capture state when canceling (Substep 2.2.2.2.1)
    setShowCaptureModal(false)
    setCapturedImage(null)
    setCaptureLabel('normal')
    setCaptureCustomLabel('')
    setCaptureDescription('')
    setError(null)
  }

  const captureAnother = () => {
    // Clear capture state but keep modal open for another capture (Substep 2.2.2.2.3)
    setCapturedImage(null)
    setCaptureLabel('normal')
    setCaptureCustomLabel('')
    setCaptureDescription('')
    setError(null)
    setSuccessMessage(null)
    // Modal will stay open, user can capture again
  }

  // Enhanced delete functions (Substep 2.2.2.5.4)
  const openDeleteConfirm = (screenshot: Screenshot) => {
    setScreenshotToDelete(screenshot)
    setShowDeleteConfirm(true)
  }

  const closeDeleteConfirm = () => {
    setShowDeleteConfirm(false)
    setScreenshotToDelete(null)
    setDeleting(false)
  }

  const deleteScreenshot = async (screenshot: Screenshot) => {
    if (!screenshot) return

    try {
      setDeleting(true)
      setError(null)

      await api.delete(`/screenshots/${screenshot.id}`)

      // Add to undo list (Substep 2.2.2.5.7)
      setDeletedScreenshots((prev) => [...prev, { screenshot, deletedAt: Date.now() }])

      // Show toast with undo option (Substep 2.2.2.5.7)
      showToast('Screenshot deleted successfully', 'success')

      // Refresh screenshots list
      await fetchScreenshots()

      // Refresh cameras to update dataset status (Substep 2.2.2.5.4)
      setRefreshingStatus(true)
      try {
        await fetchCameras()
      } finally {
        setRefreshingStatus(false)
      }

      // Close detail modal if it's open for this screenshot
      if (showDetailModal && selectedScreenshot?.id === screenshot.id) {
        closeDetailModal()
      }

      // Close delete confirmation
      closeDeleteConfirm()
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to delete screenshot'
      setError(errorMessage)
      showToast(errorMessage, 'error')
      // Add to retry list (Substep 2.2.2.5.7)
      setFailedOperations((prev) => [
        ...prev,
        {
          id: `delete-${screenshot.id}`,
          operation: () => deleteScreenshot(screenshot),
          error: errorMessage,
        },
      ])
      setDeleting(false)
    }
  }

  // Undo delete function (Substep 2.2.2.5.7)
  const undoDelete = async (deletedItem: { screenshot: Screenshot; deletedAt: number }) => {
    try {
      // Recreate the screenshot (this would require backend support for undo)
      // For now, we'll show a message that undo is not fully supported
      showToast('Undo functionality requires backend support. Screenshot cannot be restored.', 'warning')
      setDeletedScreenshots((prev) => prev.filter((item) => item !== deletedItem))
    } catch (err) {
      showToast('Failed to undo delete', 'error')
    }
  }

  // Retry failed operation (Substep 2.2.2.5.7)
  const retryOperation = async (operationId: string) => {
    const operation = failedOperations.find((op) => op.id === operationId)
    if (!operation) return

    try {
      await operation.operation()
      setFailedOperations((prev) => prev.filter((op) => op.id !== operationId))
      showToast('Operation retried successfully', 'success')
    } catch (err) {
      showToast('Retry failed. Please try again.', 'error')
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

  // Edit modal functions (Substep 2.2.2.5.3)
  const openEditModal = (screenshot: Screenshot) => {
    setSelectedScreenshot(screenshot)
    setEditLabel(screenshot.label)
    setEditCustomLabel(screenshot.custom_label || '')
    setEditDescription(screenshot.description || '')
    setEditMetadata(screenshot.metadata ? JSON.stringify(screenshot.metadata, null, 2) : '')
    setEditMetadataError(null)
    setShowEditModal(true)
  }

  const closeEditModal = () => {
    setShowEditModal(false)
    setEditLabel('normal')
    setEditCustomLabel('')
    setEditDescription('')
    setEditMetadata('')
    setEditMetadataError(null)
    setUpdating(false)
  }

  const validateEditForm = (): boolean => {
    if (editLabel === 'custom' && !editCustomLabel.trim()) {
      setError('Custom label is required when label is "custom"')
      return false
    }

    // Validate metadata JSON if provided
    if (editMetadata.trim()) {
      try {
        JSON.parse(editMetadata)
        setEditMetadataError(null)
      } catch (err) {
        setEditMetadataError('Invalid JSON format')
        return false
      }
    }

    return true
  }

  const saveScreenshotEdit = async () => {
    if (!selectedScreenshot) return

    if (!validateEditForm()) {
      return
    }

    try {
      setUpdating(true)
      setError(null)
      setSuccessMessage(null)

      const updateData: any = {
        label: editLabel,
        custom_label: editLabel === 'custom' ? editCustomLabel : undefined,
        description: editDescription || undefined,
      }

      // Parse and include metadata if provided
      // If metadata is empty, we don't send it (to keep existing metadata)
      // If metadata is provided, parse and send it
      if (editMetadata.trim()) {
        try {
          const parsedMetadata = JSON.parse(editMetadata)
          // Only send metadata if it's a valid object
          if (typeof parsedMetadata === 'object' && parsedMetadata !== null) {
            updateData.metadata = parsedMetadata
          } else {
            setError('Metadata must be a JSON object')
            setUpdating(false)
            return
          }
        } catch (err) {
          setError('Invalid metadata JSON format')
          setUpdating(false)
          return
        }
      }
      // Note: If editMetadata is empty, we don't include it in updateData
      // This means the backend will keep the existing metadata unchanged

      await api.put(`/screenshots/${selectedScreenshot.id}`, updateData)

      showToast('Screenshot updated successfully!', 'success')

      // Refresh screenshots list
      await fetchScreenshots()

      // Refresh detail modal if it's open
      if (showDetailModal) {
        await fetchScreenshotDetails(selectedScreenshot.id)
      }

      closeEditModal()
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to update screenshot'
      setError(errorMessage)
      showToast(errorMessage, 'error')
      // Add to retry list (Substep 2.2.2.5.7)
      if (selectedScreenshot) {
        setFailedOperations((prev) => [
          ...prev,
          {
            id: `update-${selectedScreenshot.id}`,
            operation: () => saveScreenshotEdit(),
            error: errorMessage,
          },
        ])
      }
    } finally {
      setUpdating(false)
    }
  }

  const fetchScreenshotDetails = async (id: string) => {
    try {
      const response = await api.get<Screenshot>(`/screenshots/${id}`)
      setSelectedScreenshot(response)
      setShowDetailModal(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load screenshot details')
    }
  }

  const openDetailModal = (screenshot: Screenshot) => {
    // Fetch full details to ensure we have metadata
    fetchScreenshotDetails(screenshot.id)
  }

  const closeDetailModal = () => {
    setShowDetailModal(false)
    setSelectedScreenshot(null)
    setIsFullscreen(false)
  }

  const toggleFullscreen = () => {
    setIsFullscreen(!isFullscreen)
  }

  // Bulk selection functions (Substep 2.2.2.5.1)
  const toggleSelectScreenshot = (id: string) => {
    const newSelected = new Set(selectedIds)
    if (newSelected.has(id)) {
      newSelected.delete(id)
    } else {
      newSelected.add(id)
    }
    setSelectedIds(newSelected)
  }

  const toggleSelectAll = () => {
    if (selectedIds.size === screenshots.length) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(screenshots.map((s) => s.id)))
    }
  }

  // Bulk operations functions (Substep 2.2.2.5.6)
  const openBulkDeleteConfirm = () => {
    if (selectedIds.size === 0) return
    setShowBulkDeleteConfirm(true)
  }

  const closeBulkDeleteConfirm = () => {
    setShowBulkDeleteConfirm(false)
    setBulkDeleting(false)
  }

  const bulkDelete = async () => {
    if (selectedIds.size === 0) return

    try {
      setBulkDeleting(true)
      setError(null)
      setSuccessMessage(null)

      const deletePromises = Array.from(selectedIds).map((id) => api.delete(`/screenshots/${id}`))
      await Promise.all(deletePromises)

      const count = selectedIds.size
      setSelectedIds(new Set())
      setShowBulkDeleteConfirm(false)

      showToast(`Successfully deleted ${count} screenshot${count !== 1 ? 's' : ''}!`, 'success')

      // Refresh screenshots list
      await fetchScreenshots()

      // Refresh cameras to update dataset status
      setRefreshingStatus(true)
      try {
        await fetchCameras()
      } finally {
        setRefreshingStatus(false)
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to delete screenshots'
      setError(errorMessage)
      showToast(errorMessage, 'error')
      // Add to retry list (Substep 2.2.2.5.7)
      setFailedOperations((prev) => [
        ...prev,
        {
          id: `bulk-delete-${Date.now()}`,
          operation: () => bulkDelete(),
          error: errorMessage,
        },
      ])
    } finally {
      setBulkDeleting(false)
    }
  }

  const openBulkLabelModal = () => {
    if (selectedIds.size === 0) return
    setBulkLabel('normal')
    setBulkCustomLabel('')
    setShowBulkLabelModal(true)
  }

  const closeBulkLabelModal = () => {
    setShowBulkLabelModal(false)
    setBulkLabel('normal')
    setBulkCustomLabel('')
    setBulkUpdating(false)
  }

  const bulkUpdateLabel = async () => {
    if (selectedIds.size === 0) return

    // Validation
    if (bulkLabel === 'custom' && !bulkCustomLabel.trim()) {
      setError('Custom label is required when label is "custom"')
      return
    }

    try {
      setBulkUpdating(true)
      setError(null)
      setSuccessMessage(null)

      const updatePromises = Array.from(selectedIds).map((id) =>
        api.put(`/screenshots/${id}`, {
          label: bulkLabel,
          custom_label: bulkLabel === 'custom' ? bulkCustomLabel : undefined,
        })
      )
      await Promise.all(updatePromises)

      const count = selectedIds.size
      setSelectedIds(new Set())
      setShowBulkLabelModal(false)

      showToast(`Successfully updated label for ${count} screenshot${count !== 1 ? 's' : ''}!`, 'success')

      // Refresh screenshots list
      await fetchScreenshots()

      // Refresh cameras to update dataset status
      setRefreshingStatus(true)
      try {
        await fetchCameras()
      } finally {
        setRefreshingStatus(false)
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to update screenshots'
      setError(errorMessage)
      showToast(errorMessage, 'error')
      // Add to retry list (Substep 2.2.2.5.7)
      setFailedOperations((prev) => [
        ...prev,
        {
          id: `bulk-update-${Date.now()}`,
          operation: () => bulkUpdateLabel(),
          error: errorMessage,
        },
      ])
    } finally {
      setBulkUpdating(false)
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

  // Memoized camera name lookup (Substep 2.2.2.5.8)
  const cameraNameMap = useMemo(() => {
    const map = new Map<string, string>()
    cameras.forEach((cam) => map.set(cam.id, cam.name))
    return map
  }, [cameras])

  // Loading skeletons (Substep 2.2.2.5.7)
  if (loading && screenshots.length === 0) {
    return (
      <div
        className="space-y-6"
        role="status"
        aria-label="Loading screenshots"
        data-testid="screenshots-loading-skeleton"
      >
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(6)].map((_, i) => (
            <ScreenshotSkeleton key={i} />
          ))}
        </div>
      </div>
    )
  }

  const acknowledgeReminder = async (cameraId: string) => {
    try {
      // Send telemetry about reminder acknowledgment
      await api.post('/telemetry/reminder', {
        camera_id: cameraId,
        action: 'acknowledged',
        timestamp: new Date().toISOString(),
      })
    } catch (err) {
      // Silently fail - telemetry is not critical
      console.warn('Failed to send reminder telemetry', err)
    }
  }

  const completeReminder = async (cameraId: string) => {
    try {
      // Send telemetry about reminder completion
      await api.post('/telemetry/reminder', {
        camera_id: cameraId,
        action: 'completed',
        timestamp: new Date().toISOString(),
      })
    } catch (err) {
      // Silently fail - telemetry is not critical
      console.warn('Failed to send reminder telemetry', err)
    }
  }

  return (
    <div className="space-y-6">
      {/* Toast notifications (Substep 2.2.2.5.7) */}
      <ToastContainer toasts={toasts} onClose={removeToast} />

      {/* Undo notifications (Substep 2.2.2.5.7) */}
      {deletedScreenshots.length > 0 && (
        <div className="bg-blue-50 border border-blue-200 rounded-lg p-4" role="alert" aria-live="polite">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Info className="w-5 h-5 text-blue-600" />
              <p className="text-sm text-blue-800">
                {deletedScreenshots.length} screenshot{deletedScreenshots.length !== 1 ? 's' : ''} deleted
                {deletedScreenshots.length > 0 && (
                  <span className="ml-2 text-xs">
                    (Undo available for {Math.max(0, 30 - Math.floor((Date.now() - deletedScreenshots[0].deletedAt) / 1000))}s)
                  </span>
                )}
              </p>
            </div>
            <div className="flex items-center gap-2">
              {deletedScreenshots.map((item) => (
                <Button
                  key={item.screenshot.id}
                  size="sm"
                  variant="secondary"
                  onClick={() => undoDelete(item)}
                  aria-label={`Undo delete of screenshot ${item.screenshot.id}`}
                >
                  <RotateCcw className="w-3 h-3 mr-1" />
                  Undo
                </Button>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Retry failed operations (Substep 2.2.2.5.7) */}
      {failedOperations.length > 0 && (
        <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4" role="alert" aria-live="polite">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Info className="w-5 h-5 text-yellow-600" />
              <p className="text-sm text-yellow-800">
                {failedOperations.length} operation{failedOperations.length !== 1 ? 's' : ''} failed
              </p>
            </div>
            <div className="flex items-center gap-2">
              {failedOperations.map((op) => (
                <Button
                  key={op.id}
                  size="sm"
                  variant="secondary"
                  onClick={() => retryOperation(op.id)}
                  aria-label={`Retry failed operation: ${op.error}`}
                >
                  <RefreshCw className="w-3 h-3 mr-1" />
                  Retry
                </Button>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Show error message at top of page */}
      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <p className="text-sm text-red-800">{error}</p>
            <button
              onClick={() => setError(null)}
              className="text-red-600 hover:text-red-800"
            >
              ×
            </button>
          </div>
        </div>
      )}

      {/* Show loading indicator while refreshing status (Substep 2.2.2.2.2) */}
      {refreshingStatus && (
        <div className="bg-blue-50 border border-blue-200 rounded-lg p-3">
          <p className="text-sm text-blue-800 flex items-center">
            <RefreshCw className="h-4 w-4 mr-2 animate-spin" />
            Refreshing dataset status...
          </p>
        </div>
      )}

      {snapshotAlerts.length > 0 && (
        <div className="rounded-lg border border-yellow-300 bg-yellow-50 p-4">
          <div className="flex items-start justify-between">
            <div className="flex-1">
              <div className="flex items-center gap-2 mb-2">
                <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold bg-yellow-200 text-yellow-800">
                  ⚠️ Action Required
                </span>
                <h3 className="text-sm font-semibold text-yellow-900">
                  {snapshotAlerts.length === 1
                    ? `Camera "${snapshotAlerts[0].name}" needs more labeled snapshots`
                    : `${snapshotAlerts.length} cameras need more labeled snapshots`}
                </h3>
              </div>
              <div className="space-y-2 mb-3">
                {snapshotAlerts.map((cam) => {
                  // Fix progress calculation - handle division by zero (Substep 2.2.2.3.1)
                  const progress = cam.dataset_status && cam.dataset_status.required_snapshot_count > 0
                    ? Math.min(100, Math.max(0, (cam.dataset_status.labeled_snapshot_count / cam.dataset_status.required_snapshot_count) * 100))
                    : 0
                  const remaining = cam.dataset_status
                    ? Math.max(0, cam.dataset_status.required_snapshot_count - cam.dataset_status.labeled_snapshot_count)
                    : 0

                  return (
                    <div
                      key={cam.id}
                      className="bg-white rounded-md p-3 border border-yellow-200"
                    >
                      <div className="flex items-center justify-between mb-2">
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-sm text-gray-900">{cam.name}</span>
                          <span className="text-xs text-gray-600">
                            {cam.dataset_status?.labeled_snapshot_count || 0}/
                            {cam.dataset_status?.required_snapshot_count || 0} normal snapshots
                          </span>
                        </div>
                        <span className="text-xs font-medium text-yellow-700">
                          {remaining} more needed
                        </span>
                      </div>
                      <div className="w-full bg-gray-200 rounded-full h-2 mb-2">
                        <div
                          className="bg-yellow-500 h-2 rounded-full transition-all"
                          style={{ width: `${Math.min(progress, 100)}%` }}
                        />
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          size="sm"
                          onClick={() => {
                            setSelectedCameraId(cam.id)
                            setCaptureLabel('normal')
                            captureScreenshot()
                            completeReminder(cam.id)
                          }}
                        >
                          <Camera className="w-3 h-3 mr-1" />
                          Capture Now
                        </Button>
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => acknowledgeReminder(cam.id)}
                        >
                          Dismiss
                        </Button>
                      </div>
                    </div>
                  )
                })}
              </div>
              <p className="text-xs text-yellow-800">
                💡 Capture labeled <span className="font-semibold">normal</span> snapshots to enable training for these cameras.
              </p>
            </div>
          </div>
        </div>
      )}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Screenshot Management</h1>
          <p className="mt-2 text-gray-600">
            Capture, label, and manage screenshots for model training
          </p>
        </div>
      </div>

      {error && (
        <div
          className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded"
          data-testid="screenshot-error-banner"
        >
          {error}
        </div>
      )}

      {/* Capture Section */}
      <Card>
        <div className="space-y-4">
          <div className="flex flex-wrap items-end gap-4">
            <Select
              label="Camera"
              value={selectedCameraId}
              onChange={(e) => setSelectedCameraId(e.target.value)}
              options={cameras.map((cam) => ({ value: cam.id, label: cam.name }))}
            />
            <Button
              onClick={captureScreenshot}
              disabled={!selectedCameraId || saving}
              data-testid="capture-screenshot-button"
            >
              <Camera className="w-4 h-4 mr-2" />
              {saving ? 'Capturing...' : 'Capture Screenshot'}
            </Button>
            <Button variant="secondary" onClick={exportDataset} disabled={exporting}>
              {exporting ? 'Exporting...' : 'Export Dataset'}
            </Button>
          </div>
          {/* Camera Dataset Progress Display */}
          {selectedCameraId && (
            <div className="mt-4 pt-4 border-t border-gray-200">
              <h3 className="text-sm font-semibold text-gray-900 mb-3">Dataset Progress</h3>
              {(() => {
                const selectedCam = cameras.find((c) => c.id === selectedCameraId)
                const status = selectedCam?.dataset_status

                // Handle null/undefined dataset_status gracefully (Substep 2.2.2.3.1)
                if (!status) {
                  return (
                    <div className="text-sm text-gray-500 italic">
                      Calculating dataset status...
                    </div>
                  )
                }

                // Fix progress calculation - handle division by zero (Substep 2.2.2.3.1)
                const progress = status.required_snapshot_count > 0
                  ? Math.min(100, Math.max(0, (status.labeled_snapshot_count / status.required_snapshot_count) * 100))
                  : 0
                const remaining = Math.max(0, status.required_snapshot_count - status.labeled_snapshot_count)
                const isComplete = !status.snapshot_required

                return (
                  <div className="space-y-3">
                    <div>
                      <div className="flex items-center justify-between mb-1">
                        <span className="text-sm text-gray-700">Normal Snapshots</span>
                        <span className="text-sm font-medium text-gray-900">
                          {status.labeled_snapshot_count} / {status.required_snapshot_count}
                        </span>
                      </div>
                      <div className="w-full bg-gray-200 rounded-full h-3">
                        <div
                          className={`h-3 rounded-full transition-all duration-500 ease-out ${isComplete ? 'bg-green-500' : 'bg-blue-500'
                            }`}
                          style={{ width: `${Math.min(progress, 100)}%` }}
                        />
                      </div>
                      {!isComplete && (
                        <p className="text-xs text-gray-600 mt-1">
                          {remaining} more normal snapshots needed for training
                        </p>
                      )}
                      {isComplete && (
                        <p className="text-xs text-green-600 mt-1 font-medium">
                          ✓ Ready for training {syncing && <span className="text-blue-600">(Syncing...)</span>}
                        </p>
                      )}
                    </div>
                    {/* Manual dataset sync button (Substep 2.2.2.8.3 / 2.2.2.9.3) */}
                    <div className="mt-3 pt-3 border-t border-gray-200 flex items-center justify-between gap-3">
                      <p className="text-xs text-gray-600">
                        When the dataset is ready, sync its status to the training service.
                      </p>
                      <Button
                        size="sm"
                        data-testid="sync-dataset-status-button"
                        onClick={async () => {
                          if (!selectedCameraId || !isComplete || syncing) return
                          try {
                            setSyncing(true)
                            setError(null)
                            showToast('Syncing dataset to VM...', 'info')
                            
                            const response = await api.post<{
                              camera_id: string
                              dataset_synced: boolean
                              dataset_id?: string
                              message?: string
                            }>(`/cameras/${selectedCameraId}/dataset/sync`)
                            
                            if (response.dataset_synced) {
                              showToast(
                                response.dataset_id
                                  ? `Dataset synced successfully! Dataset ID: ${response.dataset_id}`
                                  : 'Dataset synced successfully to VM',
                                'success'
                              )
                              // Refresh cameras to get updated status
                              setRefreshingStatus(true)
                              try {
                                await fetchCameras()
                              } finally {
                                setRefreshingStatus(false)
                              }
                            } else {
                              throw new Error('Sync completed but dataset_synced is false')
                            }
                          } catch (err) {
                            const message =
                              err instanceof Error ? err.message : 'Failed to sync dataset status'
                            setError(message)
                            
                            // Show appropriate error message based on error type
                            if (message.includes('Connection') || message.includes('network') || message.includes('unavailable')) {
                              showToast('Connection unavailable. Please check WireGuard tunnel.', 'error')
                            } else if (message.includes('409') || message.includes('Conflict')) {
                              showToast('Dataset not ready for sync. More snapshots needed.', 'warning')
                            } else {
                              showToast(message, 'error')
                            }
                          } finally {
                            setSyncing(false)
                          }
                        }}
                        disabled={!isComplete || syncing}
                        aria-label={syncing ? 'Syncing dataset...' : 'Sync dataset status'}
                      >
                        <RefreshCw className={`w-3 h-3 mr-1 ${syncing ? 'animate-spin' : ''}`} />
                        {syncing ? 'Uploading dataset...' : 'Sync Dataset Status'}
                      </Button>
                    </div>

                    {/* Snapshot count by label display (Substep 2.2.2.3.2) */}
                    {status.label_counts && Object.keys(status.label_counts).length > 0 && (
                      <div className="mt-3 pt-3 border-t border-gray-200">
                        <div className="text-xs font-medium text-gray-700 mb-2">Snapshot Counts by Label:</div>
                        <div className="flex flex-wrap gap-2">
                          {Object.entries(status.label_counts).map(([label, count]) => {
                            const getLabelColor = (l: string) => {
                              switch (l.toLowerCase()) {
                                case 'normal':
                                  return 'bg-green-100 text-green-800 border-green-200'
                                case 'threat':
                                  return 'bg-red-100 text-red-800 border-red-200'
                                case 'abnormal':
                                  return 'bg-yellow-100 text-yellow-800 border-yellow-200'
                                default:
                                  return 'bg-gray-100 text-gray-800 border-gray-200'
                              }
                            }
                            const getLabelIcon = (l: string) => {
                              switch (l.toLowerCase()) {
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
                            return (
                              <div
                                key={label}
                                className={`px-2 py-1 rounded-md border text-xs font-medium ${getLabelColor(label)}`}
                              >
                                <span className="mr-1">{getLabelIcon(label)}</span>
                                {label}: {count}
                              </div>
                            )
                          })}
                        </div>
                      </div>
                    )}

                    <div className="grid grid-cols-2 gap-4 text-xs mt-3">
                      <div>
                        <span className="text-gray-600">Label Coverage:</span>
                        <span className="ml-2 font-medium text-gray-900">
                          {Object.keys(status.label_counts || {}).length} labels
                        </span>
                      </div>
                      <div>
                        <span className="text-gray-600">Status:</span>
                        <span
                          className={`ml-2 font-medium ${isComplete ? 'text-green-600' : 'text-yellow-600'
                            }`}
                        >
                          {isComplete ? 'Ready' : 'In Progress'}
                        </span>
                      </div>
                    </div>
                  </div>
                )
              })()}
            </div>
          )}
        </div>
      </Card>

      {/* Statistics Display (Substep 2.2.2.5.1) */}
      {statistics && (
        <Card>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <h3 className="text-sm font-semibold text-gray-700 mb-2">Total Screenshots</h3>
              <p className="text-2xl font-bold text-gray-900">{statistics.total}</p>
            </div>
            <div>
              <h3 className="text-sm font-semibold text-gray-700 mb-2">By Label</h3>
              <div className="flex flex-wrap gap-2">
                {Object.entries(statistics.byLabel).map(([label, count]) => (
                  <span
                    key={label}
                    className={`px-2 py-1 rounded text-xs font-medium ${getLabelColor(label)}`}
                  >
                    {getLabelIcon(label)} {label}: {count}
                  </span>
                ))}
              </div>
            </div>
            <div>
              <h3 className="text-sm font-semibold text-gray-700 mb-2">By Camera</h3>
              <div className="space-y-1">
                {Object.entries(statistics.byCamera).map(([cameraId, count]) => (
                  <p key={cameraId} className="text-sm text-gray-600">
                    {cameras.find((c) => c.id === cameraId)?.name || cameraId}: {count}
                  </p>
                ))}
              </div>
            </div>
          </div>
        </Card>
      )}

      {/* Filters and Controls (Substep 2.2.2.5.1) */}
      <Card>
        <div className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <Select
              label="Filter by Label"
              value={filterLabel}
              onChange={(e) => {
                setFilterLabel(e.target.value)
                setCurrentPage(1)
              }}
              options={[
                { value: '', label: 'All Labels' },
                { value: 'normal', label: 'Normal' },
                { value: 'threat', label: 'Threat' },
                { value: 'abnormal', label: 'Abnormal' },
                { value: 'custom', label: 'Custom' },
              ]}
            />
            <Input
              label="Search Description"
              value={searchDescription}
              onChange={(e) => {
                setSearchDescription(e.target.value)
                setCurrentPage(1)
              }}
              placeholder="Search in descriptions..."
            />
            <Select
              label="Sort By"
              value={sortBy}
              onChange={(e) => {
                setSortBy(e.target.value)
                setCurrentPage(1)
              }}
              options={[
                { value: 'created_at', label: 'Date Created' },
                { value: 'updated_at', label: 'Date Updated' },
                { value: 'camera_id', label: 'Camera' },
                { value: 'label', label: 'Label' },
                { value: 'custom_label', label: 'Custom Label' },
              ]}
            />
            <Select
              label="Sort Order"
              value={sortOrder}
              onChange={(e) => {
                setSortOrder(e.target.value)
                setCurrentPage(1)
              }}
              options={[
                { value: 'desc', label: 'Descending' },
                { value: 'asc', label: 'Ascending' },
              ]}
            />
          </div>

          {/* Bulk Operations (Substep 2.2.2.5.6) */}
          {selectedIds.size > 0 && (
            <div className="flex items-center gap-3 pt-3 border-t border-gray-200">
              <span className="text-sm font-medium text-gray-700">
                {selectedIds.size} screenshot{selectedIds.size !== 1 ? 's' : ''} selected
              </span>
              <Button
                size="sm"
                variant="secondary"
                onClick={openBulkLabelModal}
              >
                <Tag className="w-3 h-3 mr-1" />
                Change Label
              </Button>
              <Button
                size="sm"
                variant="secondary"
                onClick={openBulkDeleteConfirm}
              >
                <Trash2 className="w-3 h-3 mr-1" />
                Delete Selected
              </Button>
              <Button
                size="sm"
                variant="secondary"
                onClick={() => setSelectedIds(new Set())}
              >
                Clear Selection
              </Button>
            </div>
          )}
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
        <>
          {/* Select All Checkbox (Substep 2.2.2.5.1) */}
          <div className="flex items-center gap-2 mb-2">
            <button
              onClick={toggleSelectAll}
              className="flex items-center gap-2 text-sm text-gray-700 hover:text-gray-900"
              aria-label={selectedIds.size === screenshots.length ? 'Deselect all screenshots' : 'Select all screenshots'}
            >
              {selectedIds.size === screenshots.length ? (
                <CheckSquare className="w-5 h-5" aria-hidden="true" />
              ) : (
                <Square className="w-5 h-5" aria-hidden="true" />
              )}
              <span>Select All</span>
            </button>
            {selectedIds.size > 0 && (
              <span className="text-sm text-gray-600" aria-live="polite" aria-atomic="true">
                ({selectedIds.size} selected)
              </span>
            )}
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {screenshots.map((screenshot) => (
              <Card key={screenshot.id} dataTestId="screenshot-card">
                <div className="space-y-3">
                  <div className="relative">
                    {/* Bulk Selection Checkbox (Substep 2.2.2.5.1) */}
                    <div className="absolute top-2 left-2 z-10">
                      <button
                        onClick={() => toggleSelectScreenshot(screenshot.id)}
                        className="bg-white rounded p-1 shadow-sm hover:bg-gray-50"
                        aria-label={selectedIds.has(screenshot.id) ? `Deselect screenshot ${screenshot.id}` : `Select screenshot ${screenshot.id}`}
                      >
                        {selectedIds.has(screenshot.id) ? (
                          <CheckSquare className="w-4 h-4 text-blue-600" aria-hidden="true" />
                        ) : (
                          <Square className="w-4 h-4 text-gray-400" aria-hidden="true" />
                        )}
                      </button>
                    </div>
                    {/* Thumbnail with lazy loading (Substep 2.2.2.5.8) */}
                    <LazyImage
                      screenshot={screenshot}
                      isVisible={visibleScreenshots.has(screenshot.id)}
                      onVisible={() => {
                        setVisibleScreenshots((prev) => new Set([...prev, screenshot.id]))
                      }}
                      onClick={() => openDetailModal(screenshot)}
                      cameraNameMap={cameraNameMap}
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
                    {/* Enhanced metadata display (Substep 2.2.2.5.5) */}
                    <div className="space-y-1">
                      <p className="text-sm text-gray-600">
                        <span className="font-medium">Camera:</span>{' '}
                        {cameraNameMap.get(screenshot.camera_id) || screenshot.camera_id}
                      </p>
                      {screenshot.custom_label && (
                        <p className="text-sm text-gray-600">
                          <span className="font-medium">Custom Label:</span> {screenshot.custom_label}
                        </p>
                      )}
                      {screenshot.description && (
                        <p className="text-sm text-gray-500 mt-1 line-clamp-2">{screenshot.description}</p>
                      )}
                      {/* File size and dimensions from metadata (Substep 2.2.2.5.5) */}
                      {screenshot.metadata && (
                        <div className="flex flex-wrap gap-2 mt-2">
                          {screenshot.metadata.width && screenshot.metadata.height && (
                            <span className="inline-flex items-center gap-1 px-2 py-0.5 bg-blue-50 text-blue-700 rounded text-xs">
                              <Info className="w-3 h-3" />
                              {screenshot.metadata.width}×{screenshot.metadata.height}
                            </span>
                          )}
                          {screenshot.metadata.processed_size_bytes && (
                            <span className="inline-flex items-center gap-1 px-2 py-0.5 bg-gray-50 text-gray-700 rounded text-xs">
                              <Info className="w-3 h-3" />
                              {((screenshot.metadata.processed_size_bytes as number) / 1024).toFixed(1)} KB
                            </span>
                          )}
                          {screenshot.metadata.original_format && (
                            <span className="inline-flex items-center gap-1 px-2 py-0.5 bg-purple-50 text-purple-700 rounded text-xs">
                              <Tag className="w-3 h-3" />
                              {screenshot.metadata.original_format.toUpperCase()}
                            </span>
                          )}
                        </div>
                      )}
                      {/* Dates and created_by (Substep 2.2.2.5.5) */}
                      <div className="mt-2 space-y-0.5">
                        <p className="text-xs text-gray-400">
                          Created: {new Date(screenshot.created_at).toLocaleString()}
                        </p>
                        {screenshot.updated_at !== screenshot.created_at && (
                          <p className="text-xs text-gray-400">
                            Updated: {new Date(screenshot.updated_at).toLocaleString()}
                          </p>
                        )}
                        {screenshot.created_by && (
                          <p className="text-xs text-gray-500">
                            By: {screenshot.created_by}
                          </p>
                        )}
                      </div>
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => openDetailModal(screenshot)}
                    >
                      <Eye className="w-3 h-3 mr-1" />
                      View Details
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => openEditModal(screenshot)}
                    >
                      <Edit className="w-3 h-3 mr-1" />
                      Edit
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => openDeleteConfirm(screenshot)}
                    >
                      <Trash2 className="w-3 h-3 mr-1" />
                      Delete
                    </Button>
                  </div>
                </div>
              </Card>
            ))}
          </div>

          {/* Pagination Controls (Substep 2.2.2.5.1) */}
          {totalCount > pageSize && (
            <Card>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="text-sm text-gray-700">
                    Showing {((currentPage - 1) * pageSize) + 1} to {Math.min(currentPage * pageSize, totalCount)} of {totalCount} screenshots
                  </span>
                  <Select
                    value={pageSize.toString()}
                    onChange={(e) => {
                      setPageSize(Number(e.target.value))
                      setCurrentPage(1)
                    }}
                    options={[
                      { value: '6', label: '6 per page' },
                      { value: '12', label: '12 per page' },
                      { value: '24', label: '24 per page' },
                      { value: '48', label: '48 per page' },
                    ]}
                  />
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                    disabled={currentPage === 1}
                  >
                    <ChevronLeft className="w-4 h-4 mr-1" />
                    Previous
                  </Button>
                  <span className="text-sm text-gray-700">
                    Page {currentPage} of {Math.ceil(totalCount / pageSize)}
                  </span>
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => setCurrentPage((p) => Math.min(Math.ceil(totalCount / pageSize), p + 1))}
                    disabled={currentPage >= Math.ceil(totalCount / pageSize)}
                  >
                    Next
                    <ChevronRight className="w-4 h-4 ml-1" />
                  </Button>
                </div>
              </div>
            </Card>
          )}
        </>
      )}

      {/* Capture Modal */}
      {showCaptureModal && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"
          role="dialog"
          aria-modal="true"
          aria-labelledby="capture-modal-title"
        >
          <div className="bg-white rounded-lg p-6 max-w-2xl w-full mx-4 max-h-[90vh] overflow-y-auto">
            <h2 id="capture-modal-title" className="text-xl font-bold mb-4">Label Screenshot</h2>
            <div className="space-y-4">
              {/* Show preview of captured image (Substep 2.2.2.2.3) */}
              {capturedImage ? (
                <div>
                  <img src={capturedImage} alt="Captured" className="w-full rounded" />
                </div>
              ) : (
                <div className="border-2 border-dashed border-gray-300 rounded-lg p-8 text-center">
                  <p className="text-gray-500">No image captured yet. Click "Capture Screenshot" to take a snapshot.</p>
                </div>
              )}

              {/* Show error message if save failed (Substep 2.2.2.2.3) */}
              {error && (
                <div className="bg-red-50 border border-red-200 rounded-lg p-3">
                  <p className="text-sm text-red-800">{error}</p>
                </div>
              )}

              {/* Show success message after save (Substep 2.2.2.2.2) */}
              {successMessage && (
                <div className="bg-green-50 border border-green-200 rounded-lg p-3">
                  <p className="text-sm text-green-800">{successMessage}</p>
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
                      Cancel
                    </Button>
                    {successMessage ? (
                      <>
                        <Button variant="secondary" onClick={captureAnother}>
                          Capture Another
                        </Button>
                        <Button onClick={cancelCapture}>Close</Button>
                      </>
                    ) : (
                      <Button onClick={saveScreenshot} disabled={saving || (captureLabel === 'custom' && !captureCustomLabel.trim())}>
                        <Save className="w-4 h-4 mr-2" />
                        {saving ? 'Saving...' : 'Save Screenshot'}
                      </Button>
                    )}
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

      {/* Screenshot Detail Modal */}
      {showDetailModal && selectedScreenshot && (
        <div
          className={`fixed inset-0 bg-black ${isFullscreen ? 'bg-opacity-100' : 'bg-opacity-50'} flex items-center justify-center z-50`}
          onClick={isFullscreen ? undefined : closeDetailModal}
          role="dialog"
          aria-modal="true"
          aria-labelledby="detail-modal-title"
        >
          <div
            className={`bg-white rounded-lg ${isFullscreen ? 'w-full h-full rounded-none' : 'max-w-6xl w-full mx-4'} max-h-[95vh] overflow-y-auto`}
            onClick={(e) => e.stopPropagation()}
          >
            {/* Modal Header */}
            <div className="sticky top-0 bg-white border-b border-gray-200 px-6 py-4 flex items-center justify-between z-10">
              <h2 id="detail-modal-title" className="text-2xl font-bold text-gray-900">Screenshot Details</h2>
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={toggleFullscreen}
                  title={isFullscreen ? 'Exit Fullscreen' : 'Enter Fullscreen'}
                >
                  <Maximize2 className="w-4 h-4" />
                </Button>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => {
                    if (selectedScreenshot) {
                      closeDetailModal()
                      openEditModal(selectedScreenshot)
                    }
                  }}
                >
                  <Edit className="w-4 h-4 mr-1" />
                  Edit
                </Button>
                <Button size="sm" variant="secondary" onClick={closeDetailModal}>
                  <X className="w-4 h-4" />
                </Button>
              </div>
            </div>

            {/* Modal Content */}
            <div className="p-6 space-y-6">
              {/* Full-size Screenshot Image */}
              <div className="relative bg-gray-100 rounded-lg overflow-hidden">
                <img
                  src={`/api/screenshots/${selectedScreenshot.id}/image`}
                  alt={`Screenshot ${selectedScreenshot.id}`}
                  className={`w-full ${isFullscreen ? 'h-[calc(100vh-200px)]' : 'max-h-[600px]'} object-contain cursor-zoom-in`}
                  onClick={toggleFullscreen}
                />
                <div className="absolute top-4 left-4">
                  <span
                    className={`px-3 py-1 rounded-md text-sm font-semibold ${getLabelColor(
                      selectedScreenshot.label
                    )}`}
                  >
                    {getLabelIcon(selectedScreenshot.label)} {selectedScreenshot.label}
                  </span>
                </div>
              </div>

              {/* Metadata Grid */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                {/* Basic Information */}
                <div className="space-y-4">
                  <h3 className="text-lg font-semibold text-gray-900 border-b border-gray-200 pb-2">
                    Basic Information
                  </h3>
                  <div className="space-y-3">
                    <div>
                      <label className="text-sm font-medium text-gray-500">Camera</label>
                      <p className="text-sm text-gray-900 mt-1">
                        {cameras.find((c) => c.id === selectedScreenshot.camera_id)?.name ||
                          selectedScreenshot.camera_id}
                      </p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-gray-500">Label</label>
                      <p className="text-sm text-gray-900 mt-1">
                        <span
                          className={`inline-block px-2 py-1 rounded text-xs font-semibold ${getLabelColor(
                            selectedScreenshot.label
                          )}`}
                        >
                          {getLabelIcon(selectedScreenshot.label)} {selectedScreenshot.label}
                        </span>
                      </p>
                    </div>
                    {selectedScreenshot.custom_label && (
                      <div>
                        <label className="text-sm font-medium text-gray-500">Custom Label</label>
                        <p className="text-sm text-gray-900 mt-1">{selectedScreenshot.custom_label}</p>
                      </div>
                    )}
                    {selectedScreenshot.description && (
                      <div>
                        <label className="text-sm font-medium text-gray-500">Description</label>
                        <p className="text-sm text-gray-900 mt-1 whitespace-pre-wrap">
                          {selectedScreenshot.description}
                        </p>
                      </div>
                    )}
                    {selectedScreenshot.created_by && (
                      <div>
                        <label className="text-sm font-medium text-gray-500">Created By</label>
                        <p className="text-sm text-gray-900 mt-1">
                          {selectedScreenshot.created_by}
                        </p>
                      </div>
                    )}
                  </div>
                </div>

                {/* Timestamps and File Info */}
                <div className="space-y-4">
                  <h3 className="text-lg font-semibold text-gray-900 border-b border-gray-200 pb-2">
                    Timestamps & File Info
                  </h3>
                  <div className="space-y-3">
                    <div>
                      <label className="text-sm font-medium text-gray-500">Created At</label>
                      <p className="text-sm text-gray-900 mt-1">
                        {new Date(selectedScreenshot.created_at).toLocaleString()}
                      </p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-gray-500">Updated At</label>
                      <p className="text-sm text-gray-900 mt-1">
                        {new Date(selectedScreenshot.updated_at).toLocaleString()}
                      </p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-gray-500">File Path</label>
                      <p className="text-sm text-gray-600 mt-1 font-mono break-all">
                        {selectedScreenshot.file_path}
                      </p>
                    </div>
                    {/* Metadata badges for quick identification (Substep 2.2.2.5.5) */}
                    {selectedScreenshot.metadata && (
                      <div>
                        <label className="text-sm font-medium text-gray-500 mb-2 block">
                          Quick Info
                        </label>
                        <div className="flex flex-wrap gap-2">
                          {selectedScreenshot.metadata.width && selectedScreenshot.metadata.height && (
                            <span className="inline-flex items-center gap-1 px-2 py-1 bg-blue-100 text-blue-800 rounded-md text-xs font-medium">
                              <Info className="w-3 h-3" />
                              {selectedScreenshot.metadata.width}×{selectedScreenshot.metadata.height} px
                            </span>
                          )}
                          {selectedScreenshot.metadata.processed_size_bytes && (
                            <span className="inline-flex items-center gap-1 px-2 py-1 bg-gray-100 text-gray-800 rounded-md text-xs font-medium">
                              <Info className="w-3 h-3" />
                              {((selectedScreenshot.metadata.processed_size_bytes as number) / 1024).toFixed(2)} KB
                            </span>
                          )}
                          {selectedScreenshot.metadata.original_format && (
                            <span className="inline-flex items-center gap-1 px-2 py-1 bg-purple-100 text-purple-800 rounded-md text-xs font-medium">
                              <Tag className="w-3 h-3" />
                              {String(selectedScreenshot.metadata.original_format).toUpperCase()}
                            </span>
                          )}
                          {selectedScreenshot.metadata.compression_ratio && (
                            <span className="inline-flex items-center gap-1 px-2 py-1 bg-green-100 text-green-800 rounded-md text-xs font-medium">
                              <Info className="w-3 h-3" />
                              {((1 - (selectedScreenshot.metadata.compression_ratio as number)) * 100).toFixed(0)}% compressed
                            </span>
                          )}
                          {selectedScreenshot.metadata.converted_from && (
                            <span className="inline-flex items-center gap-1 px-2 py-1 bg-yellow-100 text-yellow-800 rounded-md text-xs font-medium">
                              <Tag className="w-3 h-3" />
                              Converted from {String(selectedScreenshot.metadata.converted_from).toUpperCase()}
                            </span>
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              </div>

              {/* Metadata JSON Viewer (Collapsible) (Substep 2.2.2.5.5) */}
              {selectedScreenshot.metadata && Object.keys(selectedScreenshot.metadata).length > 0 && (
                <div className="space-y-2">
                  <button
                    onClick={() => {
                      const newExpanded = new Set(expandedMetadata)
                      if (newExpanded.has(selectedScreenshot.id)) {
                        newExpanded.delete(selectedScreenshot.id)
                      } else {
                        newExpanded.add(selectedScreenshot.id)
                      }
                      setExpandedMetadata(newExpanded)
                    }}
                    className="w-full flex items-center justify-between text-left"
                  >
                    <h3 className="text-lg font-semibold text-gray-900 border-b border-gray-200 pb-2 flex items-center gap-2">
                      <Info className="w-5 h-5 text-gray-500" />
                      Metadata
                      <span className="text-sm font-normal text-gray-500">
                        ({Object.keys(selectedScreenshot.metadata).length} fields)
                      </span>
                    </h3>
                    {expandedMetadata.has(selectedScreenshot.id) ? (
                      <ChevronUp className="w-5 h-5 text-gray-500" />
                    ) : (
                      <ChevronDown className="w-5 h-5 text-gray-500" />
                    )}
                  </button>
                  {expandedMetadata.has(selectedScreenshot.id) && (
                    <div className="bg-gray-50 rounded-lg p-4 border border-gray-200">
                      <pre className="text-xs text-gray-800 overflow-x-auto">
                        {JSON.stringify(selectedScreenshot.metadata, null, 2)}
                      </pre>
                    </div>
                  )}
                </div>
              )}

              {/* Action Buttons */}
              <div className="flex items-center justify-end gap-3 pt-4 border-t border-gray-200">
                <Button variant="secondary" onClick={closeDetailModal}>
                  Close
                </Button>
                <Button
                  variant="secondary"
                  onClick={() => {
                    if (selectedScreenshot) {
                      closeDetailModal()
                      openDeleteConfirm(selectedScreenshot)
                    }
                  }}
                >
                  <Trash2 className="w-4 h-4 mr-2" />
                  Delete
                </Button>
                <Button
                  onClick={() => {
                    if (selectedScreenshot) {
                      closeDetailModal()
                      openEditModal(selectedScreenshot)
                    }
                  }}
                >
                  <Edit className="w-4 h-4 mr-2" />
                  Edit
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Edit Screenshot Modal (Substep 2.2.2.5.3) */}
      {showEditModal && selectedScreenshot && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"
          role="dialog"
          aria-modal="true"
          aria-labelledby="edit-modal-title"
        >
          <div className="bg-white rounded-lg p-6 max-w-2xl w-full mx-4 max-h-[90vh] overflow-y-auto">
            <h2 id="edit-modal-title" className="text-xl font-bold mb-4">Edit Screenshot</h2>

            {/* Show error message */}
            {error && (
              <div className="bg-red-50 border border-red-200 rounded-lg p-3 mb-4">
                <p className="text-sm text-red-800">{error}</p>
              </div>
            )}

            {/* Show success message */}
            {successMessage && (
              <div className="bg-green-50 border border-green-200 rounded-lg p-3 mb-4">
                <p className="text-sm text-green-800">{successMessage}</p>
              </div>
            )}

            <div className="space-y-4">
              {/* Label Dropdown */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Label <span className="text-red-500">*</span>
                </label>
                <Select
                  value={editLabel}
                  onChange={(e) =>
                    setEditLabel(e.target.value as 'normal' | 'threat' | 'abnormal' | 'custom')
                  }
                  options={[
                    { value: 'normal', label: 'Normal' },
                    { value: 'threat', label: 'Threat' },
                    { value: 'abnormal', label: 'Abnormal' },
                    { value: 'custom', label: 'Custom' },
                  ]}
                />
              </div>

              {/* Custom Label Input (shown when label is "custom") */}
              {editLabel === 'custom' && (
                <div>
                  <Input
                    label="Custom Label"
                    value={editCustomLabel}
                    onChange={(e) => setEditCustomLabel(e.target.value)}
                    placeholder="Enter custom label"
                    required
                  />
                </div>
              )}

              {/* Description Textarea */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Description (optional)
                </label>
                <textarea
                  value={editDescription}
                  onChange={(e) => setEditDescription(e.target.value)}
                  placeholder="Describe what you see in this image"
                  rows={4}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                />
              </div>

              {/* Metadata JSON Editor */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Metadata (JSON)
                </label>
                <textarea
                  value={editMetadata}
                  onChange={(e) => {
                    setEditMetadata(e.target.value)
                    setEditMetadataError(null)
                    // Try to validate JSON on change
                    if (e.target.value.trim()) {
                      try {
                        JSON.parse(e.target.value)
                        setEditMetadataError(null)
                      } catch (err) {
                        // Don't show error while typing, only on blur or submit
                      }
                    }
                  }}
                  onBlur={() => {
                    if (editMetadata.trim()) {
                      try {
                        JSON.parse(editMetadata)
                        setEditMetadataError(null)
                      } catch (err) {
                        setEditMetadataError('Invalid JSON format')
                      }
                    }
                  }}
                  placeholder='{"key": "value"}'
                  rows={8}
                  className={`w-full px-3 py-2 border rounded-md shadow-sm focus:outline-none focus:ring-2 focus:border-blue-500 font-mono text-sm ${editMetadataError
                    ? 'border-red-300 focus:ring-red-500'
                    : 'border-gray-300 focus:ring-blue-500'
                    }`}
                />
                {editMetadataError && (
                  <p className="mt-1 text-sm text-red-600">{editMetadataError}</p>
                )}
                <p className="mt-1 text-xs text-gray-500">
                  Enter metadata as JSON object. Leave empty to keep existing metadata.
                </p>
              </div>

              {/* Action Buttons */}
              <div className="flex gap-2 justify-end pt-4 border-t border-gray-200">
                <Button variant="secondary" onClick={closeEditModal} disabled={updating}>
                  Cancel
                </Button>
                <Button onClick={saveScreenshotEdit} disabled={updating || (editLabel === 'custom' && !editCustomLabel.trim())}>
                  <Save className="w-4 h-4 mr-2" />
                  {updating ? 'Saving...' : 'Save Changes'}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal (Substep 2.2.2.5.4) */}
      {showDeleteConfirm && screenshotToDelete && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"
          role="dialog"
          aria-modal="true"
          aria-labelledby="delete-modal-title"
        >
          <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4">
            <h2 id="delete-modal-title" className="text-xl font-bold mb-4 text-red-600">Delete Screenshot</h2>

            {/* Warning Message */}
            <div className="bg-red-50 border border-red-200 rounded-lg p-4 mb-4">
              <p className="text-sm text-red-800 font-semibold mb-2">
                ⚠️ Warning: This action cannot be undone!
              </p>
              <p className="text-sm text-red-700">
                This screenshot will be permanently deleted from the system. All associated data, including the image file and metadata, will be removed.
              </p>
            </div>

            {/* Screenshot Preview */}
            <div className="mb-4">
              <h3 className="text-sm font-semibold text-gray-700 mb-2">Screenshot to Delete:</h3>
              <div className="border border-gray-200 rounded-lg p-3 bg-gray-50">
                <div className="flex gap-4">
                  {/* Thumbnail */}
                  <div className="flex-shrink-0">
                    <img
                      src={`/api/screenshots/${screenshotToDelete.id}/thumbnail`}
                      alt={`Screenshot ${screenshotToDelete.id}`}
                      className="w-24 h-24 object-cover rounded"
                      onError={(e) => {
                        ; (e.target as HTMLImageElement).src = `/api/screenshots/${screenshotToDelete.id}/image`
                      }}
                    />
                  </div>
                  {/* Metadata Preview */}
                  <div className="flex-1 min-w-0">
                    <div className="space-y-1">
                      <p className="text-sm font-medium text-gray-900 truncate">
                        Camera: {cameras.find((c) => c.id === screenshotToDelete.camera_id)?.name || screenshotToDelete.camera_id}
                      </p>
                      <p className="text-sm text-gray-600">
                        Label:{' '}
                        <span
                          className={`inline-block px-2 py-0.5 rounded text-xs font-semibold ${getLabelColor(
                            screenshotToDelete.label
                          )}`}
                        >
                          {getLabelIcon(screenshotToDelete.label)} {screenshotToDelete.label}
                        </span>
                      </p>
                      {screenshotToDelete.custom_label && (
                        <p className="text-sm text-gray-600">Custom Label: {screenshotToDelete.custom_label}</p>
                      )}
                      {screenshotToDelete.description && (
                        <p className="text-sm text-gray-600 line-clamp-2">{screenshotToDelete.description}</p>
                      )}
                      <p className="text-xs text-gray-500">
                        Created: {new Date(screenshotToDelete.created_at).toLocaleString()}
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            {/* Error Message */}
            {error && (
              <div className="bg-red-50 border border-red-200 rounded-lg p-3 mb-4">
                <p className="text-sm text-red-800">{error}</p>
              </div>
            )}

            {/* Action Buttons */}
            <div className="flex gap-3 justify-end">
              <Button variant="secondary" onClick={closeDeleteConfirm} disabled={deleting}>
                Cancel
              </Button>
              <Button
                onClick={() => deleteScreenshot(screenshotToDelete)}
                disabled={deleting}
                className="bg-red-600 hover:bg-red-700 text-white"
              >
                <Trash2 className="w-4 h-4 mr-2" />
                {deleting ? 'Deleting...' : 'Delete Permanently'}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Bulk Delete Confirmation Modal (Substep 2.2.2.5.6) */}
      {showBulkDeleteConfirm && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"
          role="dialog"
          aria-modal="true"
          aria-labelledby="bulk-delete-modal-title"
        >
          <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4">
            <h2 id="bulk-delete-modal-title" className="text-xl font-bold mb-4 text-red-600">Delete Multiple Screenshots</h2>

            {/* Warning Message */}
            <div className="bg-red-50 border border-red-200 rounded-lg p-4 mb-4">
              <p className="text-sm text-red-800 font-semibold mb-2">
                ⚠️ Warning: This action cannot be undone!
              </p>
              <p className="text-sm text-red-700">
                You are about to permanently delete <strong>{selectedIds.size}</strong> screenshot{selectedIds.size !== 1 ? 's' : ''} from the system. All associated data, including image files and metadata, will be removed.
              </p>
            </div>

            {/* Error Message */}
            {error && (
              <div className="bg-red-50 border border-red-200 rounded-lg p-3 mb-4">
                <p className="text-sm text-red-800">{error}</p>
              </div>
            )}

            {/* Action Buttons */}
            <div className="flex gap-3 justify-end">
              <Button variant="secondary" onClick={closeBulkDeleteConfirm} disabled={bulkDeleting}>
                Cancel
              </Button>
              <Button
                onClick={bulkDelete}
                disabled={bulkDeleting}
                className="bg-red-600 hover:bg-red-700 text-white"
              >
                <Trash2 className="w-4 h-4 mr-2" />
                {bulkDeleting ? 'Deleting...' : `Delete ${selectedIds.size} Screenshot${selectedIds.size !== 1 ? 's' : ''}`}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Bulk Label Change Modal (Substep 2.2.2.5.6) */}
      {showBulkLabelModal && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"
          role="dialog"
          aria-modal="true"
          aria-labelledby="bulk-label-modal-title"
        >
          <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4">
            <h2 id="bulk-label-modal-title" className="text-xl font-bold mb-4">Change Label for Multiple Screenshots</h2>

            {/* Info Message */}
            <div className="bg-blue-50 border border-blue-200 rounded-lg p-4 mb-4">
              <p className="text-sm text-blue-800">
                You are about to change the label for <strong>{selectedIds.size}</strong> screenshot{selectedIds.size !== 1 ? 's' : ''}.
              </p>
            </div>

            {/* Error Message */}
            {error && (
              <div className="bg-red-50 border border-red-200 rounded-lg p-3 mb-4">
                <p className="text-sm text-red-800">{error}</p>
              </div>
            )}

            <div className="space-y-4">
              {/* Label Dropdown */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  New Label <span className="text-red-500">*</span>
                </label>
                <Select
                  value={bulkLabel}
                  onChange={(e) =>
                    setBulkLabel(e.target.value as 'normal' | 'threat' | 'abnormal' | 'custom')
                  }
                  options={[
                    { value: 'normal', label: 'Normal' },
                    { value: 'threat', label: 'Threat' },
                    { value: 'abnormal', label: 'Abnormal' },
                    { value: 'custom', label: 'Custom' },
                  ]}
                />
              </div>

              {/* Custom Label Input (shown when label is "custom") */}
              {bulkLabel === 'custom' && (
                <div>
                  <Input
                    label="Custom Label"
                    value={bulkCustomLabel}
                    onChange={(e) => setBulkCustomLabel(e.target.value)}
                    placeholder="Enter custom label"
                    required
                  />
                </div>
              )}

              {/* Action Buttons */}
              <div className="flex gap-3 justify-end pt-4 border-t border-gray-200">
                <Button variant="secondary" onClick={closeBulkLabelModal} disabled={bulkUpdating}>
                  Cancel
                </Button>
                <Button
                  onClick={bulkUpdateLabel}
                  disabled={bulkUpdating || (bulkLabel === 'custom' && !bulkCustomLabel.trim())}
                >
                  <Tag className="w-4 h-4 mr-2" />
                  {bulkUpdating ? 'Updating...' : `Update ${selectedIds.size} Screenshot${selectedIds.size !== 1 ? 's' : ''}`}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

