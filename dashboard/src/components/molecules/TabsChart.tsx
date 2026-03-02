import { useMemo } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts'

export interface TabDataPoint {
  timestamp: number
  [instanceId: string]: number // tab count per instance
}

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

  if (data.length === 0) {
    return (
      <div className="flex h-[200px] items-center justify-center rounded-lg border border-border-subtle bg-bg-surface text-sm text-text-muted">
        No data yet — waiting for instances...
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-border-subtle bg-bg-surface p-4">
      <ResponsiveContainer width="100%" height={200}>
        <LineChart data={data}>
          <XAxis
            dataKey="timestamp"
            tickFormatter={formatTime}
            stroke="#666"
            fontSize={11}
          />
          <YAxis
            stroke="#666"
            fontSize={11}
            allowDecimals={false}
            domain={[0, 'auto']}
          />
          <Tooltip
            contentStyle={{
              background: '#1a1a1a',
              border: '1px solid #333',
              borderRadius: '6px',
              fontSize: '12px',
            }}
            labelFormatter={(label) => formatTime(label as number)}
          />
          <Legend
            onClick={(e) => {
              const id = e.dataKey as string
              onSelectInstance(id)
            }}
            wrapperStyle={{ cursor: 'pointer', fontSize: '12px' }}
          />
          {instances.map((inst) => (
            <Line
              key={inst.id}
              type="monotone"
              dataKey={inst.id}
              name={inst.profileName}
              stroke={instanceColors[inst.id]}
              strokeWidth={selectedInstanceId === inst.id ? 3 : 1.5}
              strokeOpacity={
                selectedInstanceId && selectedInstanceId !== inst.id ? 0.3 : 1
              }
              dot={false}
              activeDot={{ r: 4 }}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
