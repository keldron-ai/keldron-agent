type SparklineType = "flat" | "moderate" | "rising" | "ramping" | "ceiling" | "bursty" | "idle" | "offline"

interface SparklineProps {
  type: SparklineType
  warning?: boolean
  offline?: boolean
}

function getPoints(type: SparklineType): string {
  switch (type) {
    case "flat":
      return "0,14 10,15 20,14 30,15 40,14 50,15 60,14 70,15 80,14"
    case "idle":
      return "0,16 10,17 20,16 30,17 40,16 50,17 60,16 70,17 80,16"
    case "moderate":
      return "0,10 10,8 20,11 30,7 40,9 50,8 60,10 70,7 80,9"
    case "rising":
      return "0,14 10,12 20,13 30,10 40,11 50,9 60,8 70,7 80,6"
    case "ramping":
      return "0,16 10,14 20,12 30,10 40,8 50,6 60,5 70,4 80,3"
    case "ceiling":
      return "0,4 10,3 20,5 30,3 40,4 50,3 60,5 70,3 80,4"
    case "bursty":
      return "0,4 10,3 20,10 30,4 40,3 50,11 60,4 70,3 80,5"
    case "offline":
      return "0,9 80,9"
    default:
      return "0,9 80,9"
  }
}

function getAreaPath(type: SparklineType): string {
  const points = getPoints(type).split(" ").map(p => {
    const [x, y] = p.split(",").map(Number)
    return { x, y }
  })
  
  if (points.length < 2) return ""
  
  let path = `M ${points[0].x},${points[0].y}`
  for (let i = 1; i < points.length; i++) {
    path += ` L ${points[i].x},${points[i].y}`
  }
  path += ` L 80,18 L 0,18 Z`
  return path
}

export function Sparkline({ type, warning, offline }: SparklineProps) {
  const strokeColor = offline
    ? "#475569"
    : warning
    ? "#F59E0B"
    : "#00C9B0"

  const gradientId = `sparkline-${warning ? "warning" : "teal"}-${Math.random().toString(36).slice(2, 8)}`
  const fillGradientId = `sparkline-fill-${warning ? "warning" : "teal"}-${Math.random().toString(36).slice(2, 8)}`

  if (offline) {
    return (
      <svg
        width="80"
        height="18"
        viewBox="0 0 80 18"
        preserveAspectRatio="none"
        className="block"
      >
        <polyline
          points={getPoints(type)}
          fill="none"
          stroke={strokeColor}
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    )
  }

  return (
    <svg
      width="80"
      height="18"
      viewBox="0 0 80 18"
      preserveAspectRatio="none"
      className="block"
    >
      <defs>
        {/* Horizontal gradient for line fade (older = faded, newer = bright) */}
        <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stopColor={strokeColor} stopOpacity="0.3" />
          <stop offset="100%" stopColor={strokeColor} stopOpacity="1" />
        </linearGradient>
        {/* Vertical gradient for area fill */}
        <linearGradient id={fillGradientId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={strokeColor} stopOpacity="0.15" />
          <stop offset="100%" stopColor={strokeColor} stopOpacity="0" />
        </linearGradient>
      </defs>
      
      {/* Area fill beneath the line */}
      <path
        d={getAreaPath(type)}
        fill={`url(#${fillGradientId})`}
      />
      
      {/* The sparkline itself with trailing brightness */}
      <polyline
        points={getPoints(type)}
        fill="none"
        stroke={`url(#${gradientId})`}
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
