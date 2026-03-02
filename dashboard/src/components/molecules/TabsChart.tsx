import { useMemo } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import type { TabDataPoint } from '../../stores/useAppStore'

interface Props {
  data: TabDataPoint[]
  instances: { id: string; profileName: string }[]
  selectedInstanceId: string | null
  onSelectInstance: (id: string) => void
}

// Colors for different instances
const COLORS = [
  '#f97316', // orange (primary)
  '#3b82f6', // blue
  '#22c55e', // green
  '#eab308', // yellow
  '#ef4444', // red
  '#8b5cf6', // purple
  '#ec4899', // pink
  '#14b8a6', // teal
]

function formatTime(timestamp: number): string {
  return new Date(timestamp).toLocaleTimeString('en-GB', {
    hour: '2-digit',
    minute: '2-digit',
  })
}

export default function TabsChart({
  data,
  instances,
  selectedInstanceId,
  onSelectInstance,
}: Props) {
  const instanceColors = useMemo(() => {
    const colors: Record<string, string> = {}
    instances.forEach((inst, i) => {
      colors[inst.id] = COLORS[i % COLORS.length]
    })
    return colors
  }, [instances])

  if (data.length === 0 || instances.length === 0) {
    return (
      <div className="flex h-[200px] items-center justify-center rounded-lg border border-border-subtle bg-bg-surface text-sm text-text-muted">
        {instances.length === 0
          ? 'No running instances'
          : 'Collecting data...'}
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-border-subtle bg-bg-surface">
      <ResponsiveContainer width="100%" height={200}>
        <LineChart data={data} margin={{ top: 16, right: 16, bottom: 8, left: 8 }}>
          <XAxis
            dataKey="timestamp"
            tickFormatter={formatTime}
            stroke="#666"
            fontSize={11}
            tickLine={false}
            axisLine={false}
          />
          <YAxis
            stroke="#666"
            fontSize={11}
            allowDecimals={false}
            domain={[0, 'auto']}
            tickLine={false}
            axisLine={false}
            width={30}
          />
          <Tooltip
            contentStyle={{
              background: '#1a1a1a',
              border: '1px solid #333',
              borderRadius: '6px',
              fontSize: '12px',
            }}
            labelFormatter={(label) => formatTime(label as number)}
            formatter={(value, name) => {
              const inst = instances.find((i) => i.id === name)
              return [value ?? 0, inst?.profileName || name]
            }}
          />
          {instances.map((inst) => (
            <Line
              key={inst.id}
              type="monotone"
              dataKey={inst.id}
              name={inst.id}
              stroke={instanceColors[inst.id]}
              strokeWidth={selectedInstanceId === inst.id ? 3 : 1.5}
              strokeOpacity={
                selectedInstanceId && selectedInstanceId !== inst.id ? 0.3 : 1
              }
              dot={false}
              activeDot={{
                r: 4,
                onClick: () => onSelectInstance(inst.id),
                style: { cursor: 'pointer' },
              }}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
