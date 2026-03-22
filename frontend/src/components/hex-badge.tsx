type Severity = "healthy" | "elevated" | "warning" | "critical" | "offline"

interface HexBadgeProps {
  score: number | null
  severity: Severity
}

export function HexBadge({ score, severity }: HexBadgeProps) {
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

  const getGlowClass = () => {
    switch (severity) {
      case "healthy":
        return "animate-glow-healthy"
      case "elevated":
        return "animate-glow-elevated"
      case "warning":
        return "animate-glow-warning"
      case "critical":
        return "animate-glow-critical"
      default:
        return ""
    }
  }

  return (
    <div
      className={`w-16 h-14 flex items-center justify-center ${getGlowClass()}`}
      style={{
        backgroundColor: getBgColor(),
        clipPath: "polygon(25% 0%, 75% 0%, 100% 50%, 75% 100%, 25% 100%, 0% 50%)",
      }}
    >
      <span
        className="text-[22px] font-bold"
        style={{ color: getTextColor() }}
      >
        {score !== null ? score : "—"}
      </span>
    </div>
  )
}
