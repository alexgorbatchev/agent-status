import { mkdtemp, mkdir, chmod } from "node:fs/promises";
import { resolve } from "node:path";

const justExecutable = Bun.spawnSync(["sh", "-c", "command -v just"]).stdout.toString().trim();

interface IRecipeResult {
  exitCode: number;
  output: unknown[];
}

export async function createStackSandbox(): Promise<string> {
  const root = await createServiceSandbox();
  await mkdir(resolve(root, "packages/ui-web"), { recursive: true });
  await mkdir(resolve(root, ".bin"));
  await Bun.write(resolve(root, "justfile"), Bun.file(resolve(import.meta.dir, "../../justfile")));
  const executables: Record<string, string> = {
    just: "#!/usr/bin/env bun\nconsole.log(JSON.stringify({command:'build',component:process.cwd().split('/').pop(),args:process.argv.slice(2)}));process.exit(Number(process.env.TEST_BUILD_EXIT??0));\n",
    direnv: "#!/bin/sh\nprintf '%s\\n' '\"direnv\"'\nshift 2\nexec \"$@\"\n",
    devhost:
      "#!/usr/bin/env bun\nconsole.log(JSON.stringify({command:'devhost',args:process.argv.slice(2),level:process.env.AGENT_DEV_LOG_LEVEL,summarize:process.env.AGENT_NO_SUMMARIZE,agent:process.env.AGENT}));process.exit(Number(process.env.TEST_DEVHOST_EXIT??0));\n",
  };
  for (const [name, source] of Object.entries(executables)) {
    const path = resolve(root, ".bin", name);
    await Bun.write(path, source);
    await chmod(path, 0o755);
  }
  return root;
}

export async function runStackRecipe(
  root: string,
  args: string[],
  environment: NodeJS.ProcessEnv = {},
): Promise<IRecipeResult> {
  const child = Bun.spawn([justExecutable, "--justfile", resolve(root, "justfile"), ...args], {
    cwd: root,
    env: { PATH: resolve(root, ".bin") + ":" + process.env.PATH, ...environment },
    stdout: "pipe",
    stderr: "pipe",
  });
  const [exitCode, stdout] = await Promise.all([
    child.exited,
    new Response(child.stdout).text(),
    new Response(child.stderr).text(),
  ]);
  return {
    exitCode,
    output: stdout
      .trim()
      .split("\n")
      .filter(Boolean)
      .map((line) => JSON.parse(line)),
  };
}

export async function createServiceSandbox(): Promise<string> {
  const rootDir = await mkdtemp(resolve(import.meta.dir, "../../.tmp/t-"));
  await mkdir(resolve(rootDir, "packages/agent-observer/bin"), { recursive: true });
  await mkdir(resolve(rootDir, "packages/agent-ingester/bin"), { recursive: true });
  await mkdir(resolve(rootDir, "packages/agent-server/bin"), { recursive: true });
  const binary =
    "#!/bin/sh\nexec bun -e 'process.stdout.write(JSON.stringify({args:process.argv.slice(1),data:process.env.XDG_DATA_HOME,cache:process.env.XDG_CACHE_HOME,runtime:process.env.XDG_RUNTIME_DIR,logs:process.env.AGENT_LOG_DIR,logFile:process.env.AGENT_LOG_FILE??null,sessionState:process.env.AGENT_SESSION_STATE_PATH??null,ws:process.env.AGENT_INGESTER_WS_URL,http:process.env.AGENT_INGESTER_HTTP_URL}))' -- \"$@\"\n";
  const binaries = ["agent-observer", "agent-ingester", "agent-server"];
  for (const name of binaries) {
    const path = resolve(rootDir, "packages", name, "bin", name);
    await Bun.write(path, binary);
    await chmod(path, 0o755);
  }
  return rootDir;
}
