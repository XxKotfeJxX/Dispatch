const base = import.meta.env.VITE_API_URL ?? ''
let currentAPIKey = ''

export function apiKey() {
  return currentAPIKey
}

export function setApiKey(value: string) {
  currentAPIKey = value.trim()
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {'Content-Type': 'application/json'}
  if (apiKey()) headers['X-API-Key'] = apiKey()
  const response = await fetch(`${base}/api/v1${path}`, {
    ...options,
    headers: {...headers, ...options.headers},
  })
  if (response.status === 401) window.dispatchEvent(new Event('dispatch:unauthorized'))
  if (!response.ok) {
    const payload = await response.json().catch(() => ({ error: { message: response.statusText } }))
    throw new Error(payload.error?.message ?? `Request failed (${response.status})`)
  }
  if (response.status === 204) return undefined as T
  return response.json()
}

export type Notification = {
  id: string; idempotency_key: string; recipient_id: string; event_type: string
  subject: string; body: string; category?: string; priority: string; status: string
  requested_channels: string[]; created_at: string; scheduled_at?: string
}
export type Recipient = {
  id: string; name: string; email?: string; telegram_chat_id?: string; webhook_url?: string
  preferences: { default_channels: string[]; disabled_channels?: string[]; time_zone?: string }
}
