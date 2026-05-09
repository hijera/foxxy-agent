import { useEffect, useMemo, useState } from 'react';
import { Markdown } from '../markdown/Markdown';

function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '';
  if (ms >= 60_000) {
    const mins = ms / 60_000;
    const fixed = mins < 10 ? mins.toFixed(1) : mins.toFixed(0);
    return `${fixed}m`;
  }
  return `${Math.round(ms)}ms`;
}

function looksLikeJudgeJSON(raw: string): boolean {
  const t = raw.trim();
  return t.length > 2 && t.startsWith('{') && t.endsWith('}');
}

/** Prefer curator `reason` when judge output is JSON; plain text otherwise. Omit when structured save already owns the section. */
function curatorSummaryFromJudge(raw: string, hasStructuredPersist: boolean): string {
  if (hasStructuredPersist) return '';
  const t = raw.trim();
  if (!t) return '';
  if (looksLikeJudgeJSON(t)) {
    try {
      const o = JSON.parse(t) as { reason?: string };
      return typeof o.reason === 'string' ? o.reason.trim() : '';
    } catch {
      return '';
    }
  }
  return t;
}

export function MemoryCopilotMessage(props: {
  recallStatus: 'idle' | 'in_progress' | 'completed';
  persistStatus: 'idle' | 'in_progress' | 'completed';
  recallText: string;
  recallReasoning: string;
  persistText: string;
  persistReasoning: string;
  recallDurationMs?: number;
  persistDurationMs?: number;
  memoryWallStartedAtMs?: number;
  memoryWallDurationMs?: number;
  persistSaved?: boolean;
  persistSavedBody?: string;
  persistRelativePath?: string;
  persistTitle?: string;
  recallReadPaths?: string[];
}) {
  const recallBusy = props.recallStatus === 'in_progress';
  const persistBusy = props.persistStatus === 'in_progress';
  const busy = recallBusy || persistBusy;
  const label = busy ? 'memory...' : 'memory';

  const recallReadPaths = useMemo(() => {
    const xs = props.recallReadPaths || [];
    const out: string[] = [];
    for (const raw of xs) {
      const t = typeof raw === 'string' ? raw.trim() : '';
      if (t && !out.includes(t)) out.push(t);
    }
    return out;
  }, [props.recallReadPaths]);

  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    if (!busy || typeof props.memoryWallStartedAtMs !== 'number') return;
    const h = window.setInterval(() => setNowMs(Date.now()), 160);
    return () => window.clearInterval(h);
  }, [busy, props.memoryWallStartedAtMs]);

  const durationLabel = useMemo(() => {
    if (typeof props.memoryWallDurationMs === 'number' && Number.isFinite(props.memoryWallDurationMs)) {
      return formatDuration(props.memoryWallDurationMs);
    }
    if (!busy && (props.recallDurationMs || props.persistDurationMs)) {
      const sum = (props.recallDurationMs ?? 0) + (props.persistDurationMs ?? 0);
      return formatDuration(sum);
    }
    if (busy && typeof props.memoryWallStartedAtMs === 'number' && Number.isFinite(props.memoryWallStartedAtMs)) {
      return formatDuration(Math.max(0, nowMs - props.memoryWallStartedAtMs));
    }
    return '-';
  }, [
    busy,
    nowMs,
    props.memoryWallDurationMs,
    props.memoryWallStartedAtMs,
    props.persistDurationMs,
    props.recallDurationMs,
  ]);

  const hasRecallBody = !!(props.recallText || props.recallReasoning || recallReadPaths.length > 0);
  const savedBodyTrim = (props.persistSavedBody || '').trim();
  const hasStructuredSave = !!(props.persistSaved && savedBodyTrim);
  const prTrim = (props.persistReasoning || '').trim();
  const ptTrim = (props.persistText || '').trim();
  const persistComplete = props.persistStatus === 'completed';
  const curatorSummary = persistComplete ? curatorSummaryFromJudge(ptTrim, hasStructuredSave) : '';
  const curatorSummaryTrim = curatorSummary.trim();
  const showJudgeReasoning =
    prTrim !== '' && (persistBusy || (persistComplete && !curatorSummaryTrim && !looksLikeJudgeJSON(ptTrim)));

  const showPersistPhase = props.persistStatus !== 'idle';
  const showPersistSkippedLine =
    persistComplete &&
    !props.persistSaved &&
    !hasStructuredSave &&
    !curatorSummaryTrim &&
    !showJudgeReasoning &&
    !!(ptTrim || prTrim || persistBusy);

  const showPersistNothingLine =
    persistComplete &&
    !props.persistSaved &&
    !hasStructuredSave &&
    !curatorSummaryTrim &&
    !showJudgeReasoning &&
    !persistBusy &&
    !ptTrim &&
    !prTrim;

  return (
    <div className="thinking-row">
      <details className="thinking-details" data-testid="memory-copilot-row">
        <summary className="thinking-summary" aria-label="Memory copilot summary">
          <span className="thinking-left">
            <span className="thinking-chevron" aria-hidden="true" />
            <span className="thinking-label">{label}</span>
            <span className="thinking-dur" aria-hidden="true">
              {durationLabel}
            </span>
          </span>
        </summary>
        <div className="thinking-body coddy-memory-copilot-body" aria-label="Memory copilot content">
          {props.recallStatus !== 'idle' || hasRecallBody ? (
            <div className="coddy-memory-phase coddy-memory-recall">
              {recallReadPaths.length > 0 ? (
                <p className="coddy-memory-read-files coddy-memory-hint muted">
                  Notes read: {recallReadPaths.join(', ')}
                </p>
              ) : null}
              {props.recallReasoning ? (
                <div className="coddy-memory-stream coddy-memory-reasoning md-wrap">
                  <Markdown text={props.recallReasoning} />
                </div>
              ) : null}
              {props.recallText ? (
                <div className="coddy-memory-stream md-wrap coddy-memory-for-agent">
                  <Markdown text={props.recallText} />
                </div>
              ) : null}
              {!props.recallText &&
              !props.recallReasoning &&
              props.recallStatus === 'completed' &&
              recallReadPaths.length === 0 ? (
                <p className="coddy-memory-empty">No relevant notes matched this turn.</p>
              ) : null}
            </div>
          ) : null}
          {showPersistPhase ? (
            <div className="coddy-memory-phase coddy-memory-persist">
              {hasStructuredSave ? (
                <>
                  <div className="coddy-memory-saved-summary">
                    {(props.persistTitle || '').trim() ? <span className="coddy-memory-saved-title">{(props.persistTitle || '').trim()}</span> : null}
                    {props.persistRelativePath ? (
                      <span className="coddy-memory-path">{props.persistRelativePath}</span>
                    ) : null}
                  </div>
                  <div className="coddy-memory-stream md-wrap">
                    <Markdown text={savedBodyTrim} />
                  </div>
                </>
              ) : null}
              {!hasStructuredSave && props.persistSaved ? (
                <p className="coddy-memory-saved">Marked saved ({(props.persistTitle || '').trim() || 'note'}).</p>
              ) : null}
              {persistBusy && !hasStructuredSave ? (
                <p className="coddy-memory-hint muted">Judge is evaluating whether to persist…</p>
              ) : null}
              {showJudgeReasoning ? (
                <div className="coddy-memory-stream coddy-memory-reasoning md-wrap">
                  <Markdown text={props.persistReasoning} />
                </div>
              ) : null}
              {persistComplete && curatorSummaryTrim ? (
                <div className="coddy-memory-stream md-wrap coddy-memory-curator-summary">
                  <Markdown text={curatorSummaryTrim} />
                </div>
              ) : null}
              {showPersistNothingLine ? <p className="coddy-memory-empty">Nothing new was committed to notes.</p> : null}
              {showPersistSkippedLine ? (
                <p className="coddy-memory-empty">Curator skipped a write for this turn.</p>
              ) : null}
            </div>
          ) : null}
        </div>
      </details>
    </div>
  );
}
