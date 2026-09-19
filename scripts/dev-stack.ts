import { spawn, type Subprocess } from "bun";
import { existsSync, mkdirSync, unlinkSync } from "node:fs";
import { resolve } from "node:path";

const rootDir = resolve(import.meta.dir, "..");
const runDir = resolve(rootDir, ".tmp", "dev-stack");
if (!existsSync(runDir)) {
  mkdirSync(runDir, { recursive: true });
}

const pidFile = resolve(runDir, "pids.json");
const action = process.argv[2] || "start";

function getRunningPIDs(): number[] {
  if (!existsSync(pidFile)) return [];
  try {
    const data = JSON.parse(Bun.file(pidFile).text() as unknown as string);
    return Array.isArray(data) ? data : [];
  } catch {
    return [];
  }
}

async function stopStack() {
  console.log("🛑 Stopping agent-status stack...");
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
      console.log(`   Killed PID ${pid}`);
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
          console.log(`   Cleaned up port ${port} (PID ${p})`);
        }
      }
    } catch {
      // ignore
    }
  }

  if (existsSync(pidFile)) {
    unlinkSync(pidFile);
  }
  console.log("✅ Stack stopped.");
}

async function startStack() {
  // Ensure previous stack is stopped
  await stopStack();

  console.log("🚀 Starting agent-status stack (ingester, server, observer, ui)...");

  // 1. Build Go binaries if not already built
  const ingesterBin = resolve(rootDir, "packages", "agent-ingester", "bin", "air-ingester");
  const serverBin = resolve(rootDir, "packages", "agent-server", "bin", "air-server");
  const observerBin = resolve(rootDir, "packages", "agent-observer", "bin", "air-observer");

  if (!existsSync(ingesterBin) || !existsSync(serverBin) || !existsSync(observerBin)) {
    console.log("📦 Building Go binaries...");
    const buildProc = Bun.spawn(["just", "build-all"], { cwd: rootDir, stdout: "inherit", stderr: "inherit" });
    await buildProc.exited;
  }

  const pids: number[] = [];
  const procs: Subprocess[] = [];

  const dbDir = resolve(runDir, "data");
  mkdirSync(dbDir, { recursive: true });
  const notifySock = "/tmp/agent-status/notify.sock";

  // 1. Ingester
  console.log("   [1/4] Starting air-ingester (:4501)...");
  const ingester = Bun.spawn([ingesterBin, "--no-summarize"], {
    cwd: rootDir,
    env: {
      ...process.env,
      AIR_INGESTER_ADDR: ":4501",
      AIR_BEARER_TOKEN: "dev-secret-token",
      AIR_NOTIFY_SOCK: notifySock,
      XDG_DATA_HOME: dbDir,
      XDG_RUNTIME_DIR: "/tmp",
    },
    stdout: "inherit",
    stderr: "inherit",
  });
  pids.push(ingester.pid);
  procs.push(ingester);

  // Wait 500ms for ingester to bind socket
  await Bun.sleep(500);

  // 2. Server
  console.log("   [2/4] Starting air-server (:4502)...");
  const server = Bun.spawn([serverBin, "--cors-origin", "http://localhost:3000"], {
    cwd: rootDir,
    env: {
      ...process.env,
      AIR_SERVER_ADDR: ":4502",
      AGENT_STATUS_TOKEN: "dev-secret-token",
      AIR_NOTIFY_SOCK: notifySock,
      XDG_DATA_HOME: dbDir,
      XDG_RUNTIME_DIR: "/tmp",
    },
    stdout: "inherit",
    stderr: "inherit",
  });
  pids.push(server.pid);
  procs.push(server);

  await Bun.sleep(500);

  // 3. Observer
  console.log("   [3/4] Starting air-observer...");
  const observer = Bun.spawn([observerBin, "--token", "dev-secret-token", "--ingester-url", "ws://127.0.0.1:4501/ws/ingest"], {
    cwd: rootDir,
    env: {
      ...process.env,
      AGENT_STATUS_TOKEN: "dev-secret-token",
      XDG_CACHE_HOME: resolve(runDir, "cache"),
    },
    stdout: "inherit",
    stderr: "inherit",
  });
  pids.push(observer.pid);
  procs.push(observer);

  // 4. UI Web
  console.log("   [4/4] Starting ui-web (http://localhost:3000)...");
  const uiWebDir = resolve(rootDir, "packages", "ui-web");
  const ui = Bun.spawn(["bun", "run", "src/server.ts"], {
    cwd: uiWebDir,
    env: {
      ...process.env,
      PORT: "3000",
    },
    stdout: "inherit",
    stderr: "inherit",
  });
  pids.push(ui.pid);
  procs.push(ui);

  await Bun.write(pidFile, JSON.stringify(pids, null, 2));

  console.log("\n✨ agent-status is running at http://localhost:3000");
  console.log("   Press Ctrl+C to stop or run: just stop\n");

  const shutdown = async () => {
    await stopStack();
    process.exit(0);
  };

  process.on("SIGINT", shutdown);
  process.on("SIGTERM", shutdown);

  // Keep foreground process alive
  await Promise.race(procs.map((p) => p.exited));
}

if (action === "stop") {
  await stopStack();
} else if (action === "status") {
  const pids = getRunningPIDs();
  console.log(`Running PIDs: ${pids.length > 0 ? pids.join(", ") : "none"}`);
} else {
  await startStack();
}
