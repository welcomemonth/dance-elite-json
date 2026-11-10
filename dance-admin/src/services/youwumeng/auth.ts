import { request } from '@umijs/max';
import type { LoginParams, LoginResult } from './typings';

const TOKEN_KEY = 'yw_admin_token';
const ADMIN_KEY = 'yw_admin';

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function saveSession(token: string, admin: LoginResult['admin']) {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(ADMIN_KEY, JSON.stringify(admin));
}

export function getStoredAdmin(): LoginResult['admin'] | undefined {
  const raw = localStorage.getItem(ADMIN_KEY);
  if (!raw) return undefined;
  try {
    return JSON.parse(raw) as LoginResult['admin'];
  } catch {
    return undefined;
  }
}

export function clearSession() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(ADMIN_KEY);
}

/** POST /api/v1/admin/auth/login */
export async function login(params: LoginParams): Promise<LoginResult> {
  return request('/api/v1/admin/auth/login', {
    method: 'POST',
    data: params,
  });
}
