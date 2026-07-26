import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Braces, Eye, Filter, Pencil, Plus, Power, Trash2, X,
} from 'lucide-react'
import { useRef, useState } from 'react'
import { api } from '../api'
import { BrandLogo } from '../components/BrandLogo'
import { Failure, Loading } from '../components/State'

type Condition = {field: string; operator: string; value: string}
type Template = {
  id: string
  name: string
  channel: string
  service: string
  conditions: Condition[]
  enabled: boolean
  subject_template: string
  body_template: string
  updated_at: string
}
type Draft = Omit<Template, 'id' | 'updated_at'>

const services = [
  {id: 'any', name: 'Any service'},
  {id: 'github', name: 'GitHub'},
  {id: 'google', name: 'Google'},
  {id: 'discord', name: 'Discord'},
  {id: 'telegram', name: 'Telegram'},
  {id: 'youtube', name: 'YouTube'},
  {id: 'webhook', name: 'Developer webhooks'},
]

const variables: Record<string, {key: string; label: string}[]> = {
  common: [
    {key: 'subject', label: 'Original title'},
    {key: 'body', label: 'Original message'},
    {key: 'sender', label: 'Sender'},
    {key: 'event_type', label: 'Event type'},
    {key: 'service', label: 'Service'},
    {key: 'summary', label: 'AI summary'},
    {key: 'priority', label: 'Priority'},
    {key: 'url', label: 'Open link'},
  ],
  github: [
    {key: 'repository', label: 'Repository'},
    {key: 'action', label: 'Action'},
    {key: 'event', label: 'GitHub event'},
  ],
  google: [
    {key: 'provider', label: 'Google product'},
    {key: 'from', label: 'Email sender'},
    {key: 'attachments', label: 'Attachments'},
    {key: 'actor', label: 'Drive actor'},
  ],
  discord: [
    {key: 'author_username', label: 'Username'},
    {key: 'channel_name', label: 'Channel'},
    {key: 'guild_name', label: 'Server'},
  ],
  telegram: [{key: 'chat_title', label: 'Chat'}],
  youtube: [{key: 'channel_title', label: 'Channel'}],
  webhook: [{key: 'provider', label: 'Provider'}],
}

const conditionFields = [
  {id: 'sender', name: 'Sender'},
  {id: 'content', name: 'Title or message'},
  {id: 'subject', name: 'Title'},
  {id: 'body', name: 'Message'},
  {id: 'event_type', name: 'Event type'},
  {id: 'repository', name: 'Repository'},
  {id: 'provider', name: 'Product / provider'},
]

const operators = [
  {id: 'contains', name: 'contains'},
  {id: 'not_contains', name: 'does not contain'},
  {id: 'equals', name: 'equals'},
  {id: 'starts_with', name: 'starts with'},
]

const emptyDraft = (): Draft => ({
  name: '',
  channel: 'all',
  service: 'any',
  conditions: [],
  enabled: true,
  subject_template: '{{subject}}',
  body_template: '{{body}}\n\n{{url}}',
})

const sampleValues: Record<string, Record<string, string>> = {
  any: {
    subject: 'A new notification arrived', body: 'This is the original message from the service.',
    sender: 'Alex', event_type: 'notification.created', service: 'service',
    summary: 'A short AI-generated summary', priority: 'normal', url: 'https://example.com',
  },
  github: {
    subject: 'Pull request #42 merged', body: 'Improve notification templates',
    sender: 'octocat', event_type: 'github.pull_request', service: 'github',
    summary: 'PR #42 was merged', priority: 'normal', url: 'https://github.com/acme/dispatch/pull/42',
    repository: 'acme/dispatch', action: 'closed', event: 'pull_request',
  },
  google: {
    subject: 'Quarterly report', body: 'Please review the attached report before Friday.',
    sender: 'alex@example.com', from: 'Alex <alex@example.com>', event_type: 'gmail.message.received',
    service: 'google', provider: 'gmail', summary: 'Report needs review', priority: 'high',
    attachments: 'report.xlsx', actor: 'Alex', url: 'https://mail.google.com/',
  },
  discord: {
    subject: 'Discord: Alex in #general', body: 'Can you review the latest build?',
    sender: 'Alex', author_username: 'Alex', event_type: 'discord.message.created',
    service: 'discord', channel_name: 'general', guild_name: 'Dispatch',
    summary: 'Build review requested', priority: 'normal', url: '',
  },
  telegram: {
    subject: 'Telegram message from Alex', body: 'The deployment is ready.',
    sender: 'Alex', event_type: 'telegram.message.created', service: 'telegram',
    chat_title: 'Operations', summary: 'Deployment is ready', priority: 'normal', url: '',
  },
  youtube: {
    subject: 'New video published', body: 'Dispatch 0.6 release overview',
    sender: 'Dispatch', event_type: 'youtube.video.published', service: 'youtube',
    channel_title: 'Dispatch', summary: 'A new release video', priority: 'low',
    url: 'https://youtube.com/watch?v=example',
  },
  webhook: {
    subject: 'Production alert', body: 'API latency exceeded 500 ms.',
    sender: 'Monitoring', event_type: 'monitoring.alert', service: 'webhook',
    provider: 'monitoring', summary: 'High API latency', priority: 'high', url: '',
  },
}

