import React, { useEffect } from 'react';
import { X, Keyboard } from 'lucide-react';

interface Props {
  open: boolean;
  onClose: () => void;
}

const rows: { keys: string; action: string }[] = [
  { keys: '?', action: 'Show this shortcut list' },
  { keys: 'g o', action: 'Go to Overview' },
  { keys: 'g p', action: 'Go to Proofs' },
  { keys: 'g m', action: 'Go to Models' },
  { keys: 'g a', action: 'Go to Aggregation' },
  { keys: 'g z', action: 'Go to ZK Proofs' },
  { keys: 'g q', action: 'Go to PQ Crypto' },
  { keys: 'g c', action: 'Go to Contracts' },
  { keys: 'g l', action: 'Go to Playground' },
  { keys: 'g k', action: 'Go to KYC' },
];

export default function KeyboardHelpModal({ open, onClose }: Props) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 bg-black/60 flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-label="Keyboard shortcuts"
      onClick={onClose}
    >
      <div
        className="bg-[#13131d] border border-[#222235] rounded-lg shadow-xl max-w-md w-full"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-4 border-b border-[#222235]">
          <h3 className="text-lg font-semibold text-gray-100 flex items-center gap-2">
            <Keyboard className="w-5 h-5 text-gray-400" />
            Keyboard shortcuts
          </h3>
          <button
            onClick={onClose}
            className="p-1 text-gray-400 hover:text-gray-200"
            aria-label="Close"
          >
            <X className="w-5 h-5" />
          </button>
        </div>
        <ul className="p-4 space-y-2">
          {rows.map((r) => (
            <li key={r.keys} className="flex items-center justify-between text-sm">
              <span className="text-gray-300">{r.action}</span>
              <kbd className="font-mono text-xs px-2 py-0.5 rounded border border-gray-700 bg-[#0b0b10] text-gray-200">
                {r.keys}
              </kbd>
            </li>
          ))}
        </ul>
        <div className="px-4 pb-4 text-[11px] text-gray-500">
          Press <kbd className="font-mono">Esc</kbd> to close. Shortcuts are ignored while typing in inputs.
        </div>
      </div>
    </div>
  );
}
