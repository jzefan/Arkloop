import { getCollection, type CollectionEntry } from 'astro:content';
import { getSidebar, type Locale } from '@/lib/navigation';

export async function getDocsCollection() {
  return getCollection('docs', (entry) => !entry.data.draft);
}

export async function getResearchCollection() {
  const entries = await getCollection('research', (entry) => !entry.data.draft);
  return entries.sort((left, right) => right.data.date.getTime() - left.data.date.getTime());
}

export function resolveDocEntryId(locale: Locale, section: 'docs' | 'api', slug?: string[]) {
  const prefix = locale === 'zh' ? 'zh' : 'en';

  if (section === 'api') {
    const tail = slug && slug.length > 0 ? slug.join('/') : 'index';
    return `${prefix}/api/${tail}`;
  }

  if (!slug || slug.length === 0) {
    return null;
  }

  const tail = slug.length === 1 ? `${slug[0]}/index` : slug.join('/');
  return `${prefix}/${tail}`;
}

export function findDocTitle(entry: CollectionEntry<'docs'>, fallbackTitle: string) {
  return entry.data.title ?? fallbackTitle;
}

export function findSidebarLabel(pathname: string, locale: Locale, section: 'docs' | 'api') {
  return getSidebar(locale, section)
    .flatMap((group) => group.items)
    .find((item) => item.href === pathname)?.label;
}
