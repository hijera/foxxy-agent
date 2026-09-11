import { afterEach, describe, expect, it, vi } from "vitest";
import { probeEnvHealth } from "./activeHealth";

describe("probeEnvHealth", () => {
  it("reports the local environment as up without any network call", async () => {
    expect(await probeEnvHealth({ mode: "local" })).toBe("up");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  /**
   * remoteEnv binds window.fetch once, at module load, so the stub has to be in
   * place before the import - hence the fresh module per case. answers replies one
   * status per URL substring; anything unlisted is a 500.
   */
  async function probeWith(
    answers: Array<[string, number]>,
  ): Promise<{ health: string; asked: string[] }> {
    const asked: string[] = [];
    vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
      const url = String(input);
      asked.push(url);
      const hit = answers.find(([part]) => url.includes(part));
      return Promise.resolve(new Response("", { status: hit ? hit[1] : 500 }));
    });
    vi.resetModules();
    const mod = await import("./activeHealth");
    const health = await mod.probeEnvHealth({
      mode: "remote",
      baseUrl: "http://relay:12346",
      token: "t",
    });
    return { health, asked };
  }

  it("counts a swarm relay as up even though it serves no /v1", async () => {
    // A relay has to be added as a remote to be given its token, and it answers
    // 404 for the agent API it does not have.
    const { health, asked } = await probeWith([
      ["/v1/models", 404],
      ["/swarm/info", 200],
    ]);
    expect(health).toBe("up");
    expect(asked.some((u) => u.includes("/swarm/info"))).toBe(true);
  });

  it("stays down when neither the agent API nor a relay answers", async () => {
    const { health } = await probeWith([
      ["/v1/models", 404],
      ["/swarm/info", 404],
    ]);
    expect(health).toBe("down");
  });

  it("does not ask a relay about a wrong token", async () => {
    // 401 is a credential this environment rejected, not a surface it lacks.
    const { health, asked } = await probeWith([["/v1/models", 401]]);
    expect(health).toBe("down");
    expect(asked.some((u) => u.includes("/swarm/info"))).toBe(false);
  });

  it("reports a working agent API as up without asking about a relay", async () => {
    const { health, asked } = await probeWith([["/v1/models", 200]]);
    expect(health).toBe("up");
    expect(asked.some((u) => u.includes("/swarm/info"))).toBe(false);
  });
});
