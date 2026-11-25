import type { ITelemetryFrame } from "../types";
import { correlateTelemetryFrames } from "./correlateTelemetryFrames";

export type LogBufferListener = () => void;
export type LogBufferUnsubscribe = () => void;
export type LogBufferInit = number | readonly ITelemetryFrame[];

export const DEFAULT_LOG_BUFFER_CAPACITY = 500;

export class LogBuffer {
  private frames: readonly ITelemetryFrame[] = [];
  private readonly listeners: Set<LogBufferListener> = new Set<LogBufferListener>();
  private readonly maxCapacity: number;

  public constructor(maxCapacity?: number);
  public constructor(initialFrames?: readonly ITelemetryFrame[], maxCapacity?: number);
  public constructor(arg?: LogBufferInit, maxCapacity: number = DEFAULT_LOG_BUFFER_CAPACITY) {
    if (typeof arg === "number") {
      this.maxCapacity = arg;
    } else {
      this.maxCapacity = maxCapacity;
      if (arg && arg.length > 0) {
        this.frames = arg.length > this.maxCapacity ? arg.slice(0, this.maxCapacity) : [...arg];
      }
    }
  }

  public subscribe = (listener: LogBufferListener): LogBufferUnsubscribe => {
    this.listeners.add(listener);
    return (): void => {
      this.listeners.delete(listener);
    };
  };

  public getSnapshot = (): readonly ITelemetryFrame[] => {
    return this.frames;
  };

  public getCorrelatedFrames = (): ITelemetryFrame[] => {
    return correlateTelemetryFrames(this.frames);
  };

  public set(frames: readonly ITelemetryFrame[]): void {
    this.frames = frames.length > this.maxCapacity ? frames.slice(0, this.maxCapacity) : [...frames];
    this.notify();
  }

  public prepend(frame: ITelemetryFrame): void {
    const next = [frame, ...this.frames];
    this.frames = next.length > this.maxCapacity ? next.slice(0, this.maxCapacity) : next;
    this.notify();
  }

  public clear(): void {
    if (this.frames.length > 0) {
      this.frames = [];
      this.notify();
    }
  }

  public get count(): number {
    return this.frames.length;
  }

  private notify(): void {
    for (const listener of this.listeners) {
      listener();
    }
  }
}
