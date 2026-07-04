import * as http from "http";

export interface HttpResponse {
  status: number;
  body: string;
}

/** Minimal GET helper with a connect/read timeout. Used for readiness probing
 *  and the `/v1/models` check; not for SSE streaming (see ideEventClient.ts). */
export function httpGet(url: string, timeoutMs: number): Promise<HttpResponse> {
  return new Promise((resolve, reject) => {
    const req = http.get(url, (res) => {
      let body = "";
      res.setEncoding("utf8");
      res.on("data", (chunk: string) => {
        body += chunk;
      });
      res.on("end", () => resolve({ status: res.statusCode ?? 0, body }));
      res.on("error", reject);
    });
    req.setTimeout(timeoutMs, () => {
      req.destroy(new Error(`timeout after ${timeoutMs}ms`));
    });
    req.on("error", reject);
  });
}

export interface PostOptions {
  body: string;
  contentType?: string;
  timeoutMs?: number;
}

/** Fire-and-forget POST helper. Used for the `/foxxycode/sessions/<id>/permission` call. */
export function httpPost(url: string, opts: PostOptions): Promise<HttpResponse> {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    const req = http.request(
      {
        method: "POST",
        hostname: parsed.hostname,
        port: parsed.port,
        path: parsed.pathname + parsed.search,
        headers: {
          "Content-Type": opts.contentType ?? "application/json",
          "Content-Length": Buffer.byteLength(opts.body),
        },
      },
      (res) => {
        let body = "";
        res.setEncoding("utf8");
        res.on("data", (c: string) => (body += c));
        res.on("end", () => resolve({ status: res.statusCode ?? 0, body }));
        res.on("error", reject);
      },
    );
    req.setTimeout(opts.timeoutMs ?? 5000, () => req.destroy(new Error("timeout")));
    req.on("error", reject);
    req.write(opts.body);
    req.end();
  });
}
