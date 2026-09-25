import { useState, useEffect } from 'react'
import { Menu, X } from 'lucide-react'
import { useLocation } from 'react-router-dom'
import { useWallet } from '../lib/WalletProvider'
import { shortKey } from '../lib/wallet'

interface Props { mobileOpen: boolean; setMobileOpen: (v: boolean) => void }

const links = [
  { href: '/#home', label: 'Home' },
  { href: '/#features', label: 'Features' },
  { href: '/#how', label: 'Pipeline' },
  { href: '/#use-cases', label: 'Use Cases' },
  { href: '/#demo', label: 'Demo' },
  { href: '/#sdk', label: 'SDK' },
  { href: '/#faq', label: 'FAQ' },
]

export default function Navbar({ mobileOpen, setMobileOpen }: Props) {
  const [scrolled, setScrolled] = useState(false)
  const location = useLocation()
  const isLab = location.pathname.startsWith('/lab')

  useEffect(() => {
    const handler = () => setScrolled(window.scrollY > 50)
    window.addEventListener('scroll', handler, { passive: true })
    return () => window.removeEventListener('scroll', handler)
  }, [])

  // Lab has its own header — don't render Navbar on /lab pages
  if (isLab) return null

  const handleNavClick = (e: React.MouseEvent<HTMLAnchorElement>, href: string) => {
    e.preventDefault()
    const hash = href.split('#')[1]
    if (location.pathname !== '/') {
      window.location.href = href
    } else {
      const el = document.getElementById(hash)
      if (el) el.scrollIntoView({ behavior: 'smooth' })
    }
  }

  const handleLogoClick = (e: React.MouseEvent<HTMLAnchorElement>) => {
    if (location.pathname === '/') {
      e.preventDefault()
      window.scrollTo({ top: 0, behavior: 'smooth' })
    }
  }

  return (
    <nav className={`fixed top-0 left-0 right-0 z-50 transition-all duration-300 ${scrolled ? 'bg-black/90 backdrop-blur border-b border-gray-800/50' : 'bg-transparent'}`}>
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 flex items-center justify-between h-16">
        <a href="/" onClick={handleLogoClick} className="flex items-center gap-2">
          <img src="/images/logo.jpg" alt="Bot Prove" className="h-7 w-auto" />
          <span className="font-bold text-white text-lg hidden sm:block">Bot Prove</span>
        </a>
        {!isLab && (
          <div className="hidden md:flex items-center gap-1">
            {links.map(l => (
              <a key={l.href} href={l.href} onClick={(e) => handleNavClick(e, l.href)} className="px-3 py-2 text-sm font-medium text-gray-400 hover:text-red-400 rounded-lg transition-colors">{l.label}</a>
            ))}
          </div>
        )}
        <div className="flex items-center gap-2">
          {/* No wallet button on landing — wallet connect is in Lab only */}
          {!isLab && (
            <a href="/lab" className="hidden sm:inline-flex items-center gap-2 px-5 py-2 rounded-full bg-red-600 text-white text-sm font-semibold hover:bg-red-500 transition-colors">
              Proof Lab
            </a>
          )}
          <button onClick={() => setMobileOpen(!mobileOpen)} className="md:hidden p-2 text-gray-400">
            {mobileOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
          </button>
        </div>
      </div>
      {mobileOpen && (
        <div className="md:hidden bg-black/95 border-t border-gray-800">
          <div className="px-4 py-3 space-y-1">
            {links.map(l => (
              <a key={l.href} href={l.href} onClick={(e) => { handleNavClick(e, l.href); setMobileOpen(false) }} className="block px-3 py-2.5 text-gray-300 rounded-lg hover:bg-white/5">{l.label}</a>
            ))}
            {!isLab && (
              <a href="/lab" onClick={() => setMobileOpen(false)} className="block px-3 py-2.5 text-red-400 font-semibold rounded-lg hover:bg-white/5">Proof Lab</a>
            )}
          </div>
        </div>
      )}
    </nav>
  )
}
