export function Skeleton({ className }: { className?: string }) {
  return <div className={`animate-pulse bg-gray-700/50 rounded ${className || 'h-4 w-full'}`} />;
}

export function CardSkeleton() {
  return (
    <div className="p-6 rounded-xl border border-gray-700 space-y-3">
      <Skeleton className="h-5 w-1/3" />
      <Skeleton className="h-4 w-2/3" />
      <Skeleton className="h-4 w-1/2" />
    </div>
  );
}

export function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="space-y-2">
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}
