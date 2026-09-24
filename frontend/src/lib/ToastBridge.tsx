/**
 * Bridges the React ToastProvider context to the imperative `toast` singleton,
 * so components using `import { toast } from '../ui/toast'` get real UI toasts.
 */
import { useEffect } from 'react'
import { useToast, registerToastApi } from './toast'

export default function ToastBridge() {
  const api = useToast()
  useEffect(() => {
    registerToastApi(api)
  }, [api])
  return null
}
