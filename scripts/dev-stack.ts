import { spawn, type Subprocess } from "bun";
import { existsSync, mkdirSync, unlinkSync } from "node:fs";
import { resolve } from "node:path";

const rootDir = resolve(import.meta.dir, "..");
const runDir = resolve(rootDir, ".tmp", "dev-stack");
if (!existsSync(runDir)) {
  mkdirSync(runDir, { recursive: true });
}

const pidFile = resolve(runDir, "pids.json");

export function getAction(argv: string[] = process.argv): string {
  const nonFlagArgs = argv.slice(2).filter((arg) => !arg.startsWith("-"));
  return nonFlagArgs[0] || "start";
}

const action = getAction();

export function shouldSkipBuild(argv: string[] = process.argv): boolean {
  return argv.includes("--no-build");
}

export interface IBuildOptions {
  noBuild?: boolean;
  rootDir?: string;
  isAgent?: boolean;
  spawnFn?: (
    cmd: string[],
    options: { cwd?: string; stdout?: "pipe" | "inherit"; stderr?: "pipe" | "inherit" }
  ) => {
    exited: Promise<number>;
    stdout?: ReadableStream<Uint8Array> | null;
    stderr?: ReadableStream<Uint8Array> | null;
  };
  logFn?: (message: string) => void;
  errorFn?: (message: string) => void;
}

export interface IBuildResult {
  isSkipped: boolean;
  exitCode: number;
  stdoutText: string;
  stderrText: string;
}

export async function buildBinaries(options: IBuildOptions = {}): Promise<IBuildResult> {
  const log = options.logFn ?? console.log;
  const error = options.errorFn ?? console.error;
  const isAgent = options.isAgent ?? isAgentMode();
  const targetRootDir = options.rootDir ?? rootDir;

  if (options.noBuild) {
    if (isAgent) {
      log("[dev-stack] INFO: skipping binary build (--no-build)");
    } else {
      log("[INFO] Skipping binary build (--no-build specified)...");
    }
    return {
      isSkipped: true,
      exitCode: 0,
      stdoutText: "",
      stderrText: "",
    };
  }

  if (isAgent) {
    log("[dev-stack] INFO: building binaries");
  } else {
    log("[INFO] Building Go binaries...");
  }

  const spawn = options.spawnFn ?? Bun.spawn;
  const buildProc = spawn(["just", "build-all"], {
    cwd: targetRootDir,
    stdout: "pipe",
    stderr: "pipe",
  });

  const [stdoutText, stderrText] = await Promise.all([
    buildProc.stdout ? new Response(buildProc.stdout).text() : Promise.resolve(""),
    buildProc.stderr ? new Response(buildProc.stderr).text() : Promise.resolve(""),
  ]);

  const exitCode = await buildProc.exited;

  if (exitCode !== 0) {
    if (stdoutText.trim().length > 0) {
      error(stdoutText);
    }
    if (stderrText.trim().length > 0) {
      error(stderrText);
    }
    error(`[ERROR] Build failed with exit code ${exitCode}`);
  }

  return {
    isSkipped: false,
    exitCode,
    stdoutText,
    stderrText,
  };
}

function isAgentMode(): boolean {
  const agent = process.env.AGENT?.trim().toLowerCase();
  return agent === "1" || agent === "true" || agent === "yes";
}

function getLogLevel(): string {
  const args = process.argv.slice(2);
  if (args.includes("--debug") || args.includes("-d")) {
    return "debug";
  }
  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--log-level" && i + 1 < args.length) {
      return args[i + 1];
    }
    if (args[i].startsWith("--log-level=")) {
      return args[i].split("=")[1];
    }
  }
  return "info";
}

async function getRunningPIDs(): Promise<number[]> {
  if (!existsSync(pidFile)) return [];
  try {
    const data = await Bun.file(pidFile).json();
    return Array.isArray(data) ? (data as number[]) : [];
  } catch {
    return [];
  }
}

async function pipeOutput(stream: ReadableStream<Uint8Array> | null, prefix: string) {
  if (!stream) return;
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";
      for (const line of lines) {
        if (line.length > 0) {
          console.log(`${prefix} ${line}`);
        }
      }
    }
    if (buffer.length > 0) {
      console.log(`${prefix} ${buffer}`);
    }
  } catch {
    // stream closed
  }
}

async function stopStack() {
  if (isAgentMode()) {
    console.log("[dev-stack] INFO: stopping agent-status stack");
  } else {
    console.log("[INFO] Stopping agent-status stack...");
  }
  const text = existsSync(pidFile) ? await Bun.file(pidFile).text() : "[]";
  let pids: number[] = [];
  try {
    pids = JSON.parse(text);
  } catch {
    pids = [];
  }

  for (const pid of pids) {
    try {
      process.kill(pid, "SIGTERM");
      if (!isAgentMode()) {
        console.log(`   Killed PID ${pid}`);
      }
    } catch {
      // already stopped
    }
  }

  // Also kill any remaining processes listening on the standard ports (3000, 4501, 4502)
  for (const port of [3000, 4501, 4502]) {
    try {
      const proc = Bun.spawn(["lsof", "-t", `-i:${port}`], { stdout: "pipe" });
      const out = await new Response(proc.stdout).text();
      const portPids = out.trim().split("\n").map(Number).filter(Boolean);
      for (const p of portPids) {
        if (p !== process.pid) {
          process.kill(p, "SIGKILL");
          if (!isAgentMode()) {
            console.log(`   Cleaned up port ${port} (PID ${p})`);
          }
        }
      }
    } catch {
      // ignore
    }
  }

  if (existsSync(pidFile)) {
    unlinkSync(pidFile);
  }
  if (isAgentMode()) {
    console.log("[dev-stack] OK: stack stopped");
  } else {
    console.log("[OK] Stack stopped.");
  }
}

