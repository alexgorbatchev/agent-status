import { describe, it, expect } from "bun:test";
import { useEventStream } from "../useEventStream";

describe("useEventStream Hook", () => {
  it("exports useEventStream hook function", () => {
    expect(typeof useEventStream).toBe("function");
  });
});
