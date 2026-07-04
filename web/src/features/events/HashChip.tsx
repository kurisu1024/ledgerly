import { useState } from "react";
import type { ReactNode } from "react";

interface HashChipProps {
  hash: string;
}

function truncate(hash: string): string {
  if (hash.length <= 16) return hash;
  return `${hash.slice(0, 8)}…${hash.slice(-6)}`;
}

/**
 * Displays a truncated hash; expands to the full base64 value on click or
 * Enter (keyboard-operable, matching its implicit interactive role).
 */
export function HashChip({ hash }: HashChipProps): ReactNode {
  const [expanded, setExpanded] = useState(false);

  return (
    <button
      type="button"
      className="hash-chip"
      aria-pressed={expanded}
      onClick={() => setExpanded((prev) => !prev)}
    >
      {expanded ? hash : truncate(hash)}
    </button>
  );
}
