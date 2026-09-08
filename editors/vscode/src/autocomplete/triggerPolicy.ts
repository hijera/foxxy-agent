/** Whether an automatic (as-you-type) request is worth making at this caret position. The
 *  prefix cache runs before this and an explicit invoke bypasses it, so a rejected position
 *  costs nothing; it only avoids paying for a guess that is almost always wrong.
 *
 *  A caret followed by an identifier character sits inside a word the user is still typing.
 *  A caret right after a closing bracket or statement end is where nothing is expected next.
 *  Indentation alone is not content yet - but an empty line is deliberately allowed, because
 *  Enter after `{` is exactly where a block suggestion belongs.
 *
 *  Mirrors `SuggestionText.shouldRequest` in the IntelliJ plugin. */
export function shouldRequestAutomatically(lineBefore: string, charAfter: string): boolean {
  if (charAfter !== "" && /[\p{L}\p{N}_]/u.test(charAfter)) return false;
  const trimmed = lineBefore.replace(/[ \t]+$/, "");
  if (trimmed !== "" && ")]};,".includes(trimmed[trimmed.length - 1] ?? "")) {
    // Only the keystroke that produced the closer is skipped; typing on after it (a space or
    // more code) is ordinary typing again.
    if (trimmed.length === lineBefore.length) return false;
  }
  return true;
}
