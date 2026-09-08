import * as vscode from "vscode";
import {
  DISABLED_CONFIG,
  suggestsWhileTyping,
  type AutocompleteClientConfig,
} from "./clientConfig";
import { fetchClientConfig, fetchCompletion, sendFeedback } from "./completionClient";
import { advanceCached, type CachedSuggestion } from "./suggestionCache";
import { shouldRequestAutomatically } from "./triggerPolicy";

const CONFIG_POLL_MS = 30_000;

/** Command attached to every suggestion; VS Code runs it when the item is accepted, which is
 *  the only stable signal of acceptance the inline-completion API offers. */
const ACCEPTED_COMMAND = "foxxycode.autocomplete.accepted";

/**
 * LLM-backed inline code completion: the greyed suggestion VS Code draws ahead of the caret and
 * accepts with Tab.
 *
 * VS Code supplies the rendering, the Tab accept and the dismissal, so this class only supplies
 * the things that make an LLM (rather than a local model) usable behind it:
 *
 *  - **a prefix cache** - typing the characters a suggestion already predicted answers locally
 *    instead of asking again;
 *  - **a debounce plus one in-flight request** - a newer keystroke aborts the older call, which
 *    drops the socket and kills the upstream LLM call with it;
 *  - **trigger honouring** - with `autocomplete.trigger: manual` only an explicit invoke
 *    (Alt+\ / `editor.action.inlineSuggest.trigger`) asks the model, and automatic requests
 *    are skipped where a suggestion cannot be right (inside a word, right after a closer);
 *  - **outcome reporting** - shown / accepted / cache_hit are posted to the backend counters, so
 *    the feature can be judged by numbers rather than by feel.
 *
 *  Mirrors the lifecycle of `ide/editorStateService.ts`: `startIfNeeded(baseUrl)` wires things up
 *  and `dispose()` tears them down. All settings come from the backend's config.autocomplete, so
 *  the extension contributes none of its own.
 */
export class InlineCompletionService {
  private baseUrl: string | null = null;
  private registration: vscode.Disposable | null = null;
  private acceptedCommand: vscode.Disposable | null = null;
  private configTimer: ReturnType<typeof setInterval> | null = null;
  private config: AutocompleteClientConfig = DISABLED_CONFIG;
  private cached: CachedSuggestion | null = null;
  private inFlight: AbortController | null = null;
  /** Automatic requests stay off until this time after the provider rate-limited us. */
  private pausedUntil = 0;

  constructor(private readonly log?: (line: string) => void) {}

  /** Starts (or, after a server restart on a new port, re-points) inline completion. */
  startIfNeeded(baseUrl: string): void {
    this.baseUrl = baseUrl;
    if (!this.registration) {
      this.registration = vscode.languages.registerInlineCompletionItemProvider(
        { pattern: "**" },
        { provideInlineCompletionItems: (d, p, c, t) => this.provide(d, p, c, t) },
      );
    }
    if (!this.acceptedCommand) {
      this.acceptedCommand = vscode.commands.registerCommand(ACCEPTED_COMMAND, () =>
        this.report("accepted"),
      );
    }
    if (!this.configTimer) {
      this.configTimer = setInterval(() => void this.refreshConfig(), CONFIG_POLL_MS);
    }
    void this.refreshConfig();
  }

  dispose(): void {
    if (this.configTimer) {
      clearInterval(this.configTimer);
      this.configTimer = null;
    }
    this.registration?.dispose();
    this.registration = null;
    this.acceptedCommand?.dispose();
    this.acceptedCommand = null;
    this.inFlight?.abort();
    this.inFlight = null;
    this.baseUrl = null;
    this.cached = null;
    this.config = DISABLED_CONFIG;
  }

  private report(event: string): void {
    if (this.baseUrl) sendFeedback(this.baseUrl, event);
  }

