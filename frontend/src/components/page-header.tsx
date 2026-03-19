import { Link } from "react-router-dom"

interface PageHeaderProps {
  title: string
  backLink?: {
    href: string
    label: string
  }
  rightContent?: React.ReactNode
}

export function PageHeader({ title, backLink, rightContent }: PageHeaderProps) {
  return (
    <header className="h-14 bg-[#0F172A] border-b border-white/[0.06] flex items-center justify-between px-6 shrink-0">
      {/* Left: Back link */}
      <div className="flex items-center gap-4 min-w-[140px]">
        {backLink && (
          <Link
            to={backLink.href}
            className="text-sm text-[#94A3B8] hover:text-[#E8ECF4] transition-colors"
          >
            ← {backLink.label}
          </Link>
        )}
      </div>

      {/* Center: Title with logo */}
      <div className="flex items-center gap-2">
        <svg
          width="24"
          height="24"
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
        <span className="text-[15px] font-medium text-[#E8ECF4]">{title}</span>
      </div>

      {/* Right */}
      <div className="flex items-center gap-4 min-w-[140px] justify-end">
        {rightContent || (
          <span className="text-[11px] text-[#64748B]">v0.1.0 · Local mode</span>
        )}
      </div>
    </header>
  )
}
