import { ReactNode } from 'react'

interface CardProps {
  title?: string
  children: ReactNode
  className?: string
  headerActions?: ReactNode
  dataTestId?: string
}

export default function Card({ title, children, className = '', headerActions, dataTestId }: CardProps) {
  return (
    <div className={`card ${className}`} data-testid={dataTestId}>
      {title && (
        <div className="card-header flex items-center justify-between">
          <h3 className="text-lg font-semibold text-gray-900">{title}</h3>
          {headerActions && <div>{headerActions}</div>}
        </div>
      )}
      <div>{children}</div>
    </div>
  )
}

