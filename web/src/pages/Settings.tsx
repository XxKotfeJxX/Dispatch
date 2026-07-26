import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api, setApiKey } from '../api'
import { Failure, Loading } from '../components/State'

type AIDecision = {
  category: string
  priority: 'low' | 'normal' | 'high' | 'critical'
  summary: string
  confidence: number
  reason_codes: string[]
}

type Settings = {
  version: string
  auth: { enabled: boolean }
  ai: {
    enabled: boolean
    model: string
    prompt_version: string
    min_confidence: number
    timeout: string
    max_input_chars: number
    summary_max_chars: number
    thinking_level: string
    categories: string[]
  }
  channels: Record<string, boolean>
  worker_concurrency: number
}

const examples = [
  { value: 'telegram.message', label: 'Telegram message' },
  { value: 'discord.message', label: 'Discord message' },
  { value: 'gmail.message', label: 'Gmail message' },
  { value: 'github.event', label: 'GitHub activity' },
  { value: 'google.calendar.event', label: 'Calendar event' },
  { value: 'generic.event', label: 'Other notification' },
]

export default function SettingsPage() {
  const query = useQuery({ queryKey: ['settings'], queryFn: () => api<{ data: Settings }>('/settings') })
  const [key, setKey] = useState('')
  const [sample, setSample] = useState({
    event_type: 'telegram.message',
    subject: '',
    body: '',
  })
  const preview = useMutation({
    mutationFn: () => api<{ data: AIDecision }>('/ai/preview', {
      method: 'POST',
      body: JSON.stringify({ ...sample, metadata: {} }),
    }),
  })

  if (query.isLoading) return <Loading />
  if (query.error) return <Failure error={query.error} />
  const value = query.data!.data

  return <div className="space-y-6">
    <div>
      <div className="mb-2 flex items-center gap-3">
        <h1>Settings</h1><span className="badge">{value.version}</span>
      </div>
      <p className="muted">Runtime state only. Secrets are never returned by the API.</p>
    </div>

    <section className="panel">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2>Console access</h2>
          <p className="muted mt-1">{value.auth.enabled
            ? 'API-key protection is enabled for this deployment.'
            : 'Local access mode is enabled. No sign-in is required.'}</p>
        </div>
        <span className={`badge ${value.auth.enabled ? '' : 'text-emerald-300'}`}>
          {value.auth.enabled ? 'Protected' : 'Local mode'}
        </span>
      </div>
      {value.auth.enabled && <div className="mt-4 flex gap-3">
        <input className="field" type="password" autoComplete="off" placeholder="Enter a new API key"
          value={key} onChange={event => setKey(event.target.value)} />
        <button className="btn" disabled={!key.trim()} onClick={() => {
          setApiKey(key); setKey(''); query.refetch()
        }}>Use for this tab</button>
      </div>}
    </section>

    <section className="panel">
      <h2>Provider readiness</h2>
      <div className="mt-5 grid gap-3 sm:grid-cols-3">
        {Object.entries(value.channels).map(([name, ready]) => <div
          className="rounded-xl border border-white/8 p-4" key={name}>
          <div className="flex items-center justify-between">
            <b className="capitalize">{name}</b>
            <span className={`size-2 rounded-full ${ready ? 'bg-emerald-400' : 'bg-slate-600'}`} />
          </div>
          <p className="muted mt-2">{ready ? 'Configured' : 'Unavailable'}</p>
        </div>)}
      </div>
    </section>

    <section className="panel">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2>AI analysis</h2>
          <p className="muted mt-1">AI summarizes and labels notifications. It never selects a destination.</p>
        </div>
        <span className={`badge ${value.ai.enabled ? 'text-emerald-300' : 'text-amber-300'}`}>
          {value.ai.enabled ? 'Enabled' : 'Fallback only'}
        </span>
      </div>
      <dl className="mt-5 grid gap-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <div><dt className="muted">Model</dt><dd>{value.ai.model}</dd></div>
        <div><dt className="muted">Confidence threshold</dt><dd>{Math.round(value.ai.min_confidence * 100)}%</dd></div>
        <div><dt className="muted">Response time limit</dt><dd>{value.ai.timeout}</dd></div>
        <div><dt className="muted">Analysis profile</dt><dd>{value.ai.prompt_version}</dd></div>
      </dl>
      <div className="mt-5">
        <p className="text-xs font-semibold uppercase tracking-[.14em] text-slate-500">Categories</p>
        <div className="mt-2 flex flex-wrap gap-2">
          {value.ai.categories.map(category => <span className="badge capitalize" key={category}>{category}</span>)}
        </div>
      </div>

      <form className="mt-7 border-t border-white/8 pt-6" onSubmit={event => {
        event.preventDefault()
        preview.mutate()
      }}>
        <div>
          <h3 className="text-base font-semibold">Test the analyzer</h3>
          <p className="muted mt-1">Paste a realistic notification to see exactly how Dispatch classifies it.</p>
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-2">
          <label className="space-y-1.5 text-xs text-slate-400">
            <span>Source</span>
            <select className="field" aria-label="AI test source" value={sample.event_type}
              onChange={event => setSample(previous => ({ ...previous, event_type: event.target.value }))}>
              {examples.map(example => <option key={example.value} value={example.value}>{example.label}</option>)}
            </select>
          </label>
          <label className="space-y-1.5 text-xs text-slate-400">
            <span>Title</span>
            <input className="field" aria-label="AI test title" required maxLength={500}
              placeholder="For example: Build failed"
              value={sample.subject}
              onChange={event => setSample(previous => ({ ...previous, subject: event.target.value }))} />
          </label>
          <label className="space-y-1.5 text-xs text-slate-400 md:col-span-2">
            <span>Message</span>
            <textarea className="field min-h-32 resize-y" aria-label="AI test message" required
              maxLength={value.ai.max_input_chars} placeholder="Paste the notification text here…"
              value={sample.body}
              onChange={event => setSample(previous => ({ ...previous, body: event.target.value }))} />
          </label>
        </div>
        <button className="btn mt-4" disabled={!value.ai.enabled || preview.isPending ||
          !sample.subject.trim() || !sample.body.trim()}>
          {preview.isPending ? 'Analyzing…' : 'Analyze sample'}
        </button>
        {!value.ai.enabled && <p className="mt-3 text-sm text-amber-300">
          Enable AI and configure the Gemini API key to run a test.
        </p>}
        {preview.error && <p className="mt-3 text-sm text-red-300">{preview.error.message}</p>}
      </form>

      {preview.data && <div className="mt-5 rounded-xl border border-cyan-400/20 bg-cyan-400/5 p-5">
        <div className="flex flex-wrap items-center gap-2">
          <span className="badge capitalize">{preview.data.data.category}</span>
          <span className="badge capitalize">{preview.data.data.priority}</span>
          <span className="text-xs text-slate-400">
            {Math.round(preview.data.data.confidence * 100)}% confidence
          </span>
        </div>
        <p className="mt-4 text-sm leading-6 text-slate-200">{preview.data.data.summary}</p>
        <p className="mt-3 text-xs text-slate-500">
          Signals: {preview.data.data.reason_codes.join(', ')}
        </p>
      </div>}
    </section>
  </div>
}
