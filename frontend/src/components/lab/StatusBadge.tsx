import React from 'react';

export type StatusBadgeColor = 'green' | 'blue' | 'yellow' | 'red' | 'gray';

interface StatusBadgeProps {
  label: string;
  color?: StatusBadgeColor;
  className?: string;
  title?: string;
}

const colors: Record<StatusBadgeColor, string> = {
  green: 'bg-green-900/30 text-green-400 border-green-700/40',
  blue: 'bg-blue-900/30 text-blue-400 border-blue-700/40',
  yellow: 'bg-yellow-900/30 text-yellow-400 border-yellow-700/40',
  red: 'bg-red-900/30 text-red-400 border-red-700/40',
  gray: 'bg-gray-800/40 text-gray-300 border-gray-700/40',
};

/**
 * Small pill for status / provenance labels.
 * "Live data", "Testnet", "Simulated", "Deprecated", etc.
 */
export default function StatusBadge({ label, color = 'green', className = '', title }: StatusBadgeProps) {
  return (
    <span
      className={`text-[10px] px-1.5 py-0.5 rounded-full border font-medium ${colors[color]} ${className}`}
      title={title}
    >
      {label}
    </span>
  );
}
