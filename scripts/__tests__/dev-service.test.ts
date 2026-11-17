import { afterEach, describe, expect, it } from "bun:test";
import { rm } from "node:fs/promises";
import { resolve } from "node:path";
import { createServiceSandbox } from "./helpers";

const roots: string[] = [];
afterEach(async () => {
  await Promise.all(roots.splice(0).map((path) => rm(path, { recursive: true, force: true })));
});

describe("dev service environment", () => {
  it("keeps observer output local while preserving harness discovery and the assigned ingester endpoint", async () => {
    const root = await createServiceSandbox();
    roots.push(root);
    const child = Bun.spawn(["sh", resolve(import.meta.dir, "../dev-service.sh")], {
      cwd: root,
      env: {
        PATH: process.env.PATH,
        DEVHOST_SERVICE_NAME: "observer",
        AGENT_DEV_LOG_LEVEL: "debug",
        AGENT_LOG_FILE: "/outside/shared.log",
        AGENT_SESSION_STATE_PATH: "/outside/state.json",
        XDG_DATA_HOME: "/harness/data",
        XDG_CACHE_HOME: "/outside/cache",
        XDG_RUNTIME_DIR: "/inherited/run",
        AGENT_INGESTER_WS_URL: "ws://127.0.0.1:54321/ws/ingest",
      },
      stdout: "pipe",
      stderr: "pipe",
    });
    const output: unknown = JSON.parse(await new Response(child.stdout).text());
    expect(await child.exited).toBe(0);
    expect(output).toEqual({
      args: ["--log-level", "debug"],
      data: "/harness/data",
      cache: root + "/.tmp/dev-stack/cache",
      runtime: "/inherited/run",
      logs: root + "/.tmp/dev-stack/logs",
      logFile: null,
      sessionState: null,
      ws: "ws://127.0.0.1:54321/ws/ingest",
    });
  });

  it("configures the server listener, shared data and socket, public origin, and mutation endpoint", async () => {
    const root = await createServiceSandbox();
    roots.push(root);
    const child = Bun.spawn(["sh", resolve(import.meta.dir, "../dev-service.sh")], {
      cwd: root,
      env: {
        PATH: process.env.PATH,
        DEVHOST_SERVICE_NAME: "server",
        DEVHOST_BIND_HOST: "127.0.0.1",
        PORT: "54322",
        AGENT_DEV_ORIGIN: "https://dashboard.example",
        AGENT_INGESTER_HTTP_URL: "http://127.0.0.1:54321",
        AGENT_LOG_FILE: "/outside/shared.log",
        AGENT_SESSION_STATE_PATH: "/outside/state.json",
      },
      stdout: "pipe",
      stderr: "pipe",
    });
    const output: unknown = JSON.parse(await new Response(child.stdout).text());
    expect(await child.exited).toBe(0);
    expect(output).toEqual({
      args: [
        "--addr",
        "127.0.0.1:54322",
        "--cors-origin",
        "https://dashboard.example",
        "--log-level",
        "info",
      ],
      data: root + "/.tmp/dev-stack/data",
      runtime: root + "/.tmp/dev-stack/run",
      logs: root + "/.tmp/dev-stack/logs",
      logFile: null,
      sessionState: null,
      http: "http://127.0.0.1:54321",
    });
  });

  it("configures the ingester with its assigned loopback listener and local output paths", async () => {
    const root = await createServiceSandbox();
    roots.push(root);
    const child = Bun.spawn(["sh", resolve(import.meta.dir, "../dev-service.sh")], {
      cwd: root,
      env: {
        PATH: process.env.PATH,
        DEVHOST_SERVICE_NAME: "ingester",
        DEVHOST_BIND_HOST: "127.0.0.1",
        PORT: "54323",
      },
      stdout: "pipe",
      stderr: "pipe",
    });
    const output: unknown = JSON.parse(await new Response(child.stdout).text());
    expect(await child.exited).toBe(0);
    expect(output).toEqual({
      args: ["--addr", "127.0.0.1:54323", "--log-level", "info"],
      data: root + "/.tmp/dev-stack/data",
      runtime: root + "/.tmp/dev-stack/run",
      logs: root + "/.tmp/dev-stack/logs",
      logFile: null,
      sessionState: null,
    });
  });
});
