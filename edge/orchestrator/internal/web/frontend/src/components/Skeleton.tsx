interface SkeletonProps {
  className?: string
  width?: string
  height?: string
}

function SkeletonBase({ className = '', width, height }: SkeletonProps) {
  return (
    <div
      className={`animate-pulse bg-gray-200 rounded ${className}`}
      style={{ width, height }}
      aria-hidden="true"
    />
  )
}

export default SkeletonBase

export function ScreenshotSkeleton() {
  return (
    <div className="bg-white rounded-lg border border-gray-200 p-4 shadow-sm">
      <div className="space-y-3">
        <div className="relative">
          <SkeletonBase className="w-full h-48" />
        </div>
        <div>
          <SkeletonBase className="h-4 w-24 mb-2" />
          <SkeletonBase className="h-4 w-32 mb-1" />
          <SkeletonBase className="h-3 w-20" />
        </div>
        <div className="flex gap-2">
          <SkeletonBase className="h-8 w-24" />
          <SkeletonBase className="h-8 w-20" />
          <SkeletonBase className="h-8 w-20" />
        </div>
      </div>
    </div>
  )
}
