import { Link } from "react-router-dom"
import { HelpCircle } from "lucide-react"

export function Header() {
  return (
    <header className="h-14 bg-[#0F172A] border-b border-white/[0.06] flex items-center justify-between px-6 shrink-0">
      {/* Left: Brand */}
      <div className="flex items-center gap-2">
        <Link to="/" className="flex items-center gap-2">
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
        </Link>
      </div>

      {/* Center: Fleet summary */}
      <div className="hidden sm:flex items-center gap-1.5 text-sm text-[#94A3B8]">
        <span className="w-1.5 h-1.5 rounded-full bg-[#10B981] animate-live-pulse" />
        <span>6 devices monitored</span>
        <span>·</span>
        <span>Fleet health:</span>
        <svg
          width="24"
          height="24"
          viewBox="0 0 24 24"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
          className="mx-0.5"
        >
          <polygon
            points="6,0 18,0 24,12 18,24 6,24 0,12"
            fill="#F59E0B"
          />
          <text
            x="12"
            y="16"
            textAnchor="middle"
            fill="#0A0C10"
            fontSize="10"
            fontWeight="700"
            fontFamily="system-ui, sans-serif"
          >
            67
          </text>
        </svg>
        <span>·</span>
        <span className="inline-flex items-center gap-1.5">
          <span className="w-2 h-2 rounded-full bg-[#EF4444]" />
          <span className="text-[#64748B]">1 offline</span>
        </span>
      </div>

      {/* Right: Actions */}
      <div className="flex items-center gap-4">
        <button
          className="text-[#64748B] hover:text-[#E8ECF4] transition-colors"
          title="Help"
        >
          <HelpCircle size={18} />
        </button>
        <div className="flex flex-col items-center">
          <div
            className="w-7 h-7 rounded-full flex items-center justify-center text-[13px] font-semibold hover:ring-2 hover:ring-[#00C9B0]/30 transition-all"
            style={{ backgroundColor: "rgba(0, 201, 176, 0.15)", color: "#00C9B0" }}
          >
            R
          </div>
          <span className="text-[10px] text-[#94A3B8]">Free</span>
        </div>
      </div>
    </header>
  )
}
