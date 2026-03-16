/**
 * Shared Tailwind className constants for console-lite pages.
 * Use these instead of repeating long className strings inline.
 */

export const consoleCls = {
  /** Standard text input */
  input:
    'w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-input)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] outline-none transition-colors focus:border-[var(--c-border-focus)]',

  /** Secondary / cancel button */
  btnSecondary:
    'rounded-lg border border-[var(--c-border)] px-3.5 py-1.5 text-sm text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50',

  /** Primary / save button */
  btnPrimary:
    'rounded-lg bg-[var(--c-bg-tag)] px-3.5 py-1.5 text-sm font-medium text-[var(--c-text-primary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50',

  /** Destructive / delete button */
  btnDestructive:
    'rounded-lg bg-red-600 px-3.5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50',

  /** Textarea */
  textarea:
    'w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-input)] px-3 py-2 text-sm text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] outline-none transition-colors focus:border-[var(--c-border-focus)] resize-none',
}
