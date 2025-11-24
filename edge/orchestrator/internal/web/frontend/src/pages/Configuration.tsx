import { useState } from 'react'
import ConfigForm from '../components/ConfigForm'
import { Settings, Camera, Brain, HardDrive, Shield, Activity, Lock } from 'lucide-react'
import Button from '../components/Button'

interface ConfigSection {
  id: string
  title: string
  icon: React.ComponentType<{ className?: string }>
  fields: Array<{
    name: string
    label: string
    type: 'text' | 'number' | 'boolean' | 'duration' | 'select' | 'array'
    path?: string
    options?: Array<{ value: string; label: string }>
    min?: number
    max?: number
    step?: number
    placeholder?: string
    helpText?: string
  }>
}

const configSections: ConfigSection[] = [
  {
    id: 'cameras',
    title: 'Camera Configuration',
    icon: Camera,
    fields: [
      {
        name: 'discovery.enabled',
        label: 'Enable Camera Discovery',
        type: 'boolean',
        path: 'Discovery.Enabled',
        helpText: 'Automatically discover cameras on the network',
      },
      {
        name: 'discovery.interval',
        label: 'Discovery Interval',
        type: 'duration',
        path: 'Discovery.Interval',
        placeholder: '30s',
        helpText: 'How often to scan for new cameras',
      },
      {
        name: 'rtsp.timeout',
        label: 'RTSP Timeout',
        type: 'duration',
        path: 'RTSP.Timeout',
        placeholder: '10s',
        helpText: 'Timeout for RTSP connections',
      },
      {
        name: 'rtsp.reconnect_interval',
        label: 'RTSP Reconnect Interval',
        type: 'duration',
        path: 'RTSP.ReconnectInterval',
        placeholder: '5s',
        helpText: 'How long to wait before reconnecting to a failed RTSP stream',
      },
    ],
  },
  {
    id: 'ai',
    title: 'AI Configuration',
    icon: Brain,
    fields: [
      {
        name: 'service_url',
        label: 'AI Service URL',
        type: 'text',
        path: 'ServiceURL',
        placeholder: 'http://localhost:8080',
        helpText: 'URL of the AI inference service',
      },
      {
        name: 'confidence_threshold',
        label: 'Confidence Threshold',
        type: 'number',
        path: 'ConfidenceThreshold',
        min: 0,
        max: 1,
        step: 0.01,
        placeholder: '0.5',
        helpText: 'Minimum confidence score (0.0 to 1.0) for detections',
      },
      {
        name: 'inference_interval',
        label: 'Inference Interval',
        type: 'duration',
        path: 'InferenceInterval',
        placeholder: '1s',
        helpText: 'How often to run AI inference on camera frames',
      },
      {
        name: 'enabled_classes',
        label: 'Enabled Detection Classes',
        type: 'array',
        path: 'EnabledClasses',
        placeholder: 'person, car, dog',
        helpText: 'Comma-separated list of detection classes to enable (leave empty for all)',
      },
    ],
  },
  {
    id: 'storage',
    title: 'Storage Configuration',
    icon: HardDrive,
    fields: [
      {
        name: 'clips_dir',
        label: 'Clips Directory',
        type: 'text',
        path: 'ClipsDir',
        placeholder: '/var/lib/view-guard/clips',
        helpText: 'Directory where video clips are stored',
      },
      {
        name: 'snapshots_dir',
        label: 'Snapshots Directory',
        type: 'text',
        path: 'SnapshotsDir',
        placeholder: '/var/lib/view-guard/snapshots',
        helpText: 'Directory where snapshot images are stored',
      },
      {
        name: 'retention_days',
        label: 'Retention Days',
        type: 'number',
        path: 'RetentionDays',
        min: 1,
        max: 365,
        placeholder: '7',
        helpText: 'Number of days to keep clips and snapshots',
      },
      {
        name: 'max_disk_usage_percent',
        label: 'Max Disk Usage (%)',
        type: 'number',
        path: 'MaxDiskUsagePercent',
        min: 1,
        max: 100,
        step: 0.1,
        placeholder: '80.0',
        helpText: 'Maximum disk usage percentage before cleanup',
      },
    ],
  },
  {
    id: 'wireguard',
    title: 'WireGuard Configuration',
    icon: Shield,
    fields: [
      {
        name: 'enabled',
        label: 'Enable WireGuard',
        type: 'boolean',
        path: 'Enabled',
        helpText: 'Enable WireGuard VPN connection',
      },
      {
        name: 'config_path',
        label: 'Config Path',
        type: 'text',
        path: 'ConfigPath',
        placeholder: '/etc/wireguard/wg0.conf',
        helpText: 'Path to WireGuard configuration file',
      },
      {
        name: 'kvm_endpoint',
        label: 'KVM Endpoint',
        type: 'text',
        path: 'KVMEndpoint',
        placeholder: 'https://kvm.example.com',
        helpText: 'KVM server endpoint URL',
      },
    ],
  },
  {
    id: 'telemetry',
    title: 'Telemetry Configuration',
    icon: Activity,
    fields: [
      {
        name: 'enabled',
        label: 'Enable Telemetry',
        type: 'boolean',
        path: 'Enabled',
        helpText: 'Enable telemetry data collection and transmission',
      },
      {
        name: 'interval',
        label: 'Collection Interval',
        type: 'duration',
        path: 'Interval',
        placeholder: '30s',
        helpText: 'How often to collect and transmit telemetry data',
      },
    ],
  },
  {
    id: 'encryption',
    title: 'Encryption Configuration',
    icon: Lock,
    fields: [
      {
        name: 'enabled',
        label: 'Enable Encryption',
        type: 'boolean',
        path: 'Enabled',
        helpText: 'Enable encryption for stored data',
      },
      {
        name: 'salt',
        label: 'Salt (Hex)',
        type: 'text',
        path: 'Salt',
        placeholder: 'Auto-generated if empty',
        helpText: 'Salt for key derivation (hex encoded, leave empty to auto-generate)',
      },
      {
        name: 'salt_path',
        label: 'Salt File Path',
        type: 'text',
        path: 'SaltPath',
        placeholder: '/var/lib/view-guard/salt',
        helpText: 'Path where the salt is stored',
      },
    ],
  },
]

export default function Configuration() {
  const [activeSection, setActiveSection] = useState<string>('cameras')

  const activeConfig = configSections.find((s) => s.id === activeSection)

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-gray-900">Configuration</h1>
        <p className="mt-2 text-gray-600">System configuration settings</p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
        {/* Sidebar Navigation */}
        <div className="lg:col-span-1">
          <div className="card p-2">
            <nav className="space-y-1">
              {configSections.map((section) => {
                const Icon = section.icon
                const isActive = activeSection === section.id
                return (
                  <button
                    key={section.id}
                    onClick={() => setActiveSection(section.id)}
                    className={`
                      w-full flex items-center px-4 py-3 text-sm font-medium rounded-lg transition-colors
                      ${
                        isActive
                          ? 'bg-primary-50 text-primary-700'
                          : 'text-gray-700 hover:bg-gray-100 hover:text-gray-900'
                      }
                    `}
                  >
                    <Icon className="h-5 w-5 mr-3" />
                    {section.title}
                  </button>
                )
              })}
            </nav>
          </div>
        </div>

        {/* Configuration Form */}
        <div className="lg:col-span-3">
          {activeConfig ? (
            <ConfigForm
              section={activeConfig.id}
              title={activeConfig.title}
              fields={activeConfig.fields}
            />
          ) : (
            <div className="card">
              <p className="text-gray-600">Select a configuration section</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
