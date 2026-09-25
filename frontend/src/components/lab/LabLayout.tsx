import React, { useCallback, useEffect, useState } from 'react';
import ErrorBoundary from './ErrorBoundary';
import Breadcrumbs from './Breadcrumbs';
import KeyboardHelpModal from './KeyboardHelpModal';
import { useKeyboardShortcuts } from '../../lib/useKeyboardShortcuts';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { useWallet } from '../../lib/WalletProvider';
import { shortKey } from '../../lib/wallet';
import {
  Wallet,
  FlaskConical,
  Radio,
  Menu,
  X,
  Home,
  Link2,
  BookOpen,
  Code2,
  Cpu,
  Info,
  Lightbulb,
} from 'lucide-react';

/**
 * Lab navigation groups.
 *
 * Every existing tab is preserved — same path, same label, same route.
 * Groups only add semantic labels so the nav communicates purpose to
 * three audiences (product / crypto & chain / dev / explore) without
 * removing anything or changing URLs. The flat `tabs` array is derived
 * so any code that iterated over it in the past still works.
 */
interface LabTab {
  name: string;
  path: string;
}

interface LabTabGroup {
  key: string;
  label: string;
  tabs: LabTab[];
}

const tabGroups: LabTabGroup[] = [
  {
    key: 'core',
    label: 'Core workflow',
    tabs: [
      { name: 'Overview', path: 'overview' },
      { name: 'Proofs', path: 'proofs' },
      { name: 'Models', path: 'models' },
      { name: 'Aggregation', path: 'aggregation' },
    ],
  },
  {
    key: 'crypto-chain',
    label: 'Cryptography & chain',
    tabs: [
      { name: 'ZK Proofs', path: 'zk-proofs' },
      { name: 'PQ Crypto', path: 'pq-crypto' },
      { name: 'Contracts', path: 'contracts' },
    ],
  },
  {
    key: 'dev-tools',
    label: 'Developer tools',
    tabs: [{ name: 'Playground', path: 'playground' }],
  },
  {
    key: 'explore',
    label: 'Explore',
    tabs: [
      { name: 'KYC', path: 'kyc' },
      { name: 'Attack Evidence', path: 'attack-evidence' },
      { name: 'Offline Verify', path: 'offline-verify' },
    ],
  },
];

const tabs: LabTab[] = tabGroups.flatMap(g => g.tabs);

const externalLinks = [
  { name: 'Home', href: '/', icon: Home },
  { name: 'GitHub', href: 'https://github.com/anna-stolbovskaja/Bot Prove', icon: Link2 },
  { name: 'API Docs', href: '/docs/api', icon: BookOpen },
  { name: 'SDK Docs', href: '/docs/sdk', icon: Code2 },
  { name: 'MCP Docs', href: '/docs/mcp', icon: Cpu },
];

