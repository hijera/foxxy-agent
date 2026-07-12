/** Last path segment of a project path for compact display. */
export function projectBasename(path: string): string {
  const trimmed = (path || "").replace(/[\\/]+$/, "");
  const i = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  return i >= 0 ? trimmed.slice(i + 1) : trimmed;
}
