import React, { useEffect } from "react";
import type { IStateHelpDialogProps } from "./types";
import { StatusBadge } from "./StatusBadge";
import { IconWrapper } from "./IconWrapper";
import { HelpCircle, X, CheckCircle2, Activity, Play } from "lucide-react";

export function StateHelpDialog({ onClose }: IStateHelpDialogProps): React.JSX.Element {
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [onClose]);

  return (
    <div
      data-testid="StateHelpDialog"
      role="dialog"
      aria-modal="true"
      aria-labelledby="StateHelpDialog--title"
      className="fixed inset-0 bg-slate-950/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 overflow-y-auto"
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          onClose();
        }
      }}
    >
      <div className="relative w-full max-w-2xl bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl overflow-hidden font-sans my-8">
        {/* Header */}
        <div
          data-testid="StateHelpDialog--header"
          className="flex items-center justify-between p-5 border-b border-slate-800 bg-slate-900/80"
        >
          <div className="flex items-center gap-2.5">
            <IconWrapper size="lg" tone="primary">
              <HelpCircle size={20} />
            </IconWrapper>
            <div>
              <h2
                id="StateHelpDialog--title"
                data-testid="StateHelpDialog--title"
                className="text-base font-bold text-slate-100"
              >
                Agent States & Lifecycle Guide
              </h2>
              <p className="text-xs text-slate-400">
                Understanding session statuses, active counts, and process lifecycles
              </p>
            </div>
          </div>

          <button
            type="button"
            aria-label="Close state legend help dialog"
            data-testid="StateHelpDialog--close-button"
            onClick={onClose}
            className="p-1.5 rounded-lg hover:bg-slate-800 text-slate-400 hover:text-slate-200 transition-colors cursor-pointer"
          >
            <IconWrapper size="lg">
              <X size={20} />
            </IconWrapper>
          </button>
        </div>

        {/* Body Content */}
        <div
          data-testid="StateHelpDialog--content"
          className="p-5 space-y-6 max-h-[70vh] overflow-y-auto text-xs text-slate-300"
        >
          {/* Overview Concept */}
          <div className="p-3.5 rounded-xl bg-slate-950/60 border border-slate-800 space-y-2">
            <h3 className="font-semibold text-slate-200 text-sm flex items-center gap-2">
              <IconWrapper size="md" tone="primary">
                <Activity size={16} />
              </IconWrapper>
              Active vs. Idle vs. Finished
            </h3>
            <p className="text-slate-400 leading-relaxed">
              Agent status combines two dimensions: <strong className="text-slate-200">Lifecycle</strong> (whether the
              host process is running or terminated) and <strong className="text-slate-200">Activity</strong> (what the
              agent is currently doing).
            </p>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 pt-1 font-mono text-[11px]">
              <div className="p-2 rounded bg-slate-900 border border-slate-800">
                <span className="font-bold text-blue-400 block mb-0.5">Active / Working</span>
                <span className="text-slate-400 text-[10px]">
                  Process alive + actively executing tools or reasoning
                </span>
              </div>
              <div className="p-2 rounded bg-slate-900 border border-slate-800">
                <span className="font-bold text-slate-300 block mb-0.5">Idle</span>
                <span className="text-slate-400 text-[10px]">Process alive + waiting for user prompt input</span>
              </div>
              <div className="p-2 rounded bg-slate-900 border border-slate-800">
                <span className="font-bold text-emerald-400 block mb-0.5">Finished</span>
                <span className="text-slate-400 text-[10px]">Process terminated cleanly (dimmed card)</span>
              </div>
            </div>
          </div>

          {/* Running Statuses */}
          <div>
            <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400 mb-3 flex items-center gap-1.5">
              <IconWrapper size="sm" tone="success">
                <Play size={14} />
              </IconWrapper>
              Running Agent States (Process Alive)
            </h3>
            <div className="space-y-2.5">
              <div className="p-3 rounded-lg bg-slate-950/40 border border-slate-800/80 flex items-start gap-3">
                <StatusBadge lifecycle="RUNNING" activity="BUSY" />
                <div className="flex-1">
                  <p className="text-slate-300 font-medium">Actively working</p>
                  <p className="text-slate-400 text-[11px] mt-0.5">
                    The agent is currently thinking, generating code, or executing tools. Counted in{" "}
                    <strong className="text-blue-300">Active</strong> fleet metrics and section headers.
                  </p>
                </div>
              </div>

              <div className="p-3 rounded-lg bg-slate-950/40 border border-slate-800/80 flex items-start gap-3">
                <StatusBadge lifecycle="RUNNING" activity="IDLE" />
                <div className="flex-1">
                  <p className="text-slate-300 font-medium">Idle & waiting for input</p>
                  <p className="text-slate-400 text-[11px] mt-0.5">
                    The agent session completed its turn and is sitting idle in terminal waiting for the user's next
                    prompt.
                  </p>
                </div>
              </div>

              <div className="p-3 rounded-lg bg-slate-950/40 border border-slate-800/80 flex items-start gap-3">
                <StatusBadge lifecycle="RUNNING" activity="WAITING_QUESTION" />
                <div className="flex-1">
                  <p className="text-slate-300 font-medium">Waiting on question response</p>
                  <p className="text-slate-400 text-[11px] mt-0.5">
                    The agent invoked an interactive question prompt and is blocked waiting for user response. Displayed
                    in the <strong className="text-amber-300">Act Now</strong> banner.
                  </p>
                </div>
              </div>

              <div className="p-3 rounded-lg bg-slate-950/40 border border-slate-800/80 flex items-start gap-3">
                <StatusBadge lifecycle="RUNNING" activity="ERROR" />
                <div className="flex-1">
                  <p className="text-slate-300 font-medium">Execution error</p>
                  <p className="text-slate-400 text-[11px] mt-0.5">
                    A tool execution failed with an error or unhandled exception. Displayed in the{" "}
                    <strong className="text-red-300">Act Now</strong> banner.
                  </p>
                </div>
              </div>
            </div>
          </div>

          {/* Exited Statuses */}
          <div>
            <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400 mb-3 flex items-center gap-1.5">
              <IconWrapper size="sm" tone="subtle">
                <CheckCircle2 size={14} />
              </IconWrapper>
              Exited Agent States (Process Terminated)
            </h3>
            <div className="space-y-2.5">
              <div className="p-3 rounded-lg bg-slate-950/40 border border-slate-800/80 flex items-start gap-3">
                <StatusBadge lifecycle="EXITED" activity="IDLE" />
                <div className="flex-1">
                  <p className="text-slate-300 font-medium">Finished</p>
                  <p className="text-slate-400 text-[11px] mt-0.5">
                    The agent process exited cleanly. Displayed with reduced opacity (0.6) on the dashboard and 1.0 on
                    hover.
                  </p>
                </div>
              </div>

              <div className="p-3 rounded-lg bg-slate-950/40 border border-slate-800/80 flex items-start gap-3">
                <StatusBadge lifecycle="EXITED" activity="ERROR" />
                <div className="flex-1">
                  <p className="text-slate-300 font-medium">Died on error</p>
                  <p className="text-slate-400 text-[11px] mt-0.5">
                    The agent process terminated after encountering an error.
                  </p>
                </div>
              </div>

              <div className="p-3 rounded-lg bg-slate-950/40 border border-slate-800/80 flex items-start gap-3">
                <StatusBadge lifecycle="EXITED" activity="WAITING_QUESTION" />
                <div className="flex-1">
                  <p className="text-slate-300 font-medium">Abandoned prompt</p>
                  <p className="text-slate-400 text-[11px] mt-0.5">
                    The session was stopped while waiting for user question input.
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div
          data-testid="StateHelpDialog--footer"
          className="flex items-center justify-end p-4 border-t border-slate-800 bg-slate-900/90"
        >
          <button
            type="button"
            data-testid="StateHelpDialog--got-it-button"
            onClick={onClose}
            className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium text-xs transition-colors cursor-pointer"
          >
            Got it
          </button>
        </div>
      </div>
    </div>
  );
}
