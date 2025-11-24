import Card from './Card'

interface MetricCardProps {
  title: string
  value: string | number
  subtitle?: string
  percentage?: number
  trend?: 'up' | 'down' | 'neutral'
  className?: string
}

export default function MetricCard({
  title,
  value,
  subtitle,
  percentage,
  trend,
  className = '',
}: MetricCardProps) {
  const getPercentageColor = (percent?: number) => {
    if (percent === undefined) return 'bg-gray-200'
    if (percent < 50) return 'bg-green-500'
    if (percent < 80) return 'bg-yellow-500'
    return 'bg-red-500'
  }

  return (
    <Card className={className}>
      <div>
        <p className="text-sm font-medium text-gray-600 mb-1">{title}</p>
        <div className="flex items-baseline justify-between">
          <p className="text-3xl font-bold text-gray-900">{value}</p>
          {trend && (
            <span
              className={`text-xs ${
                trend === 'up' ? 'text-green-600' : trend === 'down' ? 'text-red-600' : 'text-gray-600'
              }`}
            >
              {trend === 'up' ? '↑' : trend === 'down' ? '↓' : '→'}
            </span>
          )}
        </div>
        {subtitle && <p className="text-sm text-gray-500 mt-1">{subtitle}</p>}
        {percentage !== undefined && (
          <div className="mt-3">
            <div className="w-full bg-gray-200 rounded-full h-2">
              <div
                className={`h-2 rounded-full transition-all ${getPercentageColor(percentage)}`}
                style={{ width: `${Math.min(percentage, 100)}%` }}
              />
            </div>
            <p className="text-xs text-gray-500 mt-1">{percentage.toFixed(1)}%</p>
          </div>
        )}
      </div>
    </Card>
  )
}

