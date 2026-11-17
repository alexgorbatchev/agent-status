import { afterEach, describe, expect, it } from "bun:test";
import { rm } from "node:fs/promises";
import { createStackSandbox, runStackRecipe } from "./helpers";

const roots: string[] = [];
afterEach(async () => {
  await Promise.all(roots.splice(0).map((path) => rm(path, { recursive: true, force: true })));
});

describe("devhost lifecycle recipes", () => {
  it("builds each stack component before starting the selected services", async () => {
    const root = await createStackSandbox();
    roots.push(root);
    const result = await runStackRecipe(root, ["start", "ui"]);
    expect(result.exitCode).toBe(0);
    expect(result.output).toEqual([
      { command: "build", component: "agent-observer", args: ["build"] },
      { command: "build", component: "agent-ingester", args: ["build"] },
      { command: "build", component: "agent-server", args: ["build"] },
      { command: "build", component: "ui-web", args: ["build"] },
      "direnv",
      { command: "devhost", args: ["start", "--manifest", "devhost.toml", "ui"] },
    ]);
  });

  it("uses native dependency skipping and forwards arguments and environment intact", async () => {
    const root = await createStackSandbox();
    roots.push(root);
    const result = await runStackRecipe(
      root,
      ["--no-deps", "start", "ui", "--idle-timeout", "1m", "--debug", "literal $(false) value"],
      { AGENT_DEV_LOG_LEVEL: "warn", AGENT_NO_SUMMARIZE: "1", AGENT: "1" },
    );
    expect(result.exitCode).toBe(0);
    expect(result.output).toEqual([
      "direnv",
      {
        command: "devhost",
        args: [
          "start",
          "--manifest",
          "devhost.toml",
          "ui",
          "--idle-timeout",
          "1m",
          "--debug",
          "literal $(false) value",
        ],
        level: "warn",
        summarize: "1",
        agent: "1",
      },
    ]);
  });

  it("does not launch devhost after a build fails", async () => {
    const root = await createStackSandbox();
    roots.push(root);
    const result = await runStackRecipe(root, ["start"], { TEST_BUILD_EXIT: "7" });
    expect(result.exitCode).toBe(7);
    expect(result.output).toEqual([
      { command: "build", component: "agent-observer", args: ["build"] },
    ]);
  });

  it("stops only the repository manifest without building", async () => {
    const root = await createStackSandbox();
    roots.push(root);
    const result = await runStackRecipe(root, ["stop"]);
    expect(result.exitCode).toBe(0);
    expect(result.output).toEqual([
      "direnv",
      { command: "devhost", args: ["stop", "--manifest", "devhost.toml"] },
    ]);
  });

  it("lists machine-wide stacks without building or loading an app environment", async () => {
    const root = await createStackSandbox();
    roots.push(root);
    const result = await runStackRecipe(root, ["status"]);
    expect(result.exitCode).toBe(0);
    expect(result.output).toEqual([{ command: "devhost", args: ["stack", "list"] }]);
  });

  it("preserves a failed devhost command's exit status", async () => {
    const root = await createStackSandbox();
    roots.push(root);
    const result = await runStackRecipe(root, ["--no-deps", "start"], { TEST_DEVHOST_EXIT: "6" });
    expect(result.exitCode).toBe(6);
    expect(result.output).toEqual([
      "direnv",
      { command: "devhost", args: ["start", "--manifest", "devhost.toml"] },
    ]);
  });
});
