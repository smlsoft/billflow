import { type ReactNode } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

export interface DataTableColumn<T> {
  key: string
  header: ReactNode
  cell: (row: T, index: number) => ReactNode
  className?: string
  headerClassName?: string
  width?: string
}

export interface DataTableProps<T> {
  columns: DataTableColumn<T>[]
  data: T[]
  loading?: boolean
  loadingRows?: number
  empty?: ReactNode
  onRowClick?: (row: T) => void
  rowClassName?: string | ((row: T) => string)
  className?: string
  dense?: boolean
}

export function DataTable<T>({
  columns,
  data,
  loading,
  loadingRows = 8,
  empty,
  onRowClick,
  rowClassName,
  className,
  dense,
}: DataTableProps<T>) {
  const rowHeight = dense ? 'h-10' : 'h-12'

  return (
    <div className={cn('overflow-hidden rounded-lg border border-border bg-card', className)}>
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/40 hover:bg-muted/40">
            {columns.map((col) => (
              <TableHead
                key={col.key}
                className={cn('text-xs font-semibold uppercase tracking-wide text-muted-foreground', col.headerClassName)}
                style={col.width ? { width: col.width } : undefined}
              >
                {col.header}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? (
            Array.from({ length: loadingRows }).map((_, i) => (
              <TableRow key={`sk-${i}`} className={rowHeight}>
                {columns.map((col) => (
                  <TableCell key={col.key} className={col.className}>
                    <Skeleton className="h-4 w-full max-w-[180px]" />
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : data.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={columns.length}
                className="py-12 text-center text-sm text-muted-foreground"
              >
                {empty ?? 'ไม่พบข้อมูล'}
              </TableCell>
            </TableRow>
          ) : (
            data.map((row, i) => {
              const dynClass =
                typeof rowClassName === 'function'
                  ? rowClassName(row)
                  : rowClassName
              return (
                <TableRow
                  key={i}
                  className={cn(
                    rowHeight,
                    onRowClick && 'cursor-pointer hover:bg-muted/40',
                    dynClass,
                  )}
                  onClick={onRowClick ? () => onRowClick(row) : undefined}
                >
                  {columns.map((col) => (
                    <TableCell key={col.key} className={col.className}>
                      {col.cell(row, i)}
                    </TableCell>
                  ))}
                </TableRow>
              )
            })
          )}
        </TableBody>
      </Table>
    </div>
  )
}
