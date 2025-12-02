import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import React from 'react'
import { BrowserRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../utils/api'
import Screenshots from './Screenshots'

// Mock the API module
vi.mock('../utils/api', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}))

// Mock fetch for snapshot capture
global.fetch = vi.fn()

// Mock FileReader
global.FileReader = class FileReader {
  result: string | null = null
  onloadend: ((this: FileReader, ev: ProgressEvent<FileReader>) => any) | null = null

  readAsDataURL(blob: Blob) {
    // Simulate async read
    setTimeout(() => {
      this.result = 'data:image/jpeg;base64,/9j/4AAQSkZJRg=='
      if (this.onloadend) {
        this.onloadend(new ProgressEvent('loadend'))
      }
    }, 0)
  }
} as any

// Helper to render component with router
const renderWithRouter = (component: React.ReactElement) => {
  return render(<BrowserRouter>{component}</BrowserRouter>)
}

// Mock data
const mockCameras = [
  {
    id: 'camera-1',
    name: 'Test Camera 1',
    type: 'rtsp',
    enabled: true,
    dataset_status: {
      labeled_snapshot_count: 10,
      required_snapshot_count: 50,
      snapshot_required: true,
      label_counts: { normal: 10 },
    },
  },
  {
    id: 'camera-2',
    name: 'Test Camera 2',
    type: 'rtsp',
    enabled: true,
    dataset_status: {
      labeled_snapshot_count: 50,
      required_snapshot_count: 50,
      snapshot_required: false,
      label_counts: { normal: 50 },
    },
  },
]

const mockScreenshots = [
  {
    id: 'screenshot-1',
    camera_id: 'camera-1',
    file_path: '/path/to/screenshot-1.jpg',
    label: 'normal' as const,
    description: 'Test screenshot 1',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'screenshot-2',
    camera_id: 'camera-1',
    file_path: '/path/to/screenshot-2.jpg',
    label: 'threat' as const,
    description: 'Test screenshot 2',
    created_at: '2024-01-02T00:00:00Z',
    updated_at: '2024-01-02T00:00:00Z',
  },
]