/** Content for the "Demo Mode explained" popup */
const DemoModeInfo: React.FC<{ onClose: () => void }> = ({ onClose }) => (
  <div className="fixed inset-0 bg-black/75 flex items-center justify-center z-[60] p-4" onClick={onClose}>
    <div
      className="bg-[#13131d] border border-[#222235] rounded-lg shadow-xl max-w-lg w-full max-h-[90vh] overflow-y-auto"
      onClick={(e) => e.stopPropagation()}
    >
      <div className="flex justify-between items-center p-4 border-b border-[#222235]">
        <h3 className="text-lg font-semibold text-gray-100 flex items-center gap-2">
          <FlaskConical className="w-5 h-5 text-yellow-400" /> About Demo Mode
        </h3>
        <button onClick={onClose} className="text-gray-400 hover:text-gray-200 p-1">
          <X size={18} />
        </button>
      </div>
      <div className="p-4 space-y-4 text-sm">
        <div className="space-y-2">
          <p className="text-gray-300 leading-relaxed">
            <strong className="text-yellow-400">Demo Mode</strong> means write operations
            (creating proofs, registering models, anchoring to blockchain) are signed by the
            server's key — not your wallet. <strong>All data shown is real.</strong>
          </p>
          <p className="text-gray-300 leading-relaxed">
            <strong className="text-green-400">Testnet Mode</strong> (with wallet connected):
            transactions are signed with your BOT Chain wallet keys directly on testnet.
          </p>
        </div>

        <div className="border-t border-[#222235] pt-3">
          <h4 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2">Data sources by section</h4>
          <div className="space-y-1.5">
            {[
              ['Overview', 'Live stats from Bot Prove API + real contract addresses on BOT Chain testnet'],
              ['Proofs', 'Real cryptographic proofs (Merkle trees, SHA-256) stored in the proof engine'],
              ['Models', 'On-chain model registry via verifier_gate smart contract'],
              ['ZK Proofs', 'Real Groth16 zero-knowledge proofs computed in real-time via gnark'],
              ['PQ Crypto', 'Real ML-DSA-65 post-quantum digital signatures (FIPS 204)'],
              ['Contracts', '9 smart contracts deployed on BOT Chain testnet with verified deploy hashes'],
              ['Aggregation', 'Live batch operations via proof_aggregation engine (session-scoped)'],
              ['KYC', 'Privacy-preserving KYC verification through zero-knowledge proofs'],
              ['Playground', 'API Playground (32 endpoints) + Agent Playground (full pipeline demo)'],
              ['Attack Evidence', 'Live tampering demos — 5 attacks executed against the real verifier'],
            ].map(([section, desc]) => (
              <div key={section} className="flex gap-2">
                <span className="text-green-400 text-xs font-mono w-28 shrink-0">{section}</span>
                <span className="text-gray-400 text-xs">{desc}</span>
              </div>
            ))}
          </div>
        </div>

        <div className="bg-green-900/20 border border-green-700/30 rounded-lg p-3">
          <p className="text-green-300 text-xs leading-relaxed flex items-start gap-2">
            <Lightbulb className="w-4 h-4 shrink-0 mt-0.5 text-green-300" aria-hidden="true" />
            <span>
              <strong>Bottom line:</strong> Everything you see is real data from a running cryptographic
              engine and real smart contracts on BOT Chain testnet. "Demo" only refers to who signs the
              on-chain transactions.
            </span>
          </p>
        </div>
      </div>
    </div>
  </div>
);

/** Which group owns the given /lab/<path> segment, defaulting to the first group. */
function groupKeyForPath(pathname: string): string {
  const segment = pathname.split('/').filter(Boolean).pop() ?? '';
  const owner = tabGroups.find(g => g.tabs.some(t => t.path === segment));
  return owner ? owner.key : tabGroups[0].key;
}

