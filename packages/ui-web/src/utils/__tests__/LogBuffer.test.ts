import { describe, it, expect, mock } from "bun:test";
import { LogBuffer } from "../LogBuffer";
import { fixture_telemetryFrame, fixture_telemetryFrameSubagent } from "./fixtures";

describe("LogBuffer Store", () => {
  it("initializes with empty frames and count 0", () => {
    const buffer = new LogBuffer();
    expect(buffer.getSnapshot()).toEqual([]);
    expect(buffer.count).toBe(0);
  });

  it("returns stable snapshot reference when unmutated", () => {
    const buffer = new LogBuffer();
    const snapshot1 = buffer.getSnapshot();
    const snapshot2 = buffer.getSnapshot();
    expect(snapshot1).toBe(snapshot2);
  });

  it("sets frames and notifies subscribers", () => {
    const buffer = new LogBuffer();
    const subscriber = mock(() => {});
    const unsubscribe = buffer.subscribe(subscriber);

    buffer.set([fixture_telemetryFrame]);

    expect(subscriber).toHaveBeenCalledTimes(1);
    expect(buffer.count).toBe(1);
    expect(buffer.getSnapshot()).toEqual([fixture_telemetryFrame]);

    unsubscribe();
  });

  it("prepends a frame and notifies subscribers", () => {
    const buffer = new LogBuffer();
    const subscriber = mock(() => {});
    buffer.subscribe(subscriber);

    buffer.set([fixture_telemetryFrame]);
    buffer.prepend(fixture_telemetryFrameSubagent);

    expect(subscriber).toHaveBeenCalledTimes(2);
    expect(buffer.count).toBe(2);
    expect(buffer.getSnapshot()[0]).toEqual(fixture_telemetryFrameSubagent);
    expect(buffer.getSnapshot()[1]).toEqual(fixture_telemetryFrame);
  });

  it("clears frames and notifies subscribers only when non-empty", () => {
    const buffer = new LogBuffer();
    const subscriber = mock(() => {});
    buffer.subscribe(subscriber);

    // Initial clear when empty -> no notification
    buffer.clear();
    expect(subscriber).toHaveBeenCalledTimes(0);

    // Populate and clear
    buffer.set([fixture_telemetryFrame]);
    expect(subscriber).toHaveBeenCalledTimes(1);

    buffer.clear();
    expect(subscriber).toHaveBeenCalledTimes(2);
    expect(buffer.count).toBe(0);
    expect(buffer.getSnapshot()).toEqual([]);
  });

  it("unsubscribes listeners cleanly", () => {
    const buffer = new LogBuffer();
    const subscriber = mock(() => {});
    const unsubscribe = buffer.subscribe(subscriber);

    unsubscribe();
    buffer.set([fixture_telemetryFrame]);
    expect(subscriber).toHaveBeenCalledTimes(0);
  });

  it("notifies multiple subscribers independently", () => {
    const buffer = new LogBuffer();
    const sub1 = mock(() => {});
    const sub2 = mock(() => {});

    const unsub1 = buffer.subscribe(sub1);
    buffer.subscribe(sub2);

    buffer.prepend(fixture_telemetryFrame);
    expect(sub1).toHaveBeenCalledTimes(1);
    expect(sub2).toHaveBeenCalledTimes(1);

    unsub1();
    buffer.prepend(fixture_telemetryFrameSubagent);
    expect(sub1).toHaveBeenCalledTimes(1);
    expect(sub2).toHaveBeenCalledTimes(2);
  });
});
