import { DEFAULT_DEV_TOKEN, resolveApiBaseUrl } from "../utils/config";

export interface IFocusAgentResponse {
  ok: boolean;
  paneId?: string;
  workspaceId?: string;
  tabId?: string;
  error?: string;
}

export async function focusAgent(
  id: string,
  apiBaseUrl?: string,
  token: string = DEFAULT_DEV_TOKEN,
): Promise<IFocusAgentResponse> {
  const baseUrl = resolveApiBaseUrl(apiBaseUrl);
  try {
    const response = await fetch(`${baseUrl}/api/v1/agents/${encodeURIComponent(id)}/focus`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
      },
    });

    const data = (await response.json().catch(() => ({}))) as Record<string, unknown>;

    if (!response.ok) {
      const errorMessage =
        typeof data.error === "string" ? data.error : `HTTP ${response.status}: ${response.statusText}`;
      return {
        ok: false,
        error: errorMessage,
      };
    }

    return {
      ok: Boolean(data.ok),
      paneId: typeof data.paneId === "string" ? data.paneId : undefined,
      workspaceId: typeof data.workspaceId === "string" ? data.workspaceId : undefined,
      tabId: typeof data.tabId === "string" ? data.tabId : undefined,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : "Network error";
    return {
      ok: false,
      error: message,
    };
  }
}

export interface IDismissCompletionResponse {
  ok: boolean;
  error?: string;
}

export async function dismissCompletion(
  agentId: string,
  apiBaseUrl?: string,
  token: string = DEFAULT_DEV_TOKEN,
): Promise<IDismissCompletionResponse> {
  const baseUrl = resolveApiBaseUrl(apiBaseUrl);
  try {
    const response = await fetch(`${baseUrl}/api/v1/agents/${encodeURIComponent(agentId)}`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ dismissCompletion: true }),
    });

    const data = (await response.json().catch(() => ({}))) as Record<string, unknown>;

    if (!response.ok) {
      const errorMessage =
        typeof data.error === "string" ? data.error : `HTTP ${response.status}: ${response.statusText}`;
      return {
        ok: false,
        error: errorMessage,
      };
    }

    return {
      ok: true,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : "Network error";
    return {
      ok: false,
      error: message,
    };
  }
}

export interface ISetAgentVisibilityResponse {
  ok: boolean;
  error?: string;
}

export async function setAgentVisibility(
  agentId: string,
  isHidden: boolean,
  apiBaseUrl?: string,
  token: string = DEFAULT_DEV_TOKEN,
): Promise<ISetAgentVisibilityResponse> {
  const baseUrl = resolveApiBaseUrl(apiBaseUrl);
  try {
    const response = await fetch(`${baseUrl}/api/v1/agents/${encodeURIComponent(agentId)}`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ isHidden }),
    });

    const data = (await response.json().catch(() => ({}))) as Record<string, unknown>;

    if (!response.ok) {
      const errorMessage =
        typeof data.error === "string" ? data.error : `HTTP ${response.status}: ${response.statusText}`;
      return {
        ok: false,
        error: errorMessage,
      };
    }

    return {
      ok: true,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : "Network error";
    return {
      ok: false,
      error: message,
    };
  }
}
