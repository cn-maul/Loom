export function todayISO(): string {
  const now = new Date();
  return new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 10);
}

function parseDate(value: string): Date | null {
  if (!value) return null;
  // Bare YYYY-MM-DD must stay a local date; parsing it as UTC shifts it a day west.
  const date = /^\d{4}-\d{2}-\d{2}$/.test(value) ? new Date(`${value}T00:00:00`) : new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

/** Short form for timeline rows: "09-08", or the value unchanged when unparseable. */
export function shortDate(value: string): string {
  const date = parseDate(value);
  if (!date) return value;
  const month = `${date.getMonth() + 1}`.padStart(2, '0');
  const day = `${date.getDate()}`.padStart(2, '0');
  return `${month}-${day}`;
}

export function fullDate(value: string): string {
  const date = parseDate(value);
  if (!date) return value;
  return `${date.getFullYear()}-${`${date.getMonth() + 1}`.padStart(2, '0')}-${`${date.getDate()}`.padStart(2, '0')}`;
}

/** Human distance to a date, e.g. 今天 / 昨天 / 3天前 / 2周前 / 8个月前. */
export function relativeTime(value: string): string {
  const date = parseDate(value);
  if (!date) return '';
  const now = new Date();
  const days = Math.floor(
    (new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime() -
      new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()) /
      86400000,
  );
  if (days <= 0) return '今天';
  if (days === 1) return '昨天';
  if (days < 14) return `${days}天前`;
  if (days < 60) return `${Math.floor(days / 7)}周前`;
  if (days < 365) return `${Math.floor(days / 30)}个月前`;
  return `${Math.floor(days / 365)}年前`;
}

/** Timeline rows fall back to the raw note when the AI extraction came back empty. */
export function eventHeadline(text: string, fallback: string): string {
  if (text.trim()) return text.trim();
  const line = fallback.trim().split('\n')[0] ?? '';
  return line.length > 60 ? `${line.slice(0, 60)}…` : line;
}

const QUOTE_PAIRS: [string, string][] = [
  ['“', '”'],
  ['「', '」'],
  ['『', '』'],
  ['"', '"'],
  ["'", "'"],
];

/** Models often wrap a suggested script in their own quotes; unwrap them before adding 「」. */
export function quoteScript(script: string): string {
  let text = script.trim();
  for (;;) {
    const pair = text.length > 1 ? QUOTE_PAIRS.find(([open, close]) => text.startsWith(open) && text.endsWith(close)) : undefined;
    if (!pair) return `「${text}」`;
    text = text.slice(pair[0].length, -pair[1].length).trim();
  }
}
