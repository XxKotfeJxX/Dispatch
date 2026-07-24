const base = import.meta.env.VITE_API_URL ?? ''

export function apiKey() {
  return localStorage.getItem('dispatch_api_key') || 'dispatch-local-development-key'
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${base}/api/v1${path}`, {
    ...options,
    headers: { 'Content-Type': 'application/json', 'X-API-Key': apiKey(), ...options.headers },
  })
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
