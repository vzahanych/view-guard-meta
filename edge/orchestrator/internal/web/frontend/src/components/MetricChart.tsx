import { LineChart, Line, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import Card from './Card'

interface MetricChartProps {
  title: string
  data: Array<{ time: string; value: number }>
  type?: 'line' | 'bar'
  color?: string
  unit?: string
}

export default function MetricChart({
  title,
  data,
  type = 'line',
  color = '#3b82f6',
  unit = '',
}: MetricChartProps) {
  const chartData = data.map((item) => ({
    time: new Date(item.time).toLocaleTimeString(),
    value: item.value,
  }))

  return (
    <Card>
      <h3 className="text-lg font-semibold text-gray-900 mb-4">{title}</h3>
      <ResponsiveContainer width="100%" height={200}>
        {type === 'line' ? (
          <LineChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="time" />
            <YAxis />
            <Tooltip
              formatter={(value: number) => [`${value}${unit}`, 'Value']}
              labelFormatter={(label) => `Time: ${label}`}
            />
            <Line type="monotone" dataKey="value" stroke={color} strokeWidth={2} dot={false} />
          </LineChart>
        ) : (
          <BarChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="time" />
            <YAxis />
            <Tooltip
              formatter={(value: number) => [`${value}${unit}`, 'Value']}
              labelFormatter={(label) => `Time: ${label}`}
            />
            <Bar dataKey="value" fill={color} />
          </BarChart>
        )}
      </ResponsiveContainer>
    </Card>
  )
}

