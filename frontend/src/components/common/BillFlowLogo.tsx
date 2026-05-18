import { cn } from '@/lib/utils'

interface BillFlowLogoProps {
  className?: string
}

export function BillFlowLogo({ className }: BillFlowLogoProps) {
  return (
    <img
      src="/billflow-mark.svg"
      alt="BillFlow"
      className={cn('h-8 w-8 shrink-0 rounded-lg shadow-sm', className)}
      draggable={false}
    />
  )
}
