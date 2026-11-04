import { spawn, type ChildProcess, type StdioOptions } from "node:child_process";
import { createServer } from "node:net";
import { existsSync, mkdirSync, openSync, rmSync, readFileSync, writeFileSync } from "node:fs";
import { basename, resolve, dirname, relative } from "node:path";
import { fileURLToPath } from "node:url";

const currentDir = dirname(fileURLToPath(import.meta.url));

export interface ITestStack {
  rootDir: string;
  testDir: string;
  dbPath: string;
  notifySock: string;
  claudeSessionsDir: string;
  claudeProjectsDir: string;
  piSessionsDir: string;
  ingesterPort: number;
  serverPort: number;
  uiPort: number;
  token: string;
  start: () => Promise<void>;
  stop: () => Promise<void>;
  createClaudeSession: (sessionId: string, cwd: string) => Promise<string>;
  appendClaudeTranscript: (sessionId: string, cwd: string, lineObj: Record<string, unknown>) => Promise<void>;
  createPiSession: (sessionId: string, cwd: string) => Promise<string>;
  appendPiTranscript: (filePath: string, lineObj: Record<string, unknown>) => Promise<void>;
}

async function getAvailablePort(): Promise<number> {
  return new Promise((res, rej) => {
    const srv = createServer();
    srv.listen(0, "127.0.0.1", () => {
      const addr = srv.address();
      if (addr && typeof addr === "object") {
        const port = addr.port;
        srv.close(() => res(port));
      } else {
        srv.close(() => rej(new Error("Unable to obtain free port")));
      }
    });
    srv.on("error", rej);
  });
}

