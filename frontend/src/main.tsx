import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { ToastProvider } from './lib/toast'
import ToastBridge from './lib/ToastBridge'
import App from './App'
import { installDevConsoleInfo } from './lib/devConsole'
import './index.css'

// Read-only devtools banner: contract status + /health snapshot + repo
// links. See lib/devConsole.ts.
installDevConsoleInfo()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <ToastProvider>
        <ToastBridge />
        <App />
      </ToastProvider>
    </BrowserRouter>
  </StrictMode>
)

// Register the service worker for PWA offline-first behavior (backlog 9.3/9.4).
// SW itself lives in /public/sw.js; only production builds register.
if ('serviceWorker' in navigator && import.meta.env.PROD) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(err => {
      // Non-fatal — the app must still work without a SW.
      console.warn('SW registration failed', err)
    })
  })
}
