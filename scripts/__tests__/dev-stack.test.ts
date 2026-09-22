import { describe, expect, it } from "bun:test";
import {
  buildBinaries,
  getIngesterArgs,
  shouldDisableSummarizer,
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

  describe("summarizer flag controls (#62)", () => {
    describe("shouldDisableSummarizer", () => {
      it("returns true when --no-summarize flag is explicitly passed in argv", () => {
        expect(
          shouldDisableSummarizer(["bun", "scripts/dev-stack.ts", "start", "--no-summarize"], {
            OPENAI_API_KEY: "sk-test",
          })
        ).toBe(true);

        expect(
          shouldDisableSummarizer(["bun", "scripts/dev-stack.ts", "--no-summarize"], {
            LITELLM_BASE_URL: "https://litellm.example.com",
            LITELLM_API_KEY: "secret",
          })
        ).toBe(true);
      });

      it("returns true when no LLM provider credentials are in env", () => {
        expect(shouldDisableSummarizer(["bun", "scripts/dev-stack.ts", "start"], {})).toBe(true);
        expect(shouldDisableSummarizer([], {})).toBe(true);
      });

      it("returns false when OPENAI_API_KEY is present in env", () => {
        expect(
          shouldDisableSummarizer(["bun", "scripts/dev-stack.ts", "start"], {
            OPENAI_API_KEY: "sk-openai-test",
          })
        ).toBe(false);
      });

      it("returns false when LITELLM_BASE_URL and LITELLM_API_KEY are present", () => {
        expect(
          shouldDisableSummarizer(["bun", "scripts/dev-stack.ts", "start"], {
            LITELLM_BASE_URL: "https://litellm.example.com",
            LITELLM_API_KEY: "sk-litellm-1",
          })
        ).toBe(false);
      });

      it("returns false when LITELLM_BASE_URL and LITELLM_KEY alias are present", () => {
        expect(
          shouldDisableSummarizer(["bun", "scripts/dev-stack.ts", "start"], {
            LITELLM_BASE_URL: "https://litellm.example.com",
            LITELLM_KEY: "sk-litellm-alias",
          })
        ).toBe(false);
      });

      it("returns false when GEMINI_API_KEY is present", () => {
        expect(
          shouldDisableSummarizer(["bun", "scripts/dev-stack.ts", "start"], {
            GEMINI_API_KEY: "gemini-test-key",
          })
        ).toBe(false);
      });

      it("returns false when ANTHROPIC_API_KEY is present", () => {
        expect(
          shouldDisableSummarizer(["bun", "scripts/dev-stack.ts", "start"], {
            ANTHROPIC_API_KEY: "anthropic-test-key",
          })
        ).toBe(false);
      });
    });

    describe("getIngesterArgs", () => {
      it("includes --no-summarize when credentials are empty", () => {
        const args = getIngesterArgs({
          argv: ["bun", "scripts/dev-stack.ts", "start"],
          env: {},
          logLevel: "info",
          ingesterBin: "/bin/air-ingester",
        });
        expect(args).toEqual(["/bin/air-ingester", "--no-summarize", "--log-level", "info"]);
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

      it("includes --no-summarize when explicitly requested despite credentials", () => {
        const args = getIngesterArgs({
          argv: ["bun", "scripts/dev-stack.ts", "start", "--no-summarize"],
          env: { OPENAI_API_KEY: "sk-openai" },
          logLevel: "info",
          ingesterBin: "/bin/air-ingester",
        });
        expect(args).toEqual(["/bin/air-ingester", "--no-summarize", "--log-level", "info"]);
      });
    });
  });
});