export async function createTestStack(testId: string): Promise<ITestStack> {
  const rootDir = resolve(currentDir, "../../../..");
  const testDir = resolve(rootDir, ".tmp", `e2e-${testId}-${Date.now().toString(36)}`);

  if (existsSync(testDir)) {
    rmSync(testDir, { recursive: true, force: true });
  }
  mkdirSync(testDir, { recursive: true });
  // Daemon output outlives stop() so a failed run can be diagnosed.
  const logDir = resolve(rootDir, ".tmp", "e2e-logs", basename(testDir));
  mkdirSync(logDir, { recursive: true });
  const daemonOutput = (name: string): StdioOptions => {
    const fd = openSync(resolve(logDir, `${name}.log`), "a");
    return ["ignore", fd, fd];
  };

  const dbPath = resolve(testDir, "db.sqlite");
  // Unix socket addresses are limited to about 104 bytes, which an absolute path
  // inside a deep checkout exceeds. The daemons run from rootDir, so pass the
  // socket relative to it, as the dev stack does.
  const notifySock = relative(rootDir, resolve(testDir, "n.sock"));
  const claudeSessionsDir = resolve(testDir, "claude-sessions");
  const claudeProjectsDir = resolve(testDir, "claude-projects");
  const piSessionsDir = resolve(testDir, "pi-sessions");
  const cursorPath = resolve(testDir, "cursors.json");

  mkdirSync(claudeSessionsDir, { recursive: true });
  mkdirSync(claudeProjectsDir, { recursive: true });
  mkdirSync(piSessionsDir, { recursive: true });

  const ingesterPort = await getAvailablePort();
  const serverPort = await getAvailablePort();
  const uiPort = Number(process.env.PLAYWRIGHT_PORT) || 3100;
  const token = "dev-secret-token";

  const ingesterBin = resolve(rootDir, "packages", "agent-ingester", "bin", "agent-ingester");
  const serverBin = resolve(rootDir, "packages", "agent-server", "bin", "agent-server");
  const observerBin = resolve(rootDir, "packages", "agent-observer", "bin", "agent-observer");

  const procs: ChildProcess[] = [];

  const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

  const start = async () => {
    // 1. Ingester
    const ingester = spawn(
      ingesterBin,
      [
        "--addr",
        `127.0.0.1:${ingesterPort}`,
        "--db",
        dbPath,
        "--notify-sock",
        notifySock,
        "--token",
        token,
        "--no-summarize",
        "--log-level",
        "info",
      ],
      {
        cwd: rootDir,
        env: {
          ...process.env,
          XDG_DATA_HOME: testDir,
          XDG_STATE_HOME: testDir,
          XDG_RUNTIME_DIR: testDir,
        },
        stdio: daemonOutput("ingester"),
      },
    );
    procs.push(ingester);
    await sleep(600);

    // 2. Server
    const server = spawn(
      serverBin,
      [
        "--addr",
        `127.0.0.1:${serverPort}`,
        "--db",
        dbPath,
        "--socket",
        notifySock,
        "--token",
        token,
        "--cors-origin",
        `http://localhost:${uiPort}`,
        "--log-level",
        "info",
      ],
      {
        cwd: rootDir,
        env: {
          ...process.env,
          XDG_DATA_HOME: testDir,
          XDG_STATE_HOME: testDir,
          XDG_RUNTIME_DIR: testDir,
        },
        stdio: daemonOutput("server"),
      },
    );
    procs.push(server);
    await sleep(600);

    // 3. Observer
    const observer = spawn(
      observerBin,
      [
        "--ingester-url",
        `ws://127.0.0.1:${ingesterPort}/ws/ingest`,
        "--token",
        token,
        "--env",
        "test-runner",
        "--scan-interval",
        "200ms",
        "--cursor-path",
        cursorPath,
        "--claude-sessions-dir",
        claudeSessionsDir,
        "--claude-projects-dir",
        claudeProjectsDir,
        "--pi-sessions-dir",
        piSessionsDir,
        "--log-level",
        "info",
      ],
      {
        cwd: rootDir,
        env: {
          ...process.env,
          // Harness defaults without a flag (Codex, OpenCode) resolve under HOME;
          // keep the observer away from the developer's real sessions.
          HOME: testDir,
          XDG_DATA_HOME: testDir,
          XDG_STATE_HOME: testDir,
          XDG_RUNTIME_DIR: testDir,
        },
        stdio: daemonOutput("observer"),
      },
    );
    procs.push(observer);
    await sleep(600);
  };

  const stop = async () => {
    for (const p of procs) {
      try {
        p.kill("SIGKILL");
      } catch {
        // already stopped
      }
    }
    if (existsSync(testDir)) {
      rmSync(testDir, { recursive: true, force: true });
    }
  };

  const getClaudeSlug = (cwd: string) => {
    return cwd.replace(/[/.]/g, "-");
  };

  // The observer reports a session only while a matching harness process is
  // alive: Claude Code needs the descriptor's PID started at its startedAt, and
  // Pi needs a process whose command ends in /pi running in the session's cwd.
  // An idle script named after the harness stands in for it, so the observer's
  // production liveness checks apply unchanged.
  const startHarnessStandIn = async (harnessCommand: string, cwd: string) => {
    const binDir = resolve(testDir, "harness-bin");
    mkdirSync(binDir, { recursive: true });
    mkdirSync(cwd, { recursive: true });
    const script = resolve(binDir, harnessCommand);
    writeFileSync(script, "setInterval(() => {}, 1 << 30);\n");
    const standIn = spawn(process.execPath, [script], { cwd, stdio: "ignore" });
    procs.push(standIn);
    await new Promise<void>((res, rej) => {
      standIn.once("spawn", res);
      standIn.once("error", rej);
    });
    if (standIn.pid === undefined) {
      throw new Error("harness stand-in process has no PID");
    }
    return { pid: standIn.pid, startedAt: Date.now() };
  };

  const createClaudeSession = async (sessionId: string, cwd: string) => {
    const { pid, startedAt } = await startHarnessStandIn("claude", cwd);
    const desc = {
      pid,
      sessionId,
      cwd,
      startedAt,
      status: "busy",
      version: "2.1.277",
    };
    const descPath = resolve(claudeSessionsDir, `${pid}.json`);
    writeFileSync(descPath, JSON.stringify(desc));

    const slug = getClaudeSlug(cwd);
    const projDir = resolve(claudeProjectsDir, slug);
    mkdirSync(projDir, { recursive: true });
    const transcriptPath = resolve(projDir, `${sessionId}.jsonl`);
    writeFileSync(transcriptPath, "");
    return transcriptPath;
  };

  const appendClaudeTranscript = async (sessionId: string, cwd: string, lineObj: Record<string, unknown>) => {
    const slug = getClaudeSlug(cwd);
    const transcriptPath = resolve(claudeProjectsDir, slug, `${sessionId}.jsonl`);
    const current = existsSync(transcriptPath) ? readFileSync(transcriptPath, "utf-8") : "";
    writeFileSync(transcriptPath, current + JSON.stringify(lineObj) + "\n");
  };

  const createPiSession = async (sessionId: string, cwd: string) => {
    await startHarnessStandIn("pi", cwd);
    // Encoded CWD for Pi: strip leading slash, replace separators and colons with dash
    const clean = cwd.replace(/^[/\\]+/, "").replace(/[/\\:]/g, "-");
    const folder = `--${clean}--`;
    const projDir = resolve(piSessionsDir, folder);
    mkdirSync(projDir, { recursive: true });
    const transcriptPath = resolve(projDir, `${Date.now()}_${sessionId}.jsonl`);
    const header = {
      type: "session",
      version: 3,
      id: sessionId,
      timestamp: new Date().toISOString(),
      cwd,
    };
    writeFileSync(transcriptPath, JSON.stringify(header) + "\n");
    return transcriptPath;
  };

  const appendPiTranscript = async (filePath: string, lineObj: Record<string, unknown>) => {
    const current = existsSync(filePath) ? readFileSync(filePath, "utf-8") : "";
    writeFileSync(filePath, current + JSON.stringify(lineObj) + "\n");
  };

  return {
    rootDir,
    testDir,
    dbPath,
    notifySock,
    claudeSessionsDir,
    claudeProjectsDir,
    piSessionsDir,
    ingesterPort,
    serverPort,
    uiPort,
    token,
    start,
    stop,
    createClaudeSession,
    appendClaudeTranscript,
    createPiSession,
    appendPiTranscript,
  };
}
