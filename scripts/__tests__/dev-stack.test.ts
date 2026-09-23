import { describe, expect, it } from "bun:test";
import {
  buildBinaries,
  getAction,
  getIngesterArgs,
  getLogLevel,
  shouldSkipBuild,
  type IBuildOptions,
} from "../dev-stack";

describe("dev-stack build controls (#72)", () => {
  describe("shouldSkipBuild", () => {
    it("returns false when --no-build is not present in arguments", () => {
      expect(shouldSkipBuild(["bun", "scripts/dev-stack.ts", "start"])).toBe(false);
      expect(shouldSkipBuild(["bun", "scripts/dev-stack.ts"])).toBe(false);
      expect(shouldSkipBuild([])).toBe(false);
    });

    it("returns true when --no-build is present in arguments", () => {
      expect(shouldSkipBuild(["bun", "scripts/dev-stack.ts", "start", "--no-build"])).toBe(true);
      expect(shouldSkipBuild(["bun", "scripts/dev-stack.ts", "--no-build"])).toBe(true);
    });
  });

  describe("getLogLevel (#75)", () => {
    it("defaults to info when no log level or debug flags are present", () => {
      expect(getLogLevel(["node", "dev-stack.ts"])).toBe("info");
      expect(getLogLevel([])).toBe("info");
    });

    it("detects --debug and -d flags", () => {
      expect(getLogLevel(["node", "dev-stack.ts", "--debug"])).toBe("debug");
      expect(getLogLevel(["node", "dev-stack.ts", "-d"])).toBe("debug");
      expect(getLogLevel(["--debug"])).toBe("debug");
      expect(getLogLevel(["-d"])).toBe("debug");
    });

    it("parses --log-level with space-separated argument", () => {
      expect(getLogLevel(["node", "dev-stack.ts", "--log-level", "warn"])).toBe("warn");
      expect(getLogLevel(["--log-level", "error"])).toBe("error");
    });

    it("parses --log-level=value syntax", () => {
      expect(getLogLevel(["node", "dev-stack.ts", "--log-level=debug"])).toBe("debug");
      expect(getLogLevel(["--log-level=trace"])).toBe("trace");
    });
  });

  describe("getAction (#80)", () => {
    it("resolves action to stop when --log-level precedes action", () => {
      expect(getAction(["node", "dev-stack.ts", "--log-level", "debug", "stop"])).toBe("stop");
      expect(getAction(["--log-level", "debug", "stop"])).toBe("stop");
    });

    it("resolves action to status with boolean flags", () => {
      expect(getAction(["node", "dev-stack.ts", "-d", "status"])).toBe("status");
      expect(getAction(["node", "dev-stack.ts", "--debug", "status"])).toBe("status");
    });

    it("defaults to start when no action is specified", () => {
      expect(getAction(["node", "dev-stack.ts"])).toBe("start");
      expect(getAction([])).toBe("start");
      expect(getAction(["node", "dev-stack.ts", "--debug"])).toBe("start");
    });

    it("throws an error for unknown action", () => {
      expect(() => getAction(["node", "dev-stack.ts", "invalid"])).toThrow(/Unknown action/);
      expect(() => getAction(["unknown"])).toThrow(/Unknown action/);
    });
  });

  describe("buildBinaries", () => {
    it("skips build completely when noBuild option is true", async () => {
      let spawnCalled = false;
      const logs: string[] = [];

      const result = await buildBinaries({
        noBuild: true,
        spawnFn: () => {
          spawnCalled = true;
          throw new Error("spawn should not be called");
        },
        logFn: (msg: string) => logs.push(msg),
      });

      expect(result.isSkipped).toBe(true);
      expect(result.exitCode).toBe(0);
      expect(spawnCalled).toBe(false);
      expect(logs.some((l) => l.toLowerCase().includes("skipping binary build"))).toBe(true);
    });

    it("spawns just build-stack instead of build-all (#84)", async () => {
      let capturedCmd: string[] = [];

      await buildBinaries({
        noBuild: false,
        rootDir: "/mock/root",
        spawnFn: (cmd) => {
          capturedCmd = [...cmd];
          return {
            exited: Promise.resolve(0),
            stdout: new Response("").body,
            stderr: new Response("").body,
          };
        },
        logFn: () => {},
      });

      expect(capturedCmd).toEqual(["just", "build-stack"]);
    });

    it("buffers build output on successful build without dumping stdout to console", async () => {
      const logs: string[] = [];
      const errors: string[] = [];
      let capturedStdoutMode: string | undefined;
      let capturedStderrMode: string | undefined;

      const mockStdoutText = "Compiling air-ingester...\nCompiling air-server...\n";
      const mockStderrText = "";

      const result = await buildBinaries({
        noBuild: false,
        rootDir: "/mock/root",
        isAgent: false,
        spawnFn: (_cmd, options) => {
          capturedStdoutMode = options.stdout;
          capturedStderrMode = options.stderr;
          return {
            exited: Promise.resolve(0),
            stdout: new Response(mockStdoutText).body,
            stderr: new Response(mockStderrText).body,
          };
        },
        logFn: (msg: string) => logs.push(msg),
        errorFn: (msg: string) => errors.push(msg),
      });

      expect(result.isSkipped).toBe(false);
      expect(result.exitCode).toBe(0);
      expect(result.stdoutText).toBe(mockStdoutText);
      expect(capturedStdoutMode).toBe("pipe");
      expect(capturedStderrMode).toBe("pipe");
      // Successful build must NOT dump the raw compiler output
      expect(logs.includes(mockStdoutText)).toBe(false);
      expect(errors).toHaveLength(0);
      expect(logs.some((l) => l.includes("Building Go binaries"))).toBe(true);
    });

    it("dumps buffered stdout and stderr when build fails with non-zero exit code", async () => {
      const errors: string[] = [];
      const mockStdoutText = "Step 1: packages/agent-server\n";
      const mockStderrText = "go: syntax error in main.go:42\n";

      const result = await buildBinaries({
        noBuild: false,
        rootDir: "/mock/root",
        spawnFn: () => ({
          exited: Promise.resolve(2),
          stdout: new Response(mockStdoutText).body,
          stderr: new Response(mockStderrText).body,
        }),
        errorFn: (msg: string) => errors.push(msg),
      });

      expect(result.isSkipped).toBe(false);
      expect(result.exitCode).toBe(2);
      expect(result.stdoutText).toBe(mockStdoutText);
      expect(result.stderrText).toBe(mockStderrText);
      // Errored build MUST dump buffered stdout and stderr
      expect(errors).toContain(mockStdoutText);
      expect(errors).toContain(mockStderrText);
      expect(errors.some((e) => e.includes("exit code 2"))).toBe(true);
    });
  });

  describe("summarizer flag controls (#62, #74)", () => {
    describe("getIngesterArgs", () => {
      it("omits --no-summarize when credentials are empty and flag is not passed (#74)", () => {
        const args = getIngesterArgs({
          argv: ["bun", "scripts/dev-stack.ts", "start"],
          env: {},
          logLevel: "info",
          ingesterBin: "/bin/air-ingester",
        });
        expect(args).toEqual(["/bin/air-ingester", "--log-level", "info"]);
        expect(args.includes("--no-summarize")).toBe(false);
      });

      it("omits --no-summarize for URL-only environment without API keys (#74)", () => {
        const args = getIngesterArgs({
          argv: ["bun", "scripts/dev-stack.ts", "start"],
          env: { LITELLM_BASE_URL: "http://localhost:8000" },
          logLevel: "info",
          ingesterBin: "/bin/air-ingester",
        });
        expect(args).toEqual(["/bin/air-ingester", "--log-level", "info"]);
        expect(args.includes("--no-summarize")).toBe(false);
      });

      it("omits --no-summarize when valid credentials are provided", () => {
        const args = getIngesterArgs({
          argv: ["bun", "scripts/dev-stack.ts", "start"],
          env: { OPENAI_API_KEY: "sk-openai" },
          logLevel: "debug",
          ingesterBin: "/bin/air-ingester",
        });
        expect(args).toEqual(["/bin/air-ingester", "--log-level", "debug"]);
        expect(args.includes("--no-summarize")).toBe(false);
      });

      it("includes --no-summarize when explicitly requested in argv despite credentials", () => {
        const args = getIngesterArgs({
          argv: ["bun", "scripts/dev-stack.ts", "start", "--no-summarize"],
          env: { OPENAI_API_KEY: "sk-openai" },
          logLevel: "info",
          ingesterBin: "/bin/air-ingester",
        });
        expect(args).toEqual(["/bin/air-ingester", "--no-summarize", "--log-level", "info"]);
        expect(args.includes("--no-summarize")).toBe(true);
      });

      it("includes --no-summarize when noSummarize option is true (#74)", () => {
        const args = getIngesterArgs({
          argv: ["bun", "scripts/dev-stack.ts", "start"],
          noSummarize: true,
          logLevel: "info",
          ingesterBin: "/bin/air-ingester",
        });
        expect(args).toEqual(["/bin/air-ingester", "--no-summarize", "--log-level", "info"]);
        expect(args.includes("--no-summarize")).toBe(true);
      });

      it("derives log level from argv option regardless of process.argv (#75)", () => {
        const args = getIngesterArgs({
          argv: ["node", "dev-stack.ts", "--debug"],
          env: { OPENAI_API_KEY: "sk-openai" },
          ingesterBin: "/bin/air-ingester",
        });
        expect(args).toContain("--log-level");
        expect(args[args.indexOf("--log-level") + 1]).toBe("debug");
      });
    });
  });

  describe("documentation coverage (#87)", () => {
    it("documents --no-build and --no-summarize flags in README.md", async () => {
      const readme = await Bun.file(new URL("../../README.md", import.meta.url).pathname).text();
      expect(readme).toContain("--no-build");
      expect(readme).toContain("--no-summarize");
      expect(readme).toContain("AIR_NO_SUMMARIZE");
      expect(readme).toContain("LITELLM_KEY");
      expect(readme).toContain("Environment Variables");
    });

    it("documents updated sample startup output without stale stopping line in README.md", async () => {
      const readme = await Bun.file(new URL("../../README.md", import.meta.url).pathname).text();
      expect(readme).not.toContain("[INFO] Stopping agent-status stack...");
      expect(readme).toContain("[OK] Stack stopped.");
    });

    it("documents LITELLM_KEY and AIR_NO_SUMMARIZE in docs/prd.md and docs/implementation.md", async () => {
      const prd = await Bun.file(new URL("../../docs/prd.md", import.meta.url).pathname).text();
      const impl = await Bun.file(new URL("../../docs/implementation.md", import.meta.url).pathname).text();

      expect(prd).toContain("LITELLM_KEY");
      expect(prd).toContain("AIR_NO_SUMMARIZE");
      expect(impl).toContain("LITELLM_KEY");
      expect(impl).toContain("AIR_NO_SUMMARIZE");
    });
  });
});

