import { useState } from 'react';
import { Check, Copy } from 'lucide-react';

interface CopyButtonProps {
  text: string;
  className?: string;
  size?: number;
}

export function CopyButton({ text, className, size = 14 }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback for browsers without clipboard API
      const el = document.createElement('textarea');
      el.value = text;
      el.style.position = 'fixed';
      el.style.opacity = '0';
      document.body.appendChild(el);
      el.select();
      try {
        document.execCommand('copy');
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      } catch {
        /* ignore */
      }
      document.body.removeChild(el);
    }
  };

  return (
    <button
      onClick={copy}
      title={copied ? 'Copied!' : 'Copy'}
      aria-label={copied ? 'Copied' : 'Copy to clipboard'}
      className={`inline-flex items-center gap-1 text-gray-400 hover:text-purple-400 transition-colors ${className || ''}`}
      type="button"
    >
      {copied ? <Check size={size} className="text-green-400" /> : <Copy size={size} />}
    </button>
  );
}
