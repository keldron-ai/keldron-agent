type Severity = "healthy" | "elevated" | "warning" | "critical" | "offline"

interface LargeHexBadgeProps {
  score: number
  severity: Severity
  trendText?: string
}

export function LargeHexBadge({ score, severity, trendText }: LargeHexBadgeProps) {
  const getBgColor = () => {
    switch (severity) {
      case "healthy":
        return "#10B981"
      case "elevated":
        return "#00C9B0"
      case "warning":
        return "#F59E0B"
      case "critical":
        return "#EF4444"
      case "offline":
        return "#1E293B"
      default:
        return "#1E293B"
    }
  }

  const getTextColor = () => {
    if (severity === "critical") return "#FFFFFF"
    if (severity === "offline") return "#475569"
    return "#0A0C10"
  }

  const getSeverityLabel = () => {
    switch (severity) {
      case "healthy":
        return "Healthy"
      case "elevated":
        return "Elevated"
      case "warning":
        return "Warning"
      case "critical":
        return "Critical"
      case "offline":
        return "Offline"
      default:
        return ""
    }
  }

  const getLabelColor = () => {
    switch (severity) {
      case "healthy":
        return "#10B981"
      case "elevated":
        return "#00C9B0"
      case "warning":
        return "#F59E0B"
      case "critical":
        return "#EF4444"
      case "offline":
        return "#475569"
      default:
        return "#475569"
    }
  }

  const getGlowClass = () => {
    switch (severity) {
      case "healthy":
        return "animate-glow-healthy"
      case "elevated":
        return "animate-glow-elevated"
      case "warning":
        return "animate-glow-warning-large"
      case "critical":
        return "animate-glow-critical"
      default:
        return ""
    }
  }

  return (
    <div className="flex flex-col items-center">
      <div
        className={`w-24 h-[84px] flex items-center justify-center ${getGlowClass()}`}
        style={{
          backgroundColor: getBgColor(),
          clipPath: "polygon(25% 0%, 75% 0%, 100% 50%, 75% 100%, 25% 100%, 0% 50%)",
        }}
      >
        <span
          className="text-[32px] font-bold"
          style={{ color: getTextColor() }}
        >
          {score}
        </span>
      </div>
      <span
        className="mt-2 text-sm font-semibold"
        style={{ color: getLabelColor() }}
      >
        {getSeverityLabel()}
      </span>
      {trendText && (
        <span className="mt-1 text-xs text-[#94A3B8]">
          {trendText}
        </span>
      )}
    </div>
  )
}
