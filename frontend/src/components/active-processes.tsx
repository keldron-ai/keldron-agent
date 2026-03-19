interface Process {
  name: string
  detail: string
  gpuMemory: string
  gpuPercent: string
  runtime: string
  isHighUsage?: boolean
}

interface ActiveProcessesProps {
  processes: Process[]
}

export function ActiveProcesses({ processes }: ActiveProcessesProps) {
  return (
    <div className="bg-[#0F172A] rounded-xl border border-white/[0.06] p-5 hover:border-white/[0.12] transition-colors h-full">
      <h3 className="text-[13px] font-semibold text-[#E8ECF4] mb-4">Active Processes</h3>
      <table className="w-full">
        <thead>
          <tr className="text-[11px] text-[#64748B] uppercase">
            <th className="text-left font-normal pb-3">Process</th>
            <th className="text-right font-normal pb-3">GPU Memory</th>
            <th className="text-right font-normal pb-3">GPU %</th>
            <th className="text-right font-normal pb-3">Runtime</th>
          </tr>
        </thead>
        <tbody>
          {processes.map((process, index) => (
            <tr
              key={index}
              className="border-t border-white/[0.04]"
            >
              <td className="py-2.5">
                <span className="text-[13px] text-[#E8ECF4]">{process.name}</span>
                {process.detail && (
                  <span className="text-[13px] text-[#94A3B8]"> ({process.detail})</span>
                )}
              </td>
              <td className="text-right text-[13px] text-[#E8ECF4] py-2.5">
                {process.gpuMemory}
              </td>
              <td
                className={`text-right text-[13px] py-2.5 ${
                  process.isHighUsage ? "text-[#F59E0B]" : "text-[#E8ECF4]"
                }`}
              >
                {process.gpuPercent}
              </td>
              <td className="text-right text-[13px] text-[#E8ECF4] py-2.5">
                {process.runtime}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
