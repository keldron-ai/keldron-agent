import { Link } from "react-router-dom"

interface DetailHeaderProps {
  deviceName: string
}

export function DetailHeader({ deviceName }: DetailHeaderProps) {
  return (
    <header className="h-14 bg-[#0F172A] border-b border-white/[0.06] flex items-center justify-between px-6 shrink-0">
      {/* Left: Back nav + Brand */}
      <div className="flex items-center gap-4">
        <Link
          to="/"
          className="text-sm text-[#94A3B8] hover:text-[#E8ECF4] transition-colors"
        >
          ← Fleet
        </Link>
        <div className="h-5 w-px bg-white/[0.06]" />
        <div className="flex items-center gap-2">
          <svg
            width="28"
            height="28"
            viewBox="0 0 28 28"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
          >
            <polygon
              points="7,0 21,0 28,14 21,28 7,28 0,14"
              fill="#00C9B0"
            />
            <text
              x="14"
              y="19"
              textAnchor="middle"
              fill="#0A0C10"
              fontSize="14"
              fontWeight="700"
              fontFamily="system-ui, sans-serif"
            >
              K
            </text>
          </svg>
          <span className="text-lg font-semibold text-[#E8ECF4]">Keldron</span>
        </div>
      </div>

      {/* Center: Breadcrumb */}
      <div className="hidden sm:flex items-center gap-1.5 text-sm text-[#94A3B8]">
        <Link to="/" className="hover:text-[#E8ECF4] transition-colors">
          Fleet
        </Link>
        <span>/</span>
        <span className="text-[#E8ECF4]">{deviceName}</span>
      </div>

      {/* Right: Cloud link */}
      <div className="flex flex-col items-end">
        <a
          href="#"
          className="text-sm text-[#00C9B0] hover:text-[#00E5CC] transition-colors"
        >
          Keldron Cloud →
        </a>
        <span className="text-[11px] text-[#64748B]">v0.1.0 · Local mode</span>
      </div>
    </header>
  )
}
