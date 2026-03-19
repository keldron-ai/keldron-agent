import { useParams, Link } from 'react-router-dom'

export function DeviceDetail() {
  const { id } = useParams<{ id: string }>()

  return (
    <div className="flex-1 p-6 space-y-4 overflow-auto">
      <div className="flex items-center gap-4 mb-4">
        <Link
          to="/"
          className="text-sm text-[#94A3B8] hover:text-[#E8ECF4] transition-colors"
        >
          &larr; Back
        </Link>
        <h1 className="text-[18px] font-semibold text-[#E8ECF4]">
          Device: {id}
        </h1>
      </div>

      <section className="bg-[#0F172A] rounded-xl border border-white/[0.06] p-6">
        <p className="text-[13px] text-[#94A3B8]">
          Device detail view coming soon.
        </p>
      </section>
    </div>
  )
}
