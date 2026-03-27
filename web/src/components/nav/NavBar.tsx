'use client';

import { useStore } from '@/lib/store';
import { usePathname } from 'next/navigation';
import Link from 'next/link';

const NAV_ITEMS = [
  { href: '/', label: 'Dashboard' },
  { href: '/flows', label: 'Flows' },
  { href: '/topology', label: 'Topology' },
  { href: '/training', label: 'Training' },
  { href: '/alerts', label: 'Alerts' },
];

export function NavBar() {
  const pathname = usePathname();
  const unacknowledgedCount = useStore((s) => s.unacknowledgedCount);

  return (
    <header className="border-b border-fp-border px-6 py-3 flex items-center justify-between">
      <div className="flex items-center gap-3">
        <div className="w-8 h-8 rounded-lg bg-fp-accent flex items-center justify-center text-white font-bold text-sm">
          FP
        </div>
        <h1 className="text-lg font-semibold text-white">FlowPulse</h1>
        <span className="text-xs text-fp-muted">Training Flow Monitor</span>
      </div>
      <nav className="flex items-center gap-6 text-sm">
        {NAV_ITEMS.map((item) => {
          const isActive = pathname === item.href;
          const isAlerts = item.href === '/alerts';

          return (
            <Link
              key={item.href}
              href={item.href}
              className={`relative transition-colors ${
                isActive ? 'text-white font-medium' : 'text-fp-muted hover:text-white'
              }`}
            >
              {item.label}
              {isAlerts && unacknowledgedCount > 0 && (
                <span className="absolute -top-2 -right-4 min-w-[18px] h-[18px] flex items-center justify-center bg-red-500 text-white text-[10px] font-bold rounded-full px-1 animate-pulse">
                  {unacknowledgedCount > 99 ? '99+' : unacknowledgedCount}
                </span>
              )}
            </Link>
          );
        })}
      </nav>
    </header>
  );
}
