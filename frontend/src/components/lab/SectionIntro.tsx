import React from 'react';
import { Info, HelpCircle } from 'lucide-react';
import StatusBadge, { StatusBadgeColor } from './StatusBadge';

interface SectionIntroProps {
  title: string;
  description: string;
  dataSource: string;
  /** e.g. "Live API data" or "Real testnet contracts" */
  badge?: string;
  badgeColor?: StatusBadgeColor;
  /** Optional help text — renders as a hoverable ? icon next to the title. */
  helpText?: string;
}

const SectionIntro: React.FC<SectionIntroProps> = ({
  title,
  description,
  dataSource,
  badge = 'Live data',
  badgeColor = 'green',
  helpText,
}) => (
  <div className="mb-6 p-4 bg-[#1a1a2a] rounded-lg border border-[#222235]">
    <div className="flex items-start gap-3">
      <Info className="w-5 h-5 text-gray-400 mt-0.5 shrink-0" />
      <div className="space-y-1.5">
        <div className="flex items-center gap-2 flex-wrap">
          <h3 className="text-sm font-semibold text-gray-200">{title}</h3>
          {helpText && (
            <span
              tabIndex={0}
              role="button"
              aria-label="Help"
              title={helpText}
              className="inline-flex items-center justify-center text-gray-500 hover:text-gray-300 focus:text-gray-200 outline-none cursor-help"
            >
              <HelpCircle className="w-3.5 h-3.5" />
            </span>
          )}
          <StatusBadge label={badge} color={badgeColor} />
        </div>
        <p className="text-xs text-gray-400 leading-relaxed">{description}</p>
        <p className="text-[11px] text-gray-500">
          <span className="text-gray-400 font-medium">Data source:</span> {dataSource}
        </p>
      </div>
    </div>
  </div>
);

export default SectionIntro;
