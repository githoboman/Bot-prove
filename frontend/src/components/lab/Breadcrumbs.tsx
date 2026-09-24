import React from 'react';
import { Link, useLocation } from 'react-router-dom';
import { ChevronRight } from 'lucide-react';

interface Crumb {
  label: string;
  to?: string;
}

interface Props {
  /**
   * Optional explicit crumb list. If omitted, Breadcrumbs derives crumbs
   * from the current pathname (Home > Lab > <sub-page>).
   */
  items?: Crumb[];
}

function titleCase(seg: string): string {
  return seg
    .split('-')
    .map((w) => (w.length ? w[0].toUpperCase() + w.slice(1) : w))
    .join(' ');
}

function deriveFromPath(pathname: string): Crumb[] {
  const parts = pathname.split('/').filter(Boolean);
  const out: Crumb[] = [{ label: 'Home', to: '/' }];
  let acc = '';
  parts.forEach((p, i) => {
    acc += '/' + p;
    const last = i === parts.length - 1;
    out.push({ label: titleCase(p), to: last ? undefined : acc });
  });
  return out;
}

export default function Breadcrumbs({ items }: Props) {
  const location = useLocation();
  const crumbs = items ?? deriveFromPath(location.pathname);

  return (
    <nav aria-label="Breadcrumb" className="text-xs text-gray-500 mb-3">
      <ol className="flex items-center gap-1 flex-wrap">
        {crumbs.map((c, i) => {
          const isLast = i === crumbs.length - 1;
          return (
            <li key={`${c.label}-${i}`} className="flex items-center gap-1">
              {i > 0 && <ChevronRight className="h-3 w-3 text-gray-600" aria-hidden="true" />}
              {c.to && !isLast ? (
                <Link to={c.to} className="hover:text-gray-300 transition-colors">
                  {c.label}
                </Link>
              ) : (
                <span className={isLast ? 'text-gray-300' : ''}>{c.label}</span>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