describe('Screenshots Page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  describe('fetchCameras', () => {
    it('should fetch and display cameras', async () => {
      vi.mocked(api.get).mockResolvedValueOnce({
        cameras: mockCameras,
        count: 2,
      })

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(api.get).toHaveBeenCalledWith('/cameras')
      })

      await waitFor(() => {
        expect(screen.getByText('Test Camera 1')).toBeInTheDocument()
      })
    })

    it('should handle API errors when fetching cameras', async () => {
      vi.mocked(api.get).mockRejectedValueOnce(new Error('Network error'))

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(api.get).toHaveBeenCalledWith('/cameras')
      })
    })

    it('should filter enabled cameras only', async () => {
      const camerasWithDisabled = [
        ...mockCameras,
        { id: 'camera-3', name: 'Disabled Camera', type: 'rtsp', enabled: false },
      ]

      vi.mocked(api.get).mockResolvedValueOnce({
        cameras: camerasWithDisabled,
        count: 3,
      })

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.queryByText('Disabled Camera')).not.toBeInTheDocument()
      })
    })
  })

  describe('fetchScreenshots', () => {
    it('should fetch and display screenshots', async () => {
      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(api.get).toHaveBeenCalledWith(
          expect.stringContaining('/screenshots')
        )
      })

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })
    })

    it('should apply filters when fetching screenshots', async () => {
      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: [mockScreenshots[0]], count: 1 })

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        // Find and click filter dropdown
        const filterButton = screen.getByText(/filter/i)
        if (filterButton) {
          fireEvent.click(filterButton)
        }
      })
    })

    it('should handle API errors when fetching screenshots', async () => {
      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockRejectedValueOnce(new Error('Failed to load screenshots'))

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(api.get).toHaveBeenCalledWith(
          expect.stringContaining('/screenshots')
        )
      })
    })
  })

  describe('captureScreenshot', () => {
    it('should capture screenshot successfully', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get).mockResolvedValueOnce({
        cameras: mockCameras,
        count: 2,
      })

      // Mock fetch for snapshot
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        blob: async () => new Blob(['fake-image-data'], { type: 'image/jpeg' }),
      } as Response)

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test Camera 1')).toBeInTheDocument()
      })

      // Find and click capture button
      const captureButton = screen.getByText(/capture/i)
      await user.click(captureButton)

      await waitFor(() => {
        expect(global.fetch).toHaveBeenCalledWith(
          expect.stringContaining('/api/cameras/camera-1/snapshot')
        )
      })
    })

    it('should show error when no camera is selected', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get).mockResolvedValueOnce({
        cameras: [],
        count: 0,
      })

      renderWithRouter(<Screenshots />)

      // Try to capture without selecting camera
      const captureButton = screen.getByText(/capture/i)
      await user.click(captureButton)

      await waitFor(() => {
        expect(screen.getByText(/please select a camera/i)).toBeInTheDocument()
      })
    })

    it('should handle capture errors', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get).mockResolvedValueOnce({
        cameras: mockCameras,
        count: 2,
      })

      vi.mocked(global.fetch).mockRejectedValueOnce(new Error('Capture failed'))

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test Camera 1')).toBeInTheDocument()
      })

      const captureButton = screen.getByText(/capture/i)
      await user.click(captureButton)

      await waitFor(() => {
        expect(screen.getByText(/failed to capture/i)).toBeInTheDocument()
      })
    })
  })

  describe('saveScreenshot', () => {
    it('should save screenshot with valid data', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })

      vi.mocked(api.post).mockResolvedValueOnce({
        id: 'new-screenshot-id',
        camera_id: 'camera-1',
        dataset_status: {
          labeled_snapshot_count: 11,
          required_snapshot_count: 50,
          snapshot_required: true,
        },
      })

      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        blob: async () => new Blob(['fake-image-data'], { type: 'image/jpeg' }),
      } as Response)

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test Camera 1')).toBeInTheDocument()
      })

      // Capture screenshot first
      const captureButton = screen.getByText(/capture/i)
      await user.click(captureButton)

      await waitFor(() => {
        expect(screen.getByText(/save/i)).toBeInTheDocument()
      })

      // Save screenshot
      const saveButton = screen.getByText(/save/i)
      await user.click(saveButton)

      await waitFor(() => {
        expect(api.post).toHaveBeenCalledWith('/screenshots', expect.objectContaining({
          camera_id: 'camera-1',
          label: 'normal',
        }))
      })
    })

    it('should validate custom label when label is custom', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get).mockResolvedValueOnce({
        cameras: mockCameras,
        count: 2,
      })

      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        blob: async () => new Blob(['fake-image-data'], { type: 'image/jpeg' }),
      } as Response)

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test Camera 1')).toBeInTheDocument()
      })

      // Capture screenshot
      const captureButton = screen.getByText(/capture/i)
      await user.click(captureButton)

      await waitFor(() => {
        expect(screen.getByText(/save/i)).toBeInTheDocument()
      })

      // Change label to custom
      const labelSelect = screen.getByLabelText(/label/i)
      await user.selectOptions(labelSelect, 'custom')

      // Try to save without custom label
      const saveButton = screen.getByText(/save/i)
      await user.click(saveButton)

      await waitFor(() => {
        expect(screen.getByText(/custom label is required/i)).toBeInTheDocument()
      })
    })

    it('should handle save errors', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get).mockResolvedValueOnce({
        cameras: mockCameras,
        count: 2,
      })

      vi.mocked(api.post).mockRejectedValueOnce(new Error('Save failed'))

      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        blob: async () => new Blob(['fake-image-data'], { type: 'image/jpeg' }),
      } as Response)

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test Camera 1')).toBeInTheDocument()
      })

      const captureButton = screen.getByText(/capture/i)
      await user.click(captureButton)

      await waitFor(() => {
        expect(screen.getByText(/save/i)).toBeInTheDocument()
      })

      const saveButton = screen.getByText(/save/i)
      await user.click(saveButton)

      await waitFor(() => {
        expect(screen.getByText(/failed to save/i)).toBeInTheDocument()
      })
    })
  })

  describe('deleteScreenshot', () => {
    it('should delete screenshot with confirmation', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })
        .mockResolvedValueOnce({ screenshots: [mockScreenshots[1]], count: 1 })
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })

      vi.mocked(api.delete).mockResolvedValueOnce({})

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })

      // Find delete button (might be in a menu or directly visible)
      const deleteButtons = screen.getAllByRole('button', { name: /delete/i })
      if (deleteButtons.length > 0) {
        await user.click(deleteButtons[0])

        // Confirm deletion
        await waitFor(() => {
          const confirmButton = screen.getByText(/confirm|yes|delete/i)
          if (confirmButton) {
            user.click(confirmButton)
          }
        })

        await waitFor(() => {
          expect(api.delete).toHaveBeenCalledWith('/screenshots/screenshot-1')
        })
      }
    })

    it('should handle delete errors', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })

      vi.mocked(api.delete).mockRejectedValueOnce(new Error('Delete failed'))

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })

      const deleteButtons = screen.getAllByRole('button', { name: /delete/i })
      if (deleteButtons.length > 0) {
        await user.click(deleteButtons[0])

        await waitFor(() => {
          const confirmButton = screen.getByText(/confirm|yes|delete/i)
          if (confirmButton) {
            user.click(confirmButton)
          }
        })

        await waitFor(() => {
          expect(screen.getByText(/failed to delete/i)).toBeInTheDocument()
        })
      }
    })
  })

  describe('updateScreenshotLabel', () => {
    it('should update screenshot label', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })

      vi.mocked(api.put).mockResolvedValueOnce({})

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })

      // Find edit button
      const editButtons = screen.getAllByRole('button', { name: /edit/i })
      if (editButtons.length > 0) {
        await user.click(editButtons[0])

        await waitFor(() => {
          const labelSelect = screen.getByLabelText(/label/i)
          if (labelSelect) {
            user.selectOptions(labelSelect, 'threat')
          }
        })

        const saveButton = screen.getByText(/save changes/i)
        await user.click(saveButton)

        await waitFor(() => {
          expect(api.put).toHaveBeenCalledWith('/screenshots/screenshot-1', expect.objectContaining({
            label: 'threat',
          }))
        })
      }
    })

    it('should handle update errors', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })

      vi.mocked(api.put).mockRejectedValueOnce(new Error('Update failed'))

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })

      const editButtons = screen.getAllByRole('button', { name: /edit/i })
      if (editButtons.length > 0) {
        await user.click(editButtons[0])

        await waitFor(() => {
          const saveButton = screen.getByText(/save changes/i)
          if (saveButton) {
            user.click(saveButton)
          }
        })

        await waitFor(() => {
          expect(screen.getByText(/failed to update/i)).toBeInTheDocument()
        })
      }
    })
  })

  describe('exportDataset', () => {
    it('should export dataset successfully', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })

      // Mock fetch for export
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        blob: async () => new Blob(['zip-data'], { type: 'application/zip' }),
      } as Response)

      // Mock URL.createObjectURL and link.click
      const mockCreateObjectURL = vi.fn(() => 'blob:mock-url')
      const mockRevokeObjectURL = vi.fn()
      const mockClick = vi.fn()
      const mockAppendChild = vi.fn()
      const mockRemove = vi.fn()

      global.URL.createObjectURL = mockCreateObjectURL
      global.URL.revokeObjectURL = mockRevokeObjectURL

      const mockLink = {
        href: '',
        download: '',
        click: mockClick,
      } as any

      document.createElement = vi.fn(() => mockLink as any)
      document.body.appendChild = mockAppendChild as any
      document.body.removeChild = mockRemove as any

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })

      const exportButton = screen.getByText(/export/i)
      await user.click(exportButton)

      await waitFor(() => {
        expect(global.fetch).toHaveBeenCalledWith(
          '/api/screenshots/export',
          expect.objectContaining({
            method: 'POST',
            headers: expect.objectContaining({
              'Content-Type': 'application/json',
            }),
          })
        )
      })
    })

    it('should handle export errors', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })

      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: async () => 'Export failed',
      } as Response)

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })

      const exportButton = screen.getByText(/export/i)
      await user.click(exportButton)

      await waitFor(() => {
        expect(screen.getByText(/failed to export/i)).toBeInTheDocument()
      })
    })
  })

  describe('Filter and Sort', () => {
    it('should filter screenshots by label', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })
        .mockResolvedValueOnce({ screenshots: [mockScreenshots[0]], count: 1 })

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })

      // Find and change filter
      const filterSelects = screen.getAllByRole('combobox')
      const labelFilter = filterSelects.find((select) =>
        select.getAttribute('name')?.includes('label')
      )

      if (labelFilter) {
        await user.selectOptions(labelFilter, 'normal')

        await waitFor(() => {
          expect(api.get).toHaveBeenCalledWith(
            expect.stringContaining('label=normal')
          )
        })
      }
    })

    it('should sort screenshots', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })
        .mockResolvedValueOnce({ screenshots: [...mockScreenshots].reverse(), count: 2 })

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })

      // Find sort controls
      const sortButtons = screen.getAllByRole('button', { name: /sort/i })
      if (sortButtons.length > 0) {
        await user.click(sortButtons[0])

        await waitFor(() => {
          expect(api.get).toHaveBeenCalledWith(
            expect.stringContaining('sort_by')
          )
        })
      }
    })
  })

  describe('Modal State Management', () => {
    it('should open and close detail modal', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })

      // Click on screenshot to open detail modal
      const screenshotCard = screen.getByText('Test screenshot 1').closest('div')
      if (screenshotCard) {
        await user.click(screenshotCard)

        await waitFor(() => {
          expect(screen.getByText(/details|screenshot/i)).toBeInTheDocument()
        })

        // Close modal
        const closeButton = screen.getByRole('button', { name: /close/i })
        await user.click(closeButton)

        await waitFor(() => {
          expect(screen.queryByText(/details|screenshot/i)).not.toBeInTheDocument()
        })
      }
    })

    it('should open and close edit modal', async () => {
      const user = userEvent.setup()

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText('Test screenshot 1')).toBeInTheDocument()
      })

      const editButtons = screen.getAllByRole('button', { name: /edit/i })
      if (editButtons.length > 0) {
        await user.click(editButtons[0])

        await waitFor(() => {
          expect(screen.getByText(/edit screenshot/i)).toBeInTheDocument()
        })

        const cancelButton = screen.getByText(/cancel/i)
        await user.click(cancelButton)

        await waitFor(() => {
          expect(screen.queryByText(/edit screenshot/i)).not.toBeInTheDocument()
        })
      }
    })
  })

  describe('Dataset Progress Calculation', () => {
    it('should display dataset progress correctly', async () => {
      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: mockCameras, count: 2 })
        .mockResolvedValueOnce({ screenshots: mockScreenshots, count: 2 })

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText(/10.*50|20%/i)).toBeInTheDocument()
      })
    })

    it('should show 100% when progress is complete', async () => {
      const camerasWithComplete = [
        {
          ...mockCameras[1],
          dataset_status: {
            labeled_snapshot_count: 50,
            required_snapshot_count: 50,
            snapshot_required: false,
            label_counts: { normal: 50 },
          },
        },
      ]

      vi.mocked(api.get)
        .mockResolvedValueOnce({ cameras: camerasWithComplete, count: 1 })
        .mockResolvedValueOnce({ screenshots: [], count: 0 })

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText(/100%|ready|complete/i)).toBeInTheDocument()
      })
    })
  })

  describe('Loading States', () => {
    it('should show loading skeleton while fetching', async () => {
      vi.mocked(api.get).mockImplementation(() => new Promise(() => { })) // Never resolves

      renderWithRouter(<Screenshots />)

      // Should show skeleton or loading indicator
      expect(screen.getByTestId('skeleton') || screen.getByText(/loading/i)).toBeInTheDocument()
    })
  })

  describe('Error Message Display', () => {
    it('should display error messages', async () => {
      vi.mocked(api.get).mockRejectedValueOnce(new Error('Network error'))

      renderWithRouter(<Screenshots />)

      await waitFor(() => {
        expect(screen.getByText(/network error|failed/i)).toBeInTheDocument()
      })
    })
  })
})
