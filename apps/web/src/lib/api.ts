// Cliente HTTP da API. Cookie de sessão é HttpOnly (gerenciado pelo navegador);
// o token CSRF fica só em memória e é enviado em toda requisição que altera estado.

export type FieldErrors = Record<string, string>;

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public fields: FieldErrors = {},
  ) {
    super(message);
  }
}

let csrfToken = "";
let onUnauthorized: (() => void) | null = null;

export function setCsrfToken(token: string) {
  csrfToken = token;
}

/** Registrado pelo app para reagir a sessão expirada (401). */
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

async function demoCall<T>(method: Method, path: string, body?: unknown): Promise<T> {
  const { demoApi, DemoError } = await import("./demo-api");
  try {
    return (await demoApi(method, path, body as Record<string, unknown>)) as T;
  } catch (e) {
    if (e instanceof DemoError) {
      if (e.status === 401 && !path.startsWith("/auth/login")) onUnauthorized?.();
      throw new ApiError(e.status, e.code, e.message, e.fields);
    }
    throw e;
  }
}

export async function api<T>(method: Method, path: string, body?: unknown): Promise<T> {
  // Condição inline para o bundler remover a demo do build normal.
  if (import.meta.env.VITE_DEMO === "1") return demoCall<T>(method, path, body);
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (method !== "GET" && csrfToken) headers["X-CSRF-Token"] = csrfToken;

  const res = await fetch(`/api/v1${path}`, {
    method,
    headers,
    credentials: "include",
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  if (res.status === 204) return undefined as T;

  let data: unknown = null;
  try {
    data = await res.json();
  } catch {
    /* corpo vazio ou não-JSON */
  }

  if (!res.ok) {
    const err = (data as { error?: { code?: string; message?: string; fields?: FieldErrors } } | null)?.error;
    if (res.status === 401 && !path.startsWith("/auth/login")) onUnauthorized?.();
    throw new ApiError(res.status, err?.code ?? "http_error", err?.message ?? "Erro inesperado.", err?.fields);
  }

  const token = (data as { csrfToken?: unknown } | null)?.csrfToken;
  if (typeof token === "string") csrfToken = token;
  return data as T;
}

export const get = <T>(path: string) => api<T>("GET", path);
export const post = <T>(path: string, body?: unknown) => api<T>("POST", path, body ?? {});
export const patch = <T>(path: string, body: unknown) => api<T>("PATCH", path, body);
export const put = <T>(path: string) => api<T>("PUT", path);
export const del = <T>(path: string) => api<T>("DELETE", path);

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  return "Não foi possível conectar ao servidor.";
}
