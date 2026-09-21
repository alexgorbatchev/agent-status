import type { ITelemetryFrame } from "../types";
import { correlateTelemetryFrames } from "./correlateTelemetryFrames";

export type LogBufferListener = () => void;
export type LogBufferUnsubscribe = () => void;

export class LogBuffer {
  private frames: readonly ITelemetryFrame[] = [];
  private readonly listeners: Set<LogBufferListener> = new Set<LogBufferListener>();

  constructor(initialFrames?: readonly ITelemetryFrame[]) {
    if (initialFrames && initialFrames.length > 0) {
      this.frames = [...initialFrames];
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
    this.frames = [...frames];
    this.notify();
  }

  public prepend(frame: ITelemetryFrame): void {
    this.frames = [frame, ...this.frames];
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
