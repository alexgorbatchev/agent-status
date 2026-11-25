import { useState, useEffect } from "react";
import { formatRelativeTime } from "../utils/agentUtils";

export function useRelativeTime(timestamp: number, intervalMs: number = 1000): string {
  const [now, setNow] = useState<number>(() => Date.now());

  useEffect(() => {
    if (!timestamp || timestamp <= 0 || Number.isNaN(timestamp)) {
      return;
    }

    const timer = setInterval(() => {
      setNow(Date.now());
    }, intervalMs);

    return () => {
      clearInterval(timer);
    };
  }, [timestamp, intervalMs]);

  return formatRelativeTime(timestamp, now);
}
