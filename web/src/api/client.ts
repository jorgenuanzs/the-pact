import { desktopBridge } from "@/platform/desktop";

export class APIError extends Error {
  status: number;
  code: string;

  constructor(message: string, status = 0, code = "") {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = code;
  }
}

type RequestOptions = Omit<RequestInit, "body"> & {
  body?: unknown;
  skipSessionExpirySignal?: boolean;
};

export const SESSION_EXPIRED_EVENT = "pact:session-expired";

export function notifySessionExpired(): void {
  window.dispatchEvent(new Event(SESSION_EXPIRED_EVENT));
}

function readCookie(name: string): string {
  const prefix = `${encodeURIComponent(name)}=`;
  return document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(prefix))
    ?.slice(prefix.length) ?? "";
}

async function responseError(response: Response): Promise<APIError> {
  let message = `La solicitud falló (${response.status}).`;
  let code = "";
  try {
    const payload = (await response.json()) as {
      error?: { message?: string; code?: string } | string;
      message?: string;
      detail?: string;
      title?: string;
      code?: string;
    };
    if (typeof payload.error === "string") message = payload.error;
    if (payload.error && typeof payload.error === "object") {
      message = payload.error.message || message;
      code = payload.error.code || "";
    }
    message = payload.message || payload.detail || payload.title || message;
    code = payload.code || code;
  } catch {
    // The fallback message is intentionally retained for non-JSON responses.
  }
  return new APIError(message, response.status, code);
}

function desktopResponseError(status: number, body: string): APIError {
  let message = `La solicitud falló (${status}).`;
  let code = "";
  try {
    const payload = JSON.parse(body) as {
      error?: { message?: string; code?: string } | string;
      message?: string;
      detail?: string;
      title?: string;
      code?: string;
    };
    if (typeof payload.error === "string") message = payload.error;
    if (payload.error && typeof payload.error === "object") {
      message = payload.error.message || message;
      code = payload.error.code || "";
    }
    message = payload.message || payload.detail || payload.title || message;
    code = payload.code || code;
  } catch {
    if (body.trim()) message = body.trim();
  }
  return new APIError(message, status, code);
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { body, skipSessionExpirySignal = false, ...requestOptions } = options;
  const method = String(requestOptions.method || "GET").toUpperCase();
  const headers = new Headers(requestOptions.headers);
  headers.set("Accept", "application/json");
  if (body !== undefined) headers.set("Content-Type", "application/json");
  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    const csrf = decodeURIComponent(readCookie("pact_csrf"));
    if (csrf) headers.set("X-Pact-CSRF", csrf);
  }

  const native = desktopBridge();
  if (native) {
    let response;
    try {
      response = await native.APIRequest({
        method,
        path,
        headers: Object.fromEntries(headers.entries()),
        body: body === undefined ? "" : JSON.stringify(body),
      });
    } catch (error) {
      throw new APIError(error instanceof Error && error.message ? error.message : "No se pudo conectar con PACT Server.");
    }
    if (response.status === 401 && !skipSessionExpirySignal) notifySessionExpired();
    if (response.status < 200 || response.status >= 300) {
      throw desktopResponseError(response.status, response.body);
    }
    if (response.status === 204 || !response.body.trim()) return undefined as T;
    try {
      return JSON.parse(response.body) as T;
    } catch {
      throw new APIError("PACT Server devolvió una respuesta que la aplicación no pudo interpretar.", response.status);
    }
  }

  let response: Response;
  try {
    response = await fetch(path, {
      ...requestOptions,
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      credentials: "same-origin",
      cache: "no-store",
    });
  } catch {
    throw new APIError("No se pudo conectar con PACT Server.");
  }

  if (response.status === 401 && !skipSessionExpirySignal) notifySessionExpired();
  if (!response.ok) throw await responseError(response);
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

export function unwrap<T>(payload: T | { data: T }): T {
  if (payload && typeof payload === "object" && "data" in payload) {
    return (payload as { data: T }).data;
  }
  return payload as T;
}

export async function requestData<T>(path: string, options: RequestOptions = {}): Promise<T> {
  return unwrap(await request<T | { data: T }>(path, options));
}

export function idempotencyKey(scope: string): string {
  const suffix = typeof crypto.randomUUID === "function"
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `${scope}-${suffix}`;
}
