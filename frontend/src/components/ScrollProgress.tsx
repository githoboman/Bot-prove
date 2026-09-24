import { useEffect, useState } from 'react'
export default function ScrollProgress() {
  const [p, setP] = useState(0)
  useEffect(() => {
    const h = () => { const t = document.documentElement.scrollHeight - window.innerHeight; setP(t > 0 ? (window.scrollY / t) * 100 : 0) }
    window.addEventListener('scroll', h, { passive: true })
    return () => window.removeEventListener('scroll', h)
  }, [])
  return <div className="fixed top-0 left-0 right-0 z-[60] h-0.5"><div className="h-full bg-gradient-to-r from-red-600 to-orange-500 transition-[width] duration-75" style={{ width: `${p}%` }} /></div>
}