  private async refreshConfig(): Promise<void> {
    if (!this.baseUrl) return;
    const fetched = await fetchClientConfig(this.baseUrl);
    if (!fetched) return;
    if (fetched.enabled !== this.config.enabled || fetched.trigger !== this.config.trigger) {
      this.log?.(
        `[foxxycode] autocomplete: enabled=${fetched.enabled} trigger=${fetched.trigger}`,
      );
    }
    this.config = fetched;
  }

  private async provide(
    document: vscode.TextDocument,
    position: vscode.Position,
    context: vscode.InlineCompletionContext,
    token: vscode.CancellationToken,
  ): Promise<vscode.InlineCompletionItem[] | undefined> {
    const cfg = this.config;
    const base = this.baseUrl;
    if (!cfg.enabled || !base) return undefined;

    const invoked = context.triggerKind === vscode.InlineCompletionTriggerKind.Invoke;
    if (!invoked && !suggestsWhileTyping(cfg)) return undefined;

    const offset = document.offsetAt(position);
    const uri = document.uri.toString();
    const fullText = document.getText();

    // Free path first: the user is typing out what was already suggested.
    const advanced = advanceCached(this.cached, uri, offset, fullText);
    if (advanced !== null) {
      if (advanced.length === 0) return undefined;
      this.report("cache_hit");
      return [this.item(advanced, position)];
    }

    if (!invoked) {
      // The provider rate-limited us a moment ago: automatic requests wait it out (an explicit
      // invoke still goes through, because then the user has asked).
      if (Date.now() < this.pausedUntil) return undefined;
      const lineBefore = document.getText(new vscode.Range(position.line, 0, position.line, position.character));
      const charAfter = fullText.charAt(offset);
      if (!shouldRequestAutomatically(lineBefore, charAfter)) return undefined;
      if (cfg.debounceMs > 0) {
        const survived = await delay(cfg.debounceMs, token);
        if (!survived) return undefined;
      }
    }
    if (token.isCancellationRequested) return undefined;

    this.inFlight?.abort();
    const controller = new AbortController();
    this.inFlight = controller;
    const cancelSub = token.onCancellationRequested(() => controller.abort());

    let result: { text: string; pauseMs?: number };
    try {
      result = await fetchCompletion(
        base,
        {
          prefix: document.getText(
            new vscode.Range(document.positionAt(Math.max(0, offset - cfg.maxPrefixBytes)), position),
          ),
          suffix: document.getText(
            new vscode.Range(
              position,
              document.positionAt(Math.min(fullText.length, offset + cfg.maxSuffixBytes)),
            ),
          ),
          path: document.uri.fsPath,
          language: document.languageId,
        },
        cfg.timeoutMs,
        controller.signal,
      );
    } finally {
      cancelSub.dispose();
      if (this.inFlight === controller) this.inFlight = null;
    }

    if (result.pauseMs) {
      this.pausedUntil = Date.now() + result.pauseMs;
      this.log?.(`[foxxycode] autocomplete: provider rate limit, pausing automatic requests for ${Math.round(result.pauseMs / 1000)}s`);
    }
    const text = result.text;
    if (!text || token.isCancellationRequested) return undefined;

    this.cached = { uri, offset, text };
    this.report("shown");
    return [this.item(text, position)];
  }

  private item(text: string, position: vscode.Position): vscode.InlineCompletionItem {
    const item = new vscode.InlineCompletionItem(text, new vscode.Range(position, position));
    item.command = { command: ACCEPTED_COMMAND, title: "FoxxyCode: suggestion accepted" };
    return item;
  }
}

/** Waits `ms`, resolving false if the request was cancelled while waiting. */
function delay(ms: number, token: vscode.CancellationToken): Promise<boolean> {
  return new Promise((resolve) => {
    const timer = setTimeout(() => {
      sub.dispose();
      resolve(!token.isCancellationRequested);
    }, ms);
    const sub = token.onCancellationRequested(() => {
      clearTimeout(timer);
      resolve(false);
    });
  });
}
