import { billSourceLabel } from '@/lib/labels'
import { cn } from '@/lib/utils'

interface Props {
  source?: string | null
  label?: string
  className?: string
}

export function BillSourceBadge({ source, label, className }: Props) {
  const tone = sourceBadgeTone(source)

  return (
    <span
      className={cn(
        'inline-flex w-fit min-w-0 items-center gap-1.5 rounded-full border px-2 py-1 text-xs font-medium transition-colors',
        tone.className,
        className,
      )}
    >
      <SourceAccent source={source} tone={tone} />
      <span className="min-w-0 truncate">{label ?? billSourceLabel(source)}</span>
    </span>
  )
}

function SourceAccent({
  source,
  tone,
}: {
  source?: string | null
  tone: ReturnType<typeof sourceBadgeTone>
}) {
  if (source === 'lazada_email') {
    return (
      <span className="inline-flex shrink-0 items-center gap-0.5" aria-hidden="true">
        <span className="h-1.5 w-1.5 rounded-full bg-[#ff6a00]" />
        <span className="h-1.5 w-1.5 rounded-full bg-[#f31c9b]" />
      </span>
    )
  }

  return <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full', tone.dotClass)} aria-hidden="true" />
}

function sourceBadgeTone(source?: string | null) {
  switch (source) {
    case 'shopee_shipped':
      return {
        className:
          'border-[#ee4d2d]/30 bg-[#ee4d2d]/10 text-[#a83a21] hover:bg-[#ee4d2d]/15 dark:border-[#ff7a59]/40 dark:bg-[#ee4d2d]/15 dark:text-[#ffb199] dark:hover:bg-[#ee4d2d]/20',
        dotClass: 'bg-[#ee4d2d] dark:bg-[#ff7a59]',
      }
    case 'lazada_email':
      return {
        className:
          'border-[#1a2a7b]/25 bg-[#1a2a7b]/10 text-[#1a2a7b] hover:bg-[#1a2a7b]/15 dark:border-[#f31c9b]/35 dark:bg-[#17245f]/70 dark:text-[#b9c4ff] dark:hover:bg-[#1d2f7a]/75',
        dotClass: 'bg-[#1a2a7b] dark:bg-[#b9c4ff]',
      }
    default:
      return {
        className: 'border-transparent bg-muted text-muted-foreground hover:bg-muted',
        dotClass: 'bg-muted-foreground/50',
      }
  }
}
