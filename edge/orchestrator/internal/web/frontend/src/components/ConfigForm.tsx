import { useState, useEffect } from 'react'
import Input from './Input'
import Select from './Select'
import Button from './Button'
import Card from './Card'
import Loading from './Loading'
import { api } from '../utils/api'
import { Save, AlertCircle, CheckCircle2 } from 'lucide-react'

interface ConfigFormProps {
  section: string
  title: string
  fields: ConfigField[]
  onSave?: () => void
}

interface ConfigField {
  name: string
  label: string
  type: 'text' | 'number' | 'boolean' | 'duration' | 'select' | 'array'
  path?: string // For nested fields (e.g., "discovery.enabled")
  options?: Array<{ value: string; label: string }>
  min?: number
  max?: number
  step?: number
  placeholder?: string
  helpText?: string
}

export default function ConfigForm({ section, title, fields, onSave }: ConfigFormProps) {
  const [config, setConfig] = useState<Record<string, any>>({})
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  useEffect(() => {
    const fetchConfig = async () => {
      try {
        setLoading(true)
        const response = await api.get<{ section: string; config: any }>(
          `/config?section=${section}`
        )
        setConfig(response.config || {})
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load configuration')
      } finally {
        setLoading(false)
      }
    }

    fetchConfig()
  }, [section])

  const getNestedValue = (obj: any, path: string): any => {
    const keys = path.split('.')
    let value = obj
    for (const key of keys) {
      value = value?.[key]
    }
    return value
  }

  const setNestedValue = (obj: any, path: string, value: any): any => {
    const keys = path.split('.')
    const newObj = { ...obj }
    let current = newObj
    for (let i = 0; i < keys.length - 1; i++) {
      const key = keys[i]
      if (!current[key] || typeof current[key] !== 'object') {
        current[key] = {}
      } else {
        current[key] = { ...current[key] }
      }
      current = current[key]
    }
    current[keys[keys.length - 1]] = value
    return newObj
  }

  const handleChange = (field: ConfigField, value: any) => {
    const fieldPath = field.path || field.name
    setConfig(setNestedValue(config, fieldPath, value))
    setError(null)
    setSuccess(false)
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError(null)
    setSuccess(false)

    try {
      await api.put(`/config?section=${section}`, config)
      setSuccess(true)
      if (onSave) {
        onSave()
      }
      // Clear success message after 3 seconds
      setTimeout(() => setSuccess(false), 3000)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save configuration')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <Card>
        <Loading text={`Loading ${title} configuration...`} />
      </Card>
    )
  }

  return (
    <Card>
      <form onSubmit={handleSubmit} className="space-y-6">
        <div className="card-header">
          <h3 className="text-lg font-semibold text-gray-900">{title}</h3>
        </div>

        {error && (
          <div className="bg-red-50 border border-red-200 rounded-lg p-4 flex items-start">
            <AlertCircle className="h-5 w-5 text-red-600 mr-2 flex-shrink-0 mt-0.5" />
            <p className="text-sm text-red-600">{error}</p>
          </div>
        )}

        {success && (
          <div className="bg-green-50 border border-green-200 rounded-lg p-4 flex items-start">
            <CheckCircle2 className="h-5 w-5 text-green-600 mr-2 flex-shrink-0 mt-0.5" />
            <p className="text-sm text-green-600">Configuration saved successfully</p>
          </div>
        )}

        <div className="space-y-4">
          {fields.map((field) => {
            const fieldPath = field.path || field.name
            const value = getNestedValue(config, fieldPath)

            if (field.type === 'boolean') {
              return (
                <div key={field.name} className="flex items-center justify-between">
                  <div className="flex-1">
                    <label className="block text-sm font-medium text-gray-700 mb-1">
                      {field.label}
                    </label>
                    {field.helpText && (
                      <p className="text-xs text-gray-500 mb-2">{field.helpText}</p>
                    )}
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      checked={value || false}
                      onChange={(e) => handleChange(field, e.target.checked)}
                      className="sr-only peer"
                    />
                    <div className="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
                  </label>
                </div>
              )
            }

            if (field.type === 'select') {
              return (
                <Select
                  key={field.name}
                  label={field.label}
                  value={value || ''}
                  onChange={(e) => handleChange(field, e.target.value)}
                  options={field.options || []}
                  error={undefined}
                />
              )
            }

            if (field.type === 'duration') {
              return (
                <Input
                  key={field.name}
                  label={field.label}
                  type="text"
                  value={value || ''}
                  onChange={(e) => handleChange(field, e.target.value)}
                  placeholder={field.placeholder || 'e.g., 30s, 5m, 1h'}
                  error={undefined}
                />
              )
            }

            if (field.type === 'number') {
              return (
                <Input
                  key={field.name}
                  label={field.label}
                  type="number"
                  value={value || ''}
                  onChange={(e) =>
                    handleChange(field, field.step ? parseFloat(e.target.value) : parseInt(e.target.value, 10))
                  }
                  min={field.min}
                  max={field.max}
                  step={field.step}
                  placeholder={field.placeholder}
                  error={undefined}
                />
              )
            }

            if (field.type === 'array') {
              return (
                <div key={field.name}>
                  <label className="block text-sm font-medium text-gray-700 mb-1">
                    {field.label}
                  </label>
                  {field.helpText && (
                    <p className="text-xs text-gray-500 mb-2">{field.helpText}</p>
                  )}
                  <textarea
                    value={Array.isArray(value) ? value.join(', ') : ''}
                    onChange={(e) => {
                      const items = e.target.value
                        .split(',')
                        .map((item) => item.trim())
                        .filter((item) => item.length > 0)
                      handleChange(field, items)
                    }}
                    className="input"
                    placeholder={field.placeholder || 'Comma-separated values'}
                    rows={3}
                  />
                </div>
              )
            }

            return (
              <Input
                key={field.name}
                label={field.label}
                type="text"
                value={value || ''}
                onChange={(e) => handleChange(field, e.target.value)}
                placeholder={field.placeholder}
                error={undefined}
              />
            )
          })}
        </div>

        <div className="flex justify-end pt-4 border-t border-gray-200">
          <Button type="submit" disabled={saving}>
            {saving ? (
              <div className="flex items-center">
                <div className="h-4 w-4 border-2 border-white border-t-transparent rounded-full animate-spin mr-2" />
                <span>Saving...</span>
              </div>
            ) : (
              <>
                <Save className="h-4 w-4 mr-2" />
                Save Configuration
              </>
            )}
          </Button>
        </div>
      </form>
    </Card>
  )
}

