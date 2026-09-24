import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';

/**
 * Global keyboard-shortcut hook for the Proof Lab.
 *
 * - `?` (shift + /)  -> open the help modal (via onHelp)
 * - `g` then a target letter -> navigate:
 *     o overview, p proofs, m models, a aggregation, z zk-proofs,
 *     q pq-crypto, c contracts, l playground, k kyc
 *
 * Ignores key events fired inside inputs, textareas, and contentEditable.
 */
export function useKeyboardShortcuts(onHelp: () => void) {
  const navigate = useNavigate();

  useEffect(() => {
    let awaitingG = false;
    let awaitingGTimer: number | null = null;

    const clearG = () => {
      awaitingG = false;
      if (awaitingGTimer !== null) {
        window.clearTimeout(awaitingGTimer);
        awaitingGTimer = null;
      }
    };

    const routeMap: Record<string, string> = {
      o: '/lab/overview',
      p: '/lab/proofs',
      m: '/lab/models',
      a: '/lab/aggregation',
      z: '/lab/zk-proofs',
      q: '/lab/pq-crypto',
      c: '/lab/contracts',
      l: '/lab/playground',
      k: '/lab/kyc',
    };

    const isTypingContext = (t: EventTarget | null): boolean => {
      const el = t as HTMLElement | null;
      if (!el) return false;
      const tag = el.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
      if (el.isContentEditable) return true;
      return false;
    };

    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (isTypingContext(e.target)) return;

      // Help
      if (e.key === '?' || (e.shiftKey && e.key === '/')) {
        e.preventDefault();
        clearG();
        onHelp();
        return;
      }

      if (awaitingG) {
        const target = routeMap[e.key.toLowerCase()];
        clearG();
        if (target) {
          e.preventDefault();
          navigate(target);
        }
        return;
      }

      if (e.key === 'g') {
        awaitingG = true;
        awaitingGTimer = window.setTimeout(clearG, 1200);
      }
    };

    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('keydown', onKey);
      clearG();
    };
  }, [navigate, onHelp]);
}
