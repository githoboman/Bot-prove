import React from 'react';
import { AlertTriangle, X } from 'lucide-react';

interface ConfirmModalProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
  variant?: 'danger' | 'warning';
}

const ConfirmModal: React.FC<ConfirmModalProps> = ({
  isOpen,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  onConfirm,
  onCancel,
  variant = 'danger',
}) => {
  if (!isOpen) return null;
  const isDanger = variant === 'danger';
  return (
    <div className="fixed inset-0 bg-black/75 flex items-center justify-center z-[60] p-4" onClick={onCancel}>
      <div
        className="bg-[#13131d] border border-[#222235] rounded-lg shadow-xl max-w-sm w-full"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 p-4 border-b border-[#222235]">
          <div className={`p-2 rounded-full ${isDanger ? 'bg-red-900/40' : 'bg-yellow-900/40'}`}>
            <AlertTriangle className={isDanger ? 'text-red-400' : 'text-yellow-400'} size={20} />
          </div>
          <h3 className="text-lg font-semibold text-gray-100 flex-1">{title}</h3>
          <button onClick={onCancel} className="text-gray-400 hover:text-gray-200 p-1">
            <X size={18} />
          </button>
        </div>
        <div className="p-4">
          <p className="text-sm text-gray-300">{message}</p>
        </div>
        <div className="flex justify-end gap-2 p-4 border-t border-[#222235]">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm text-gray-300 bg-[#1a1a2a] hover:bg-[#222235] border border-[#222235] rounded-lg transition-colors"
          >
            {cancelLabel}
          </button>
          <button
            onClick={onConfirm}
            className={`px-4 py-2 text-sm text-white rounded-lg transition-colors ${
              isDanger ? 'bg-red-600 hover:bg-red-700' : 'bg-yellow-600 hover:bg-yellow-700'
            }`}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
};

export default ConfirmModal;
