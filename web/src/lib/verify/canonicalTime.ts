const TIMESTAMP_RE =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;

/**
 * Reproduces `e.OccurredAt.UTC().Format(time.RFC3339Nano)` from
 * internal/audit/events.go string-arithmetically, WITHOUT round-tripping
 * through `Date` — JS `Date` only carries millisecond precision and would
 * silently destroy the nanosecond-level detail that the Go hash is computed
 * over. The wire string may carry any offset (e.g. `+02:00`); the canonical
 * form is always expressed in `Z` with Go's trailing-zero trimming applied.
 */
export function canonicalTime(occurredAtRaw: string): string {
  const match = TIMESTAMP_RE.exec(occurredAtRaw);
  if (!match) {
    throw new Error(`canonicalTime: unparseable timestamp: ${occurredAtRaw}`);
  }

  const [, year, month, day, hour, minute, second, fraction, offset] = match;

  // Date is used only to carry out the integer-second UTC conversion (and
  // any resulting date/hour rollover from a non-Z offset). The fractional
  // part is handled separately, as a raw digit string, so nanosecond
  // precision never passes through Date.
  const localMs = Date.UTC(
    Number(year),
    Number(month) - 1,
    Number(day),
    Number(hour),
    Number(minute),
    Number(second),
  );
  const utcMs = offset === "Z" ? localMs : localMs - offsetToMillis(offset!);
  const utc = new Date(utcMs);

  const datePart = [
    String(utc.getUTCFullYear()).padStart(4, "0"),
    String(utc.getUTCMonth() + 1).padStart(2, "0"),
    String(utc.getUTCDate()).padStart(2, "0"),
  ].join("-");
  const timePart = [
    String(utc.getUTCHours()).padStart(2, "0"),
    String(utc.getUTCMinutes()).padStart(2, "0"),
    String(utc.getUTCSeconds()).padStart(2, "0"),
  ].join(":");

  return `${datePart}T${timePart}${trimTrailingZeros(fraction)}Z`;
}

function offsetToMillis(offset: string): number {
  const sign = offset.startsWith("-") ? -1 : 1;
  const [hours, minutes] = offset.slice(1).split(":").map(Number);
  return sign * (((hours ?? 0) * 60 + (minutes ?? 0)) * 60 * 1000);
}

// Mirrors Go's RFC3339Nano fractional-second trimming: trailing zero digits
// are dropped, and the fraction (including its leading dot) disappears
// entirely if nothing but zeros remain.
function trimTrailingZeros(fraction: string | undefined): string {
  if (!fraction) return "";
  const digits = fraction.slice(1).replace(/0+$/, "");
  return digits ? `.${digits}` : "";
}
