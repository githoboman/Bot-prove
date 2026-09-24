/**
 * Lightweight toast notification system.
 * Bottom-right, auto-dismiss, dismissible.
 */
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
} from 'react'
import type { ReactNode } from 'react'
import { CheckCircle2, XCircle, Info, X } from 'lucide-react'

type ToastKind = 'success' | 'error' | 'info'

interface ToastItem {
  id: number
  kind: ToastKind
  message: string
}

export interface ToastApi {
  success: (message: string) => void
  error: (message: string) => void
  info: (message: string) => void
}

const ToastContext = createContext<ToastApi | undefined>(undefined)

const KIND_STYLES: Record<ToastKind, { icon: typeof CheckCircle2; cls: string }> = {
  success: {
    icon: CheckCircle2,
    cls: 'border-green-500/40 bg-green-950/90 text-green-100',
  },
  error: {
    icon: XCircle,
    cls: 'border-red-500/40 bg-red-950/90 text-red-100',
  },
  info: {
    icon: Info,
    cls: 'border-gray-500/40 bg-[#1a1a2a]/95 text-gray-100',
  },
}

const AUTO_DISMISS_MS = 4500

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])
  const idRef = useRef(0)

  const dismiss = useCallback((id: number) => {
    setItems((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const push = useCallback(
    (kind: ToastKind, message: string) => {
      const id = ++idRef.current
      setItems((prev) => [...prev, { id, kind, message }])
      window.setTimeout(() => dismiss(id), AUTO_DISMISS_MS)
    },
    [dismiss],
  )

  const api = useMemo<ToastApi>(
    () => ({
      success: (m) => push('success', m),
      error: (m) => push('error', m),
      info: (m) => push('info', m),
    }),
    [push],
  )

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className="fixed bottom-4 right-4 z-[100] flex flex-col gap-2 w-[calc(100vw-2rem)] max-w-sm pointer-events-none">
        {items.map((t) => {
          const { icon: Icon, cls } = KIND_STYLES[t.kind]
          return (
            <div
              key={t.id}
              role="status"
              className={`pointer-events-auto flex items-start gap-2.5 rounded-xl border px-4 py-3 text-sm shadow-xl backdrop-blur animate-toast-in ${cls}`}
            >
              <Icon className="h-[18px] w-[18px] shrink-0 mt-0.5" />
              <p className="flex-1 leading-snug break-words">{t.message}</p>
              <button
                type="button"
                onClick={() => dismiss(t.id)}
                aria-label="Dismiss notification"
                className="shrink-0 opacity-60 hover:opacity-100"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          )
        })}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext)
  if (!ctx)
    throw new Error('useToast() must be used within a ToastProvider')
  return ctx
}

/* ---------- singleton for non-React callers ---------- */
let _singletonApi: ToastApi | null = null

export function registerToastApi(api: ToastApi) {
  _singletonApi = api
}

/** Imperative toast usable outside React tree. Falls back to console. */
export const toast: ToastApi = {
  success: (m) => (_singletonApi ? _singletonApi.success(m) : console.info(`✅ ${m}`)),
  error: (m) => (_singletonApi ? _singletonApi.error(m) : console.error(`❌ ${m}`)),
  info: (m) => (_singletonApi ? _singletonApi.info(m) : console.info(`ℹ️ ${m}`)),
}
