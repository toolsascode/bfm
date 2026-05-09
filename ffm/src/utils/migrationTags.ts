/** Parse comma-separated key=value tags for migration API (AND semantics on the server). */
export function parseExecutionTags(
  raw: string,
): { ok: true; tags: string[] } | { ok: false; error: string } {
  const trimmed = raw.trim();
  if (!trimmed) {
    return { ok: true, tags: [] };
  }
  const parts = trimmed
    .split(",")
    .map((p) => p.trim())
    .filter(Boolean);
  for (const p of parts) {
    const eq = p.indexOf("=");
    if (eq <= 0 || !p.slice(0, eq).trim()) {
      return {
        ok: false,
        error: `Invalid tag "${p}" — each tag must be key=value`,
      };
    }
  }
  return { ok: true, tags: parts };
}