const LabLayout: React.FC = () => {
  const { publicKey, connected: isConnected, signIn, signOut } = useWallet();
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [demoInfoOpen, setDemoInfoOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const location = useLocation();
  const openShortcuts = useCallback(() => setShortcutsOpen(true), []);
  useKeyboardShortcuts(openShortcuts);

  // Two-level desktop nav: which category row is expanded. Starts on
  // whichever group owns the current route, then stays under user control
  // (clicking a category switches the second row without navigating) but
  // still follows direct links / browser back-forward to the right group.
  const [activeGroupKey, setActiveGroupKey] = useState(() => groupKeyForPath(location.pathname));
  useEffect(() => {
    setActiveGroupKey(groupKeyForPath(location.pathname));
  }, [location.pathname]);
  const activeGroup = tabGroups.find(g => g.key === activeGroupKey) ?? tabGroups[0];

  // Auto-close mobile menu on route change
  useEffect(() => {
    setMobileMenuOpen(false);
  }, [location.pathname]);

  return (
    <div className="min-h-screen bg-[#0b0b10] text-gray-100 font-sans">
      {/* Sticky top bar */}
      <div className="sticky top-0 z-40 bg-[#0b0b10]/95 backdrop-blur border-b border-[#222235]">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          {/* Top row: logo + wallet */}
          <div className="flex items-center justify-between h-14">
            <a href="/" className="flex items-center gap-2">
              <img src="/images/logo.jpg" alt="Bot Prove" className="h-6 w-auto" />
              <span className="font-bold text-white text-base hidden sm:block">Bot Prove</span>
            </a>

            <div className="flex items-center gap-2 sm:gap-3">
              {/* Demo / Testnet mode badge — clickable */}
              {isConnected ? (
                <button
                  onClick={() => setDemoInfoOpen(true)}
                  className="flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-green-900/30 border border-green-700/40 text-green-400 text-xs font-medium hover:bg-green-900/50 transition-colors"
                  title="Click for details"
                >
                  <Radio className="w-3 h-3" />
                  Testnet
                  <Info className="w-3 h-3 opacity-60" />
                </button>
              ) : (
                <button
                  onClick={() => setDemoInfoOpen(true)}
                  className="flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-yellow-900/30 border border-yellow-700/40 text-yellow-400 text-xs font-medium hover:bg-yellow-900/50 transition-colors"
                  title="Click for details"
                >
                  <FlaskConical className="w-3 h-3" />
                  Demo Mode
                  <Info className="w-3 h-3 opacity-60" />
                </button>
              )}

              {/* Wallet button */}
              {isConnected ? (
                <button
                  onClick={() => signOut()}
                  className="flex items-center gap-2 px-3 py-1.5 bg-green-900/30 border border-green-700/50 text-green-400 rounded-full text-sm font-mono hover:bg-green-900/50 transition-colors"
                >
                  <div className="w-2 h-2 rounded-full bg-green-400 animate-pulse" />
                  {publicKey ? shortKey(publicKey) : 'Connected'}
                </button>
              ) : (
                <button
                  onClick={() => signIn()}
                  className="hidden sm:flex items-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-full text-sm font-medium transition-colors"
                >
                  <Wallet className="w-4 h-4" />
                  Connect Wallet
                </button>
              )}

              {/* Mobile hamburger */}
              <button
                onClick={() => setMobileMenuOpen(true)}
                className="md:hidden p-2 text-gray-400 hover:text-gray-200 transition-colors"
                aria-label="Open menu"
              >
                <Menu className="w-5 h-5" />
              </button>
            </div>
          </div>

          {/*
            Desktop nav is two-level: a category row (4 groups, always all
            visible, never scrolls/wraps unpredictably) plus a tab row for
            whichever category is active. This replaced a flat 11-tab flex
            row: at 4 groups it either horizontal-scrolled with no visible
            affordance, or (briefly) wrapped onto two lines which read as
            messy/unpolished. Two-level fixes both: one clean line of
            categories, one clean line of tabs, always exactly 2 rows.
          */}
          <div className="hidden md:block -mb-px">
            <nav className="flex items-center gap-1" aria-label="Lab categories" role="tablist">
              {tabGroups.map((group) => {
                const isActive = group.key === activeGroupKey;
                return (
                  <button
                    key={group.key}
                    type="button"
                    role="tab"
                    aria-selected={isActive}
                    onClick={() => setActiveGroupKey(group.key)}
                    className={`px-3 py-2 text-[11px] font-semibold uppercase tracking-wider rounded-t-md transition-colors whitespace-nowrap ${
                      isActive
                        ? 'bg-[#13131d] text-red-400 border border-[#222235] border-b-0'
                        : 'text-gray-500 hover:text-gray-300 border border-transparent'
                    }`}
                  >
                    {group.label}
                  </button>
                );
              })}
            </nav>
            <nav
              key={activeGroup.key}
              className="flex items-center gap-x-1 border-t border-[#222235] bg-[#13131d]/60"
              aria-label={`${activeGroup.label} sections`}
            >
              {activeGroup.tabs.map((tab) => (
                <NavLink
                  key={tab.name}
                  to={tab.path}
                  className={({ isActive }) =>
                    `py-2.5 px-3 text-sm font-medium transition-colors duration-200 whitespace-nowrap border-b-2 ${
                      isActive
                        ? 'text-red-500 border-red-500'
                        : 'text-gray-400 border-transparent hover:text-gray-200 hover:border-gray-600'
                    }`
                  }
                  end={tab.path === 'overview'}
                >
                  {tab.name}
                </NavLink>
              ))}
            </nav>
          </div>
        </div>
      </div>

      {/* Mobile fullscreen menu */}
      {mobileMenuOpen && (
        <div className="fixed inset-0 z-50 bg-[#0b0b10] flex flex-col">
          <div className="flex items-center justify-between px-4 h-14 border-b border-[#222235]">
            <a href="/" className="flex items-center gap-2">
              <img src="/images/logo.jpg" alt="Bot Prove" className="h-6 w-auto" />
              <span className="font-bold text-white text-base">Bot Prove</span>
            </a>
            <button
              onClick={() => setMobileMenuOpen(false)}
              className="p-2 text-gray-400 hover:text-gray-200"
              aria-label="Close menu"
            >
              <X className="w-5 h-5" />
            </button>
          </div>

          <div className="flex-1 overflow-y-auto px-4 py-4">
            {/*
              Mobile: one section per group, same links, same paths. The
              previous single "Proof Lab" heading is replaced by four
              purpose-labeled subheadings so the menu communicates the
              same grouping as the desktop nav.
            */}
            {tabGroups.map(group => (
              <div key={group.key} className="mb-5">
                <p className="text-xs text-gray-500 uppercase tracking-wider font-semibold mb-2 px-3">{group.label}</p>
                <div className="space-y-0.5" role="group" aria-label={group.label}>
                  {group.tabs.map((tab) => (
                    <NavLink
                      key={tab.name}
                      to={tab.path}
                      onClick={() => setMobileMenuOpen(false)}
                      className={({ isActive }) =>
                        `block px-3 py-2.5 rounded-lg text-sm font-medium transition-colors ${
                          isActive
                            ? 'bg-red-600/20 text-red-400 border-l-2 border-red-500'
                            : 'text-gray-300 hover:bg-[#1a1a2a]'
                        }`
                      }
                      end={tab.path === 'overview'}
                    >
                      {tab.name}
                    </NavLink>
                  ))}
                </div>
              </div>
            ))}

            {/* External links */}
            <p className="text-xs text-gray-500 uppercase tracking-wider font-semibold mb-2 px-3">Links</p>
            <div className="space-y-0.5 mb-6">
              {externalLinks.map((link) => (
                <a
                  key={link.name}
                  href={link.href}
                  onClick={() => setMobileMenuOpen(false)}
                  target={link.href.startsWith('http') ? '_blank' : undefined}
                  rel={link.href.startsWith('http') ? 'noopener noreferrer' : undefined}
                  className="flex items-center gap-2.5 px-3 py-2.5 rounded-lg text-sm text-gray-300 hover:bg-[#1a1a2a] transition-colors"
                >
                  <link.icon className="w-4 h-4 text-gray-500" />
                  {link.name}
                </a>
              ))}
            </div>

            {/* Wallet button (mobile) */}
            {!isConnected && (
              <button
                onClick={() => { signIn(); setMobileMenuOpen(false); }}
                className="w-full flex items-center justify-center gap-2 px-4 py-3 bg-red-600 hover:bg-red-700 text-white rounded-full text-sm font-medium transition-colors"
              >
                <Wallet className="w-4 h-4" />
                Connect Wallet
              </button>
            )}
          </div>
        </div>
      )}

      {/* Mode banner */}
      {!isConnected && (
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 mt-2">
          <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-yellow-900/20 border border-yellow-700/30 text-yellow-300 text-xs">
            <FlaskConical className="w-3.5 h-3.5 shrink-0" />
            <span>
              <strong>Demo mode</strong> — all data is real. Write operations use the server signing key.{' '}
              <button
                onClick={() => setDemoInfoOpen(true)}
                className="underline hover:text-yellow-200 transition-colors"
              >
                Learn more
              </button>{' '}
              or connect a BOT Chain wallet to sign with your own keys.
            </span>
          </div>
        </div>
      )}

      {/* Demo mode info popup */}
      {demoInfoOpen && <DemoModeInfo onClose={() => setDemoInfoOpen(false)} />}
      <KeyboardHelpModal open={shortcutsOpen} onClose={() => setShortcutsOpen(false)} />

      {/* Content */}
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4">
        <Breadcrumbs />
        <main className="p-4 bg-[#13131d] border border-[#222235] rounded-lg shadow-lg min-h-[calc(100vh-200px)]">
          <ErrorBoundary key={location.pathname}>
            <Outlet />
          </ErrorBoundary>
        </main>
      </div>
    </div>
  );
};

export default LabLayout;
