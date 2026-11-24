import './Charts.css'

// Uma linha pequena (ex.: imagens por versão), sem eixos.
export function Sparkline({ values, width = 120, height = 32 }: { values: number[]; width?: number; height?: number }) {
  if (values.length < 2) return null
  const max = Math.max(...values)
  const min = Math.min(...values)
  const span = max - min || 1
  const step = width / (values.length - 1)
  const points = values.map((value, i) => [i * step, height - 3 - ((value - min) / span) * (height - 6)] as const)
  const line = points.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`).join(' ')
  const area = `${line} L${width},${height} L0,${height} Z`
  const [lx, ly] = points[points.length - 1]
  return (
    <svg aria-hidden="true" className="sparkline" height={height} viewBox={`0 0 ${width} ${height}`} width={width}>
      <path className="sparkline__area" d={area} />
      <path className="sparkline__line" d={line} />
      <circle className="sparkline__dot" cx={lx} cy={ly} r="2.5" />
    </svg>
  )
}