function renderPreview(value: string, service: string) {
  const sample = sampleValues[service] ?? sampleValues.any
  return value.replace(/\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}/g, (_, key: string) => sample[key] ?? '')
}

function serviceName(id: string) {
  return services.find(service => service.id === id)?.name ?? id
}

export default function Templates() {
  const client = useQueryClient()
  const query = useQuery({queryKey: ['templates'], queryFn: () => api<{data: Template[]}>('/templates')})
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [editingID, setEditingID] = useState<string | null>(null)
  const [showEditor, setShowEditor] = useState(false)
  const [activeField, setActiveField] = useState<'subject_template' | 'body_template'>('body_template')
  const subjectRef = useRef<HTMLInputElement>(null)
  const bodyRef = useRef<HTMLTextAreaElement>(null)

  const save = useMutation({
    mutationFn: () => api(editingID ? `/templates/${editingID}` : '/templates', {
      method: editingID ? 'PUT' : 'POST',
      body: JSON.stringify(draft),
    }),
    onSuccess: () => {
      closeEditor()
      client.invalidateQueries({queryKey: ['templates']})
    },
  })
  const remove = useMutation({
    mutationFn: (id: string) => api(`/templates/${id}`, {method: 'DELETE'}),
    onSuccess: () => client.invalidateQueries({queryKey: ['templates']}),
  })
  const toggle = useMutation({
    mutationFn: (item: Template) => api(`/templates/${item.id}`, {
      method: 'PUT',
      body: JSON.stringify({...item, enabled: !item.enabled}),
    }),
    onSuccess: () => client.invalidateQueries({queryKey: ['templates']}),
  })

  function closeEditor() {
    setDraft(emptyDraft())
    setEditingID(null)
    setShowEditor(false)
  }

  function edit(item: Template) {
    setDraft({
      name: item.name, channel: item.channel, service: item.service,
      conditions: item.conditions ?? [], enabled: item.enabled,
      subject_template: item.subject_template, body_template: item.body_template,
    })
    setEditingID(item.id)
    setShowEditor(true)
    window.scrollTo({top: 0, behavior: 'smooth'})
  }

  function insertVariable(key: string) {
    const token = `{{${key}}}`
    const element = activeField === 'subject_template' ? subjectRef.current : bodyRef.current
    const value = draft[activeField]
    const start = element?.selectionStart ?? value.length
    const end = element?.selectionEnd ?? value.length
    const updated = value.slice(0, start) + token + value.slice(end)
    setDraft(previous => ({...previous, [activeField]: updated}))
    requestAnimationFrame(() => {
      element?.focus()
      element?.setSelectionRange(start + token.length, start + token.length)
    })
  }

  function addCondition() {
    setDraft(previous => ({
      ...previous,
      conditions: [...previous.conditions, {field: 'sender', operator: 'contains', value: ''}],
    }))
  }

  function updateCondition(index: number, patch: Partial<Condition>) {
    setDraft(previous => ({
      ...previous,
      conditions: previous.conditions.map((condition, current) =>
        current === index ? {...condition, ...patch} : condition),
    }))
  }

  if (query.isLoading) return <Loading/>
  if (query.error) return <Failure error={query.error}/>

  const availableVariables = [
    ...variables.common,
    ...(draft.service === 'any' ? [] : variables[draft.service] ?? []),
  ]

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1>Templates</h1>
          <p className="muted mt-1">Control how notifications from each service look when Dispatch sends them.</p>
        </div>
        <button
          className={showEditor ? 'btn-secondary' : 'btn'}
          onClick={() => showEditor ? closeEditor() : setShowEditor(true)}
        >
          {showEditor ? <><X className="size-4"/>Cancel</> : <><Plus className="size-4"/>New template</>}
        </button>
      </div>

      {showEditor && (
        <form className="panel space-y-6" onSubmit={event => {event.preventDefault(); save.mutate()}}>
          <div className="flex items-center justify-between border-b border-white/8 pb-4">
            <div>
              <h2>{editingID ? 'Edit template' : 'Create a template'}</h2>
              <p className="muted mt-1">Choose a service, then build the message with ready-made fields.</p>
            </div>
            <span className="badge">3 simple steps</span>
          </div>

          <section className="grid gap-4 lg:grid-cols-2">
            <label className="space-y-2">
              <span className="text-sm font-medium"><span className="mr-2 text-cyan-300">1.</span>Template name</span>
              <input
                className="field"
                placeholder="For example: Important GitHub activity"
                required
                value={draft.name}
                onChange={event => setDraft(previous => ({...previous, name: event.target.value}))}
              />
            </label>
            <label className="space-y-2">
              <span className="text-sm font-medium"><span className="mr-2 text-cyan-300">2.</span>Apply to service</span>
              <select
                className="field"
                value={draft.service}
                onChange={event => setDraft(previous => ({...previous, service: event.target.value}))}
              >
                {services.map(service => <option key={service.id} value={service.id}>{service.name}</option>)}
              </select>
            </label>
          </section>

          <section className="grid gap-5 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,.85fr)]">
            <div className="space-y-4">
              <div>
                <div className="mb-2 flex items-center gap-2">
                  <span className="text-sm font-medium"><span className="mr-2 text-cyan-300">3.</span>Message</span>
                  <span className="muted">Click a field below to insert it.</span>
                </div>
                <div className="flex flex-wrap gap-2 rounded-xl border border-white/8 bg-black/15 p-3">
                  <Braces className="mt-1 size-4 shrink-0 text-cyan-300"/>
                  {availableVariables.map(variable => (
                    <button
                      className="rounded-md border border-white/10 bg-white/5 px-2.5 py-1 text-xs text-slate-300 transition hover:border-cyan-400/40 hover:text-cyan-200"
                      key={variable.key}
                      onClick={() => insertVariable(variable.key)}
                      title={`Insert {{${variable.key}}}`}
                      type="button"
                    >
                      {variable.label}
                    </button>
                  ))}
                </div>
              </div>
              <label className="block space-y-2">
                <span className="text-sm text-slate-300">Notification title</span>
                <input
                  className="field font-mono"
                  onFocus={() => setActiveField('subject_template')}
                  ref={subjectRef}
                  value={draft.subject_template}
                  onChange={event => setDraft(previous => ({...previous, subject_template: event.target.value}))}
                />
              </label>
              <label className="block space-y-2">
                <span className="text-sm text-slate-300">Notification message</span>
                <textarea
                  className="field min-h-48 resize-y font-mono leading-6"
                  onFocus={() => setActiveField('body_template')}
                  ref={bodyRef}
                  required
                  value={draft.body_template}
                  onChange={event => setDraft(previous => ({...previous, body_template: event.target.value}))}
                />
              </label>
            </div>

            <div className="rounded-xl border border-cyan-400/15 bg-[#08101b] p-4">
              <div className="mb-4 flex items-center gap-2 text-sm font-medium text-cyan-200">
                <Eye className="size-4"/> Live preview
              </div>
              <div className="rounded-xl border border-white/8 bg-white/[0.035] p-4">
                <div className="mb-4 flex items-center gap-3">
                  {draft.service !== 'any' && <BrandLogo className="size-8 shrink-0" service={draft.service}/>}
                  <div>
                    <div className="text-xs uppercase tracking-wider text-slate-500">{serviceName(draft.service)}</div>
                    <div className="mt-1 font-semibold">{renderPreview(draft.subject_template, draft.service) || 'No title'}</div>
                  </div>
                </div>
                <div className="whitespace-pre-wrap text-sm leading-6 text-slate-300">
                  {renderPreview(draft.body_template, draft.service) || 'No message'}
                </div>
              </div>
              <p className="muted mt-3">Preview uses safe example data. Real values are inserted automatically for every notification.</p>
            </div>
          </section>

          <section className="rounded-xl border border-white/8 bg-black/10 p-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex items-start gap-3">
                <Filter className="mt-0.5 size-4 text-violet-300"/>
                <div>
                  <div className="text-sm font-medium">When should this template be used?</div>
                  <p className="muted mt-1">
                    {draft.conditions.length === 0
                      ? `No conditions — this is the fallback for ${serviceName(draft.service)}.`
                      : 'All conditions below must match. More specific templates win automatically.'}
                  </p>
                </div>
              </div>
              <button className="btn-secondary" onClick={addCondition} type="button">
                <Plus className="size-4"/>Add condition
              </button>
            </div>
            {draft.conditions.length > 0 && (
              <div className="mt-4 space-y-3">
                {draft.conditions.map((condition, index) => (
                  <div className="grid gap-2 md:grid-cols-[1fr_1fr_1.4fr_auto]" key={index}>
                    <select
                      className="field"
                      value={condition.field}
                      onChange={event => updateCondition(index, {field: event.target.value})}
                    >
                      {conditionFields.map(field => <option key={field.id} value={field.id}>{field.name}</option>)}
                    </select>
                    <select
                      className="field"
                      value={condition.operator}
                      onChange={event => updateCondition(index, {operator: event.target.value})}
                    >
                      {operators.map(operator => <option key={operator.id} value={operator.id}>{operator.name}</option>)}
                    </select>
                    <input
                      className="field"
                      placeholder="Value to match"
                      required
                      value={condition.value}
                      onChange={event => updateCondition(index, {value: event.target.value})}
                    />
                    <button
                      aria-label="Remove condition"
                      className="btn-secondary px-3 text-red-300"
                      onClick={() => setDraft(previous => ({
                        ...previous,
                        conditions: previous.conditions.filter((_, current) => current !== index),
                      }))}
                      type="button"
                    >
                      <Trash2 className="size-4"/>
                    </button>
                  </div>
                ))}
              </div>
            )}
          </section>

          {save.error && <div className="rounded-lg border border-red-400/20 bg-red-400/5 p-3 text-sm text-red-300">{save.error.message}</div>}
          <div className="flex justify-end gap-3">
            <button className="btn-secondary" onClick={closeEditor} type="button">Cancel</button>
            <button className="btn" disabled={save.isPending}>
              {save.isPending ? 'Saving…' : editingID ? 'Save changes' : 'Create template'}
            </button>
          </div>
        </form>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        {query.data!.data.map(item => (
          <article className={`panel ${item.enabled ? '' : 'opacity-60'}`} key={item.id}>
            <div className="flex items-start justify-between gap-4">
              <div className="flex min-w-0 items-center gap-3">
                <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-white/5">
                  {item.service === 'any'
                    ? <Braces className="size-5 text-cyan-300"/>
                    : <BrandLogo className="size-6" service={item.service}/>}
                </div>
                <div className="min-w-0">
                  <h2 className="truncate">{item.name}</h2>
                  <div className="mt-1 flex flex-wrap gap-2">
                    <span className="badge">{serviceName(item.service)}</span>
                    <span className="badge">
                      {item.conditions.length === 0 ? 'Fallback' : `${item.conditions.length} condition${item.conditions.length === 1 ? '' : 's'}`}
                    </span>
                  </div>
                </div>
              </div>
              <div className="flex gap-1">
                <button aria-label="Enable or disable template" className="btn-secondary px-3" onClick={() => toggle.mutate(item)}>
                  <Power className={`size-4 ${item.enabled ? 'text-emerald-300' : 'text-slate-500'}`}/>
                </button>
                <button aria-label="Edit template" className="btn-secondary px-3" onClick={() => edit(item)}>
                  <Pencil className="size-4"/>
                </button>
                <button aria-label="Delete template" className="btn-secondary px-3 text-red-300" onClick={() => remove.mutate(item.id)}>
                  <Trash2 className="size-4"/>
                </button>
              </div>
            </div>
            {item.conditions.length > 0 && (
              <div className="mt-4 flex flex-wrap gap-2 text-xs text-slate-400">
                {item.conditions.map((condition, index) => (
                  <span className="rounded-md bg-violet-400/8 px-2.5 py-1" key={index}>
                    {conditionFields.find(field => field.id === condition.field)?.name ?? condition.field}{' '}
                    {operators.find(operator => operator.id === condition.operator)?.name ?? condition.operator}{' '}
                    <strong className="text-slate-300">{condition.value}</strong>
                  </span>
                ))}
              </div>
            )}
            <div className="mt-4 rounded-lg bg-black/20 p-4">
              <div className="text-sm font-medium">{item.subject_template || '{{subject}}'}</div>
              <pre className="mt-2 overflow-auto whitespace-pre-wrap text-xs leading-5 text-slate-400">{item.body_template}</pre>
            </div>
          </article>
        ))}
      </div>

      {query.data!.data.length === 0 && (
        <div className="panel py-12 text-center">
          <Braces className="mx-auto size-8 text-slate-600"/>
          <h2 className="mt-4">No templates yet</h2>
          <p className="muted mt-2">Create a fallback template first, then add special cases only when you need them.</p>
        </div>
      )}
    </div>
  )
}
