import * as http from "http";
import { EditEvent, parseEditEvent } from "./editEvent";

/** Reads the foxxycode `GET /foxxycode/ide/events` Server-Sent-Events stream on a
 *  background daemon (setInterval-driven) and delivers parsed `EditEvent`s to `onEvent`.
 *  Reconnects with backoff until `stop()` is called.
 *
 *  Pure Node `http` (no extra deps), matching IntelliJ `FoxxyCodeIdeEventClient`. */
export class IdeEventClient {
  private running = false;
  private req: http.ClientRequest | null = null;
  private reconnectTimer: NodeJS.Timeout | null = null;
  private readonly backoffMs: number;
  private pendingData = "";

  constructor(
    private readonly baseUrl: string,
    private readonly onEvent: (ev: EditEvent) => void,
    backoffMs: number = 1500,
  ) {
    this.backoffMs = backoffMs;
  }

  start(): void {
    if (this.running) return;
    this.running = true;
    this.scheduleConnect(0);
  }

  stop(): void {
    this.running = false;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.req) {
      try {
        this.req.destroy();
      } catch {
        // ignore
      }
      this.req = null;
    }
  }

  private scheduleConnect(delayMs: number): void {
    if (!this.running) return;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      if (!this.running) return;
      this.connect();
    }, delayMs);
  }

  private eventsUrl(): string {
    const base = this.baseUrl.endsWith("/") ? this.baseUrl : this.baseUrl + "/";
    return base + "foxxycode/ide/events";
  }

  private connect(): void {
    const url = this.eventsUrl();
    const req = http.get(
      url,
      { headers: { Accept: "text/event-stream" } },
      (res) => {
        if (res.statusCode && (res.statusCode < 200 || res.statusCode >= 300)) {
          res.destroy();
          this.scheduleConnect(this.backoffMs);
          return;
        }
        res.setEncoding("utf8");
        let data = "";
        // SSE: a blank line dispatches the buffered `data:` lines as one event.
        // Comment lines (`:`) are heartbeats and are ignored. We do not use `event:`
        // because the type lives inside each JSON payload.
        res.on("data", (chunk: string) => {
          if (!this.running) {
            res.destroy();
            return;
          }
          data += chunk;
          let nl: number;
          while ((nl = data.indexOf("\n")) >= 0) {
            const rawLine = data.slice(0, nl);
            data = data.slice(nl + 1);
            const line = rawLine.replace(/\r$/, "");
            if (line.startsWith(":")) continue;
            if (line.startsWith("data:")) {
              // accumulate into a pending buffer — see below
              this.pendingData =
                (this.pendingData ? this.pendingData + "\n" : "") + line.slice(5).trimStart();
            } else if (line === "") {
              if (this.pendingData) {
                this.dispatch(this.pendingData);
                this.pendingData = "";
              }
            }
          }
        });
        res.on("end", () => this.scheduleConnect(this.backoffMs));
        res.on("error", () => this.scheduleConnect(this.backoffMs));
      },
    );
    req.setTimeout(0);
    req.on("error", () => this.scheduleConnect(this.backoffMs));
    this.req = req;
  }

  private dispatch(payload: string): void {
    const ev = parseEditEvent(payload);
    if (!ev) return;
    try {
      this.onEvent(ev);
    } catch {
      // never let a handler error kill the SSE loop
    }
  }
}
