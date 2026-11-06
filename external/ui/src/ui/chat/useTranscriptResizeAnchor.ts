import { useCallback, useLayoutEffect, useRef, type RefObject } from "react";
import { snapshotShellStack } from "../shellBreakpoint";
import { documentScrollBottom, elementScrollBottom } from "./transcriptScrollPosition";

type Position = {
  width: number;
  height: number;
  atTail: boolean;
  row: HTMLElement | null;
  offset: number;
};

/** Carry the reader between the desktop scrollport and document scroll, and
 * preserve their row when wrapping changes within either shell. Resize events
 * arrive after reflow, so the anchor must come from the preceding layout. */
export function useTranscriptResizeAnchor(p: {
  scrollerRef: RefObject<HTMLElement | null>;
  stickToBottomRef: RefObject<boolean>;
  enabled: boolean;
  resetKey: string;
  onRestored: () => void;
}) {
  const current = useRef(p);
  current.current = p;
  const position = useRef<Position | null>(null);
  const frame = useRef<number | null>(null);

  const isResizing = useCallback(() => {
    const saved = position.current;
    return frame.current !== null || !!saved &&
      (saved.width !== window.innerWidth || saved.height !== window.innerHeight);
  }, []);

  const rememberPosition = useCallback(() => {
    if (!current.current.enabled || isResizing()) return;
    const scroller = current.current.scrollerRef.current;
    if (!scroller) return;
    const top = snapshotShellStack() ? 0 : scroller.getBoundingClientRect().top;
    const atTail = current.current.stickToBottomRef.current;
    const row = atTail ? null : Array.from(
      scroller.querySelectorAll<HTMLElement>("[data-row-id]"),
    ).find((el) => el.getBoundingClientRect().bottom > top) ?? null;
    position.current = {
      width: window.innerWidth,
      height: window.innerHeight,
      atTail,
      row,
      offset: row ? row.getBoundingClientRect().top - top : 0,
    };
  }, [isResizing]);

  const restore = useCallback(() => {
    const saved = position.current;
    const scroller = current.current.scrollerRef.current;
    if (!saved || !scroller) return;
    const docScroll = snapshotShellStack();
    if (saved.atTail) {
      if (docScroll) window.scrollTo({ top: documentScrollBottom(window), behavior: "auto" });
      else scroller.scrollTop = elementScrollBottom(scroller);
    } else if (saved.row?.isConnected) {
      const top = docScroll ? 0 : scroller.getBoundingClientRect().top;
      const delta = saved.row.getBoundingClientRect().top - top - saved.offset;
      if (Math.abs(delta) < 0.5) return;
      if (docScroll) window.scrollBy({ top: delta, behavior: "auto" });
      else scroller.scrollTop += delta;
    }
  }, []);

  const onResize = useCallback(() => {
    if (!current.current.enabled || !position.current || !isResizing()) return;
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    // Restore before the render window examines its sentinels. Settle again
    // after the shell commit and deferred composer-reserve measurement.
    restore();
    frame.current = requestAnimationFrame(() => {
      restore();
      frame.current = requestAnimationFrame(() => {
        restore();
        frame.current = null;
        position.current = null;
        rememberPosition();
        current.current.onRestored();
      });
    });
  }, [isResizing, rememberPosition, restore]);

  useLayoutEffect(() => {
    position.current = null;
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    frame.current = null;
    rememberPosition();
  }, [p.resetKey, p.enabled, rememberPosition]);

  useLayoutEffect(() => {
    if (isResizing() && frame.current === null) onResize();
    else rememberPosition();
  });

  useLayoutEffect(() => {
    window.addEventListener("resize", onResize, true);
    return () => {
      window.removeEventListener("resize", onResize, true);
      if (frame.current !== null) cancelAnimationFrame(frame.current);
    };
  }, [onResize]);

  return { isResizing, rememberPosition };
}
