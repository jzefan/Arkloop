import type { Locale } from '@/lib/navigation';

function stripLocale(id: string, locale: Locale) {
  const prefix = `${locale}/`;
  return id.startsWith(prefix) ? id.slice(prefix.length) : id;
}

function normalizeContentPath(path: string) {
  const withoutExtension = path.replace(/\.(md|mdx)$/u, '');
  return withoutExtension.endsWith('/index') ? withoutExtension.slice(0, -'/index'.length) : withoutExtension;
}

export function docsSlugFromId(id: string, locale: Locale) {
  const stripped = stripLocale(id, locale);
  if (stripped.startsWith('api/')) return null;
  return normalizeContentPath(stripped);
}

export function apiSlugFromId(id: string, locale: Locale) {
  const stripped = stripLocale(id, locale);
  if (!stripped.startsWith('api/')) return null;
  const tail = normalizeContentPath(stripped.slice(4));
  return tail.length === 0 ? null : tail;
}

export function mirrorEntryId(id: string, locale: Locale) {
  return locale === 'zh' ? id.replace(/^zh\//u, 'en/') : id.replace(/^en\//u, 'zh/');
}
