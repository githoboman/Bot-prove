import { ReactNode } from 'react';

interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: { label: string; onClick: () => void };
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="text-center py-12 px-6 rounded-xl border border-dashed border-gray-700 bg-gray-900/30">
      {icon && <div className="mx-auto mb-4 text-gray-500 flex justify-center">{icon}</div>}
      <h3 className="text-lg font-semibold text-gray-200 mb-2">{title}</h3>
      {description && <p className="text-sm text-gray-400 mb-4 max-w-md mx-auto">{description}</p>}
      {action && (
        <button
          onClick={action.onClick}
          className="mt-2 px-4 py-2 rounded-lg bg-purple-600 hover:bg-purple-500 text-white text-sm font-medium transition-colors"
        >
          {action.label}
        </button>
      )}
    </div>
  );
}
