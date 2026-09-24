import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { useState, useEffect, useCallback } from 'react'
import CsprClickWrapper from './lib/CsprClickProvider'
import Navbar from './components/Navbar'
import Hero from './components/Hero'
import LiveDemo from './components/LiveDemo'
import Features from './components/Features'
import HowItWorks from './components/HowItWorks'
import Benchmarks from './components/Benchmarks'
import SDKSection from './components/SDKSection'
import UseCases from './components/UseCases'
import FAQ from './components/FAQ'
import CtaFooter from './components/CtaFooter'
import Footer from './components/Footer'
import ScrollProgress from './components/ScrollProgress'
import ScrollToTop from './components/ScrollToTop'
import NotFound from './components/NotFound'
import LabLayout from './components/lab/LabLayout'
import Overview from './components/lab/Overview'
import Proofs from './components/lab/Proofs'
import Models from './components/lab/Models'
import Aggregation from './components/lab/Aggregation'
import PQCrypto from './components/lab/PQCrypto'
import LabContracts from './components/lab/Contracts'
import AgentDemo from './components/lab/AgentDemo'
import LabPlayground from './components/lab/Playground'
import KYC from './components/lab/KYC'
import ZKProofs from './components/lab/ZKProofs'
import AttackEvidence from './components/lab/AttackEvidence'
import Decisions from './components/lab/Decisions'
import JudgeDashboard from './components/JudgeDashboard'
import OfflineVerify from './components/lab/OfflineVerify' 
import ApiDocs from './components/docs/ApiDocs'
import SdkDocs from './components/docs/SdkDocs'
import McpDocs from './components/docs/McpDocs'

function Landing() {
  useEffect(() => {
    const hash = window.location.hash.slice(1)
    if (hash) {
      const raf = requestAnimationFrame(() => {
        const el = document.getElementById(hash)
        if (el) el.scrollIntoView({ behavior: 'smooth' })
      })
      return () => cancelAnimationFrame(raf)
    }
  }, [])

  return (
    <>
      <Hero />
      <LiveDemo />
      <Features />
      <HowItWorks />
      <Benchmarks />
      <UseCases />
      <SDKSection />
      <FAQ />
      <CtaFooter />
    </>
  )
}

export default function App() {
  const [mobileOpen, setMobileOpen] = useState(false)
  const closeMobile = useCallback(() => setMobileOpen(false), [])
  const location = useLocation()
  const isLabRoute = location.pathname.startsWith('/lab')

  useEffect(() => {
    document.body.style.overflow = mobileOpen ? 'hidden' : ''
    return () => { document.body.style.overflow = '' }
  }, [mobileOpen])

  return (
    <CsprClickWrapper>
    <div className="min-h-screen flex flex-col">
      <ScrollProgress />
      <ScrollToTop />
      {!isLabRoute && <Navbar mobileOpen={mobileOpen} setMobileOpen={setMobileOpen} />}
      <main className="flex-1" onClick={mobileOpen ? closeMobile : undefined}>
        <Routes>
          <Route path="/" element={<Landing />} />
          <Route path="/lab" element={<LabLayout />}>
            <Route index element={<Navigate to="overview" replace />} />
            <Route path="overview" element={<Overview />} />
            <Route path="proofs" element={<Proofs />} />
            <Route path="models" element={<Models />} />
            <Route path="aggregation" element={<Aggregation />} />
            <Route path="zk-proofs" element={<ZKProofs />} />
            <Route path="pq-crypto" element={<PQCrypto />} />
            <Route path="contracts" element={<LabContracts />} />
            <Route path="agent-demo" element={<AgentDemo />} />
            <Route path="playground" element={<LabPlayground />} />
            <Route path="kyc" element={<KYC />} />
            <Route path="attack-evidence" element={<AttackEvidence />} />
            <Route path="decisions" element={<Decisions />} />
            <Route path="offline-verify" element={<OfflineVerify />} />
          </Route>
          <Route path="/judge" element={<JudgeDashboard />} />
          <Route path="/docs/api" element={<ApiDocs />} />
          <Route path="/docs/sdk" element={<SdkDocs />} />
          <Route path="/docs/mcp" element={<McpDocs />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>
      <Footer />
    </div>
    </CsprClickWrapper>
  )
}
