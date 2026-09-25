import React, { useEffect, useState } from 'react';
import { Activity, CheckCircle, XCircle, Users, Clock, HeartPulse, HardHat } from 'lucide-react';
import { getStats, getHealth, StatsResponse, HealthResponse } from '../../lib/api';
import SectionIntro from './SectionIntro';
import { CardSkeleton } from '../ui/Skeleton';

const Overview: React.FC = () => {
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchData = async () => {
      setLoading(true);
      setError(null);
      try {
        const [statsRes, healthRes] = await Promise.all([getStats(), getHealth()]);

        if (statsRes.success && statsRes.data) {
          setStats(statsRes.data);
        } else {
          setError(statsRes.error || 'Failed to fetch stats');
        }

        if (healthRes.success && healthRes.data) {
          setHealth(healthRes.data);
        } else {
          setError(healthRes.error || 'Failed to fetch health status');
        }
      } catch (err) {
        setError('An unexpected error occurred while fetching data.');
        if (import.meta.env.DEV) console.error(err);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  const renderCard = (title: string, value: string | number, icon: React.ElementType, color: string) => (
    <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] flex items-center justify-between shadow-md">
      <div>
        <h3 className="text-lg font-medium text-gray-300">{title}</h3>
        <p className={`text-3xl font-bold ${color}`}>{value}</p>
      </div>
      <div className={`p-3 rounded-full ${color.replace('text-', 'bg-')}/20`}>
        {React.createElement(icon, { size: 32, className: color })}
      </div>
    </div>
  );

  const renderHealthStatus = () => {
    if (!health) return <p className="text-gray-400">Health data not available.</p>;

    const isHealthy = health.status === 'ok';
    const statusColor = isHealthy ? 'text-green-500' : 'text-red-500';
    const statusIcon = isHealthy ? CheckCircle : XCircle;

    return (
      <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
        <h3 className="text-xl font-semibold text-gray-200 mb-4 flex items-center gap-2">
          <HeartPulse size={24} className={statusColor} />
          System Health
        </h3>
        <div className="flex items-center gap-2 mb-4">
          <span className={`text-lg font-medium ${statusColor}`}>{health.status.toUpperCase()}</span>
          {React.createElement(statusIcon, { size: 20, className: statusColor })}
        </div>
        <p className="text-gray-400 mb-1">Version: <span className="font-mono text-gray-300">{health.version}</span></p>
        <p className="text-gray-400 mb-2">Chain: <span className="font-mono text-gray-300">{health.chain}</span></p>
        <h4 className="text-md font-medium text-gray-300 mt-4 mb-2 flex items-center gap-2">
          <HardHat size={20} className="text-red-500" />
          Connected Contracts
        </h4>
        <ul className="space-y-1 text-gray-400">
          {Object.entries(health.contracts).map(([name, address]) => (
            <li key={name} className="flex justify-between items-center">
              <span className="capitalize">{name.replace(/_/g, ' ')}:</span>
              <a
                href={`https://scan.botchain.ai/contract/${address}`}
                target="_blank"
                rel="noopener noreferrer"
                className="font-mono text-sm text-red-400 hover:text-red-300 break-all ml-2 truncate max-w-[320px]"
                title={String(address)}
              >
                {String(address).slice(0, 12)}...{String(address).slice(-8)}
              </a>
            </li>
          ))}
        </ul>
      </div>
    );
  };

  const renderUseCaseDistribution = () => {
    const colors = ['bg-red-500', 'bg-purple-500', 'bg-blue-500', 'bg-green-500', 'bg-yellow-500', 'bg-pink-500'];
    const useCases = stats?.use_cases;
    if (!useCases || Object.keys(useCases).length === 0) {
      return (
        <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
          <h3 className="text-xl font-semibold text-gray-200 mb-4">Use Case Distribution</h3>
          <p className="text-gray-400">No use case data available yet.</p>
        </div>
      );
    }

    const total = Object.values(useCases).reduce((s, v) => s + v, 0);
    const entries = Object.entries(useCases)
      .sort((a, b) => b[1] - a[1])
      .map(([name, count], i) => ({
        name: name.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase()),
        count,
        pct: total > 0 ? Math.round((count / total) * 100) : 0,
        color: colors[i % colors.length],
      }));

    return (
      <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
        <h3 className="text-xl font-semibold text-gray-200 mb-4">Use Case Distribution</h3>
        <div className="space-y-3">
          {entries.map((uc) => (
            <div key={uc.name}>
              <div className="flex justify-between text-gray-300 text-sm mb-1">
                <span>{uc.name}</span>
                <span>{uc.count} ({uc.pct}%)</span>
              </div>
              <div className="w-full bg-[#222235] rounded-full h-2.5">
                <div className={`${uc.color} h-2.5 rounded-full`} style={{ width: `${uc.pct}%` }}></div>
              </div>
            </div>
          ))}
        </div>
      </div>
    );
  };

  if (loading) {
    return (
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {Array.from({ length: 6 }).map((_, i) => (
          <CardSkeleton key={i} />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center p-8 text-red-400">
        <XCircle className="mx-auto mb-4" size={32} />
        Error: {error}
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <SectionIntro
        title="System Overview"
        description="Real-time dashboard showing proof engine statistics, system health, connected smart contracts on BOT Chain testnet, and use case distribution. All numbers reflect live data from the running Bot Prove backend."
        dataSource="Live Bot Prove API (/stats + /health endpoints). Contract addresses verified on BOT Chain testnet."
        badge="Live data"
        badgeColor="green"
        helpText="Numbers refresh on every page load. If /health is down, cards fall back to zeros."
      />
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        {renderCard('Total Proofs', stats?.total_proofs ?? 0, Activity, 'text-red-500')}
        {renderCard('Valid Proofs', stats?.valid_proofs ?? 0, CheckCircle, 'text-green-500')}
        {renderCard('Revoked Proofs', stats?.revoked_proofs ?? 0, XCircle, 'text-yellow-500')}
        {renderCard('Unique Agents', stats?.unique_agents ?? 0, Users, 'text-blue-500')}
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {renderHealthStatus()}
        {renderUseCaseDistribution()}
      </div>

      <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
        <h3 className="text-xl font-semibold text-gray-200 mb-4 flex items-center gap-2">
          <Clock size={24} className="text-red-500" />
          Performance Metrics
        </h3>
        <p className="text-gray-300">
          Average Proof Generation Time:{' '}
          <span className="font-bold text-red-400">
            {stats?.avg_generation_ms ? `${stats.avg_generation_ms.toFixed(2)} ms` : 'N/A'}
          </span>
        </p>
        <p className="text-gray-300 mt-2">
          Max Merkle Depth:{' '}
          <span className="font-bold text-red-400">
            {stats?.max_merkle_depth ?? 'N/A'}
          </span>
        </p>
        <p className="text-gray-400 text-sm mt-2">
          Average time to generate a cryptographic proof across all registered proofs.
        </p>
      </div>
    </div>
  );
};

export default Overview;
