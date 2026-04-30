'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useTheme } from 'next-themes';
import { useEffect, useState } from 'react';

type Lang = 'zh' | 'en';

const NAV: Record<Lang, { label: string; href: string }[]> = {
  zh: [
    { label: '文档', href: '/zh/docs/guide' },
    { label: 'API', href: '/zh/api' },
  ],
  en: [
    { label: 'Docs', href: '/en/docs/guide' },
    { label: 'API', href: '/en/api' },
  ],
};

export default function Topbar({ lang }: { lang: Lang }) {
  const pathname = usePathname();
  const otherLang = lang === 'zh' ? 'en' : 'zh';
  const switchPath = pathname.replace(new RegExp(`^/${lang}`), `/${otherLang}`);
  const { resolvedTheme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  return (
    <header className="topbar">
      <div className="topbar-inner">
        <Link href={`/${lang}`} className="brand">
          Arkloop
        </Link>
        <nav className="topnav">
          {NAV[lang].map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className={`nav-link${pathname.startsWith(item.href.replace('/guide', '')) ? ' nav-link-active' : ''}`}
            >
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="topbar-actions">
          <a
            className="topbar-gh"
            href="https://github.com/qqqqqf-q/Arkloop"
            target="_blank"
            rel="noopener noreferrer"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
              <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
            </svg>
            GitHub
          </a>
          <Link href={switchPath} className="lang-switch">
            {lang === 'zh' ? 'EN' : '中文'}
          </Link>
          <button
            className="theme-toggle"
            onClick={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}
            aria-label="Toggle theme"
            suppressHydrationWarning
          >
            {mounted && resolvedTheme === 'light' ? (
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"/>
              </svg>
            ) : (
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
              </svg>
            )}
          </button>
        </div>
      </div>
    </header>
  );
}
