import { ReactNode } from 'react'

interface CardProps {
  title?: string
  children: ReactNode
  className?: string
  headerActions?: ReactNode
}

export default function Card({ title, children, className = '', headerActions }: CardProps) {
  return (
    <div className={`card ${className}`}>
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