async function startStack() {
  // Ensure previous stack is stopped
  await stopStack();

  if (isAgentMode()) {
    console.log("[dev-stack] INFO: starting agent-status stack");
  } else {
    console.log("[INFO] Starting agent-status stack (ingester, server, observer, ui)...");
  }

  // 1. Build Go binaries
  const ingesterBin = resolve(rootDir, "packages", "agent-ingester", "bin", "air-ingester");
  const serverBin = resolve(rootDir, "packages", "agent-server", "bin", "air-server");
  const observerBin = resolve(rootDir, "packages", "agent-observer", "bin", "air-observer");

  const buildResult = await buildBinaries({
    noBuild: shouldSkipBuild(),
    rootDir,
    isAgent: isAgentMode(),
  });
  if (buildResult.exitCode !== 0) {
    process.exit(buildResult.exitCode);
  }

  const pids: number[] = [];
  const procs: Subprocess[] = [];

  const logLevel = getLogLevel();
  const dbDir = resolve(runDir, "data");
  mkdirSync(dbDir, { recursive: true });
  const notifySock = "/tmp/agent-status/notify.sock";

  const agent = isAgentMode();
  const ingesterPrefix = agent ? "[ingester]" : "\x1b[34m[ingester]\x1b[0m";
  const serverPrefix = agent ? "[server]" : "\x1b[35m[server]\x1b[0m";
  const observerPrefix = agent ? "[observer]" : "\x1b[33m[observer]\x1b[0m";
  const uiPrefix = agent ? "[ui]" : "\x1b[32m[ui]\x1b[0m";

  // 1. Ingester
  if (!agent) {
    console.log("   [1/4] Starting air-ingester (:4501)...");
  }
  const ingester = Bun.spawn([ingesterBin, "--no-summarize", "--log-level", logLevel], {
    cwd: rootDir,
    env: {
      ...process.env,
      AIR_INGESTER_ADDR: ":4501",
      AIR_BEARER_TOKEN: "dev-secret-token",
      AIR_NOTIFY_SOCK: notifySock,
      XDG_DATA_HOME: dbDir,
      XDG_RUNTIME_DIR: "/tmp",
    },
    stdout: "pipe",
    stderr: "pipe",
  });
  pids.push(ingester.pid);
  procs.push(ingester);
  pipeOutput(ingester.stdout, ingesterPrefix);
  pipeOutput(ingester.stderr, ingesterPrefix);

  // Wait 500ms for ingester to bind socket
  await Bun.sleep(500);

  // 2. Server
  if (!agent) {
    console.log("   [2/4] Starting air-server (:4502)...");
  }
  const server = Bun.spawn([serverBin, "--cors-origin", "http://localhost:3000", "--log-level", logLevel], {
    cwd: rootDir,
    env: {
      ...process.env,
      AIR_SERVER_ADDR: ":4502",
      AGENT_STATUS_TOKEN: "dev-secret-token",
      AIR_NOTIFY_SOCK: notifySock,
      XDG_DATA_HOME: dbDir,
      XDG_RUNTIME_DIR: "/tmp",
    },
    stdout: "pipe",
    stderr: "pipe",
  });
  pids.push(server.pid);
  procs.push(server);
  pipeOutput(server.stdout, serverPrefix);
  pipeOutput(server.stderr, serverPrefix);

  await Bun.sleep(500);

  // 3. Observer
  if (!agent) {
    console.log("   [3/4] Starting air-observer...");
  }
  const observer = Bun.spawn([observerBin, "--token", "dev-secret-token", "--ingester-url", "ws://127.0.0.1:4501/ws/ingest", "--log-level", logLevel], {
    cwd: rootDir,
    env: {
      ...process.env,
      AGENT_STATUS_TOKEN: "dev-secret-token",
      XDG_CACHE_HOME: resolve(runDir, "cache"),
    },
    stdout: "pipe",
    stderr: "pipe",
  });
  pids.push(observer.pid);
  procs.push(observer);
  pipeOutput(observer.stdout, observerPrefix);
  pipeOutput(observer.stderr, observerPrefix);

  // 4. UI Web
  if (!agent) {
    console.log("   [4/4] Starting ui-web (http://localhost:3000)...");
  }
  const uiWebDir = resolve(rootDir, "packages", "ui-web");
  const ui = Bun.spawn(["bun", "run", "src/server.ts"], {
    cwd: uiWebDir,
    env: {
      ...process.env,
      PORT: "3000",
    },
    stdout: "pipe",
    stderr: "pipe",
  });
  pids.push(ui.pid);
  procs.push(ui);
  pipeOutput(ui.stdout, uiPrefix);
  pipeOutput(ui.stderr, uiPrefix);

  await Bun.write(pidFile, JSON.stringify(pids, null, 2));

  if (agent) {
    console.log("[dev-stack] INFO: agent-status running at http://localhost:3000");
  } else {
    console.log("\n[INFO] agent-status is running at http://localhost:3000");
    console.log("       Press Ctrl+C to stop or run: just stop\n");
  }

  const shutdown = async () => {
    await stopStack();
    process.exit(0);
  };

  process.on("SIGINT", shutdown);
  process.on("SIGTERM", shutdown);

  // Keep foreground process alive
  await Promise.race(procs.map((p) => p.exited));
}

if (import.meta.main) {
  if (action === "stop") {
    await stopStack();
  } else if (action === "status") {
    const pids = await getRunningPIDs();
    console.log(`Running PIDs: ${pids.length > 0 ? pids.join(", ") : "none"}`);
  } else {
    await startStack();
  }
}
