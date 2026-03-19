interface SystemInfoProps {
  info: {
    label: string
    value: string
  }[]
}

export function SystemInfo({ info }: SystemInfoProps) {
  return (
    <div className="bg-[#0F172A] rounded-xl border border-white/[0.06] p-5 hover:border-white/[0.12] transition-colors h-full">
      <h3 className="text-[13px] font-semibold text-[#E8ECF4] mb-4">System</h3>
      <div className="space-y-2.5">
        {info.map((item) => (
          <div key={item.label} className="flex items-center justify-between">
            <span className="text-xs text-[#64748B]">{item.label}</span>
            <span className="text-[13px] text-[#E8ECF4]">{item.value}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
