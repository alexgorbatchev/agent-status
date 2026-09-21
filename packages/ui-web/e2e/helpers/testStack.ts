import { spawn, type ChildProcess } from "node:child_process";
import { createServer } from "node:net";
import { existsSync, mkdirSync, rmSync, readFileSync, writeFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
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
  createClaudeSession: (sessionId: string, pid: number, cwd: string) => Promise<string>;
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

  const dbPath = resolve(testDir, "db.sqlite");
  const notifySock = resolve(testDir, "n.sock");
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

  const ingesterBin = resolve(rootDir, "packages", "agent-ingester", "bin", "air-ingester");
  const serverBin = resolve(rootDir, "packages", "agent-server", "bin", "air-server");
  const observerBin = resolve(rootDir, "packages", "agent-observer", "bin", "air-observer");

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
        "error",
      ],
      {
        cwd: rootDir,
        env: {
          ...process.env,
          XDG_DATA_HOME: testDir,
          XDG_RUNTIME_DIR: testDir,
        },
        stdio: "ignore",
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
        "error",
      ],
      {
        cwd: rootDir,
        env: {
          ...process.env,
          XDG_DATA_HOME: testDir,
          XDG_RUNTIME_DIR: testDir,
        },
        stdio: "ignore",
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
        "error",
      ],
      {
        cwd: rootDir,
        env: {
          ...process.env,
          AGENT_STATUS_TEST_MOCK_PROC: "1",
        },
        stdio: "ignore",
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

  const createClaudeSession = async (sessionId: string, pid: number, cwd: string) => {
    const desc = {
      pid,
      sessionId,
      cwd,
      startedAt: Date.now(),
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
