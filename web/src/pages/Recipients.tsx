import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2, ExternalLink, Inbox, Mail, Plus, Send, Trash2, Webhook, X,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { api, Recipient } from '../api'
import { Failure, Loading } from '../components/State'

type Destination = 'email' | 'telegram' | 'mailpit' | 'webhook'
type Setup = {
  id: string
  kind: 'telegram' | 'mailpit'
  name: string
  target?: string
  status: 'pending' | 'completed'
  recipient_id?: string
  expires_at: string
}
type SetupStart = {data: Setup; bot_url?: string; mailpit_url?: string}

const destinations: {id: Destination; name: string; help: string}[] = [
  {id: 'email', name: 'Email / Gmail', help: 'Send notifications to any valid email address.'},
  {id: 'telegram', name: 'Telegram', help: 'Connect your account safely through the Dispatch bot.'},
  {id: 'mailpit', name: 'Dispatch Mailpit', help: 'Create a local test mailbox and confirm it with a code.'},
  {id: 'webhook', name: 'Other service', help: 'Send processed notifications to another application through a webhook.'},
]

function destinationIcon(type: Destination, className = 'size-5') {
  switch (type) {
  case 'email': return <Mail className={className}/>
  case 'telegram': return <Send className={className}/>
  case 'mailpit': return <Inbox className={className}/>
  case 'webhook': return <Webhook className={className}/>
  }
}

export default function Recipients() {
  const client = useQueryClient()
  const query = useQuery({
    queryKey: ['recipients'],
    queryFn: () => api<{data: Recipient[]}>('/recipients'),
  })
  const [showCreate, setShowCreate] = useState(false)
  const [destination, setDestination] = useState<Destination>('email')
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [mailpitLogin, setMailpitLogin] = useState('')
  const [serviceName, setServiceName] = useState('')
  const [webhookURL, setWebhookURL] = useState('')
  const [setup, setSetup] = useState<SetupStart | null>(null)
  const [verificationCode, setVerificationCode] = useState('')

  const setupStatus = useQuery({
    queryKey: ['recipient-setup', setup?.data.id],
    queryFn: () => api<{data: Setup}>(`/recipient-setups/${setup!.data.id}`),
    enabled: setup?.data.kind === 'telegram',
    refetchInterval: query => query.state.data?.data.status === 'completed' ? false : 2000,
  })

  useEffect(() => {
    if (setupStatus.data?.data.status === 'completed') {
      client.invalidateQueries({queryKey: ['recipients']})
    }
  }, [client, setupStatus.data?.data.status])

  const create = useMutation({
    mutationFn: () => {
      const isEmail = destination === 'email'
      return api('/recipients', {
        method: 'POST',
        body: JSON.stringify({
          name,
          destination_type: destination,
          destination_label: isEmail ? email : serviceName,
          email: isEmail ? email : '',
          telegram_chat_id: '',
          webhook_url: destination === 'webhook' ? webhookURL : '',
          preferences: {
            default_channels: [isEmail ? 'email' : 'webhook'],
            disabled_channels: [],
            time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
          },
        }),
      })
    },
    onSuccess: () => {
      client.invalidateQueries({queryKey: ['recipients']})
      closeCreate()
    },
  })

  const startTelegram = useMutation({
    mutationFn: () => api<SetupStart>('/recipient-setups/telegram', {
      method: 'POST', body: JSON.stringify({name}),
    }),
    onSuccess: setSetup,
  })
  const startMailpit = useMutation({
    mutationFn: () => api<SetupStart>('/recipient-setups/mailpit', {
      method: 'POST', body: JSON.stringify({name, login: mailpitLogin}),
    }),
    onSuccess: setSetup,
  })
  const verifyMailpit = useMutation({
    mutationFn: () => api(`/recipient-setups/mailpit/${setup!.data.id}/verify`, {
      method: 'POST', body: JSON.stringify({code: verificationCode}),
    }),
    onSuccess: () => {
      client.invalidateQueries({queryKey: ['recipients']})
      closeCreate()
    },
  })
  const remove = useMutation({
    mutationFn: (id: string) => api(`/recipients/${id}`, {method: 'DELETE'}),
    onSuccess: () => client.invalidateQueries({queryKey: ['recipients']}),
  })

  function resetSetup() {
    setSetup(null)
    setVerificationCode('')
    startTelegram.reset()
    startMailpit.reset()
    verifyMailpit.reset()
  }

  function closeCreate() {
    setShowCreate(false)
    setDestination('email')
    setName('')
    setEmail('')
    setMailpitLogin('')
    setServiceName('')
    setWebhookURL('')
    resetSetup()
  }

  function changeDestination(value: Destination) {
    setDestination(value)
    resetSetup()
  }

  function submit(event: React.FormEvent) {
    event.preventDefault()
    if (destination === 'telegram') {
      startTelegram.mutate()
    } else if (destination === 'mailpit') {
      startMailpit.mutate()
    } else {
      create.mutate()
    }
  }

  if (query.isLoading) return <Loading/>
  if (query.error) return <Failure error={query.error}/>

  const activeError = create.error ?? startTelegram.error ?? startMailpit.error ?? verifyMailpit.error
  const telegramCompleted = setupStatus.data?.data.status === 'completed'

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1>Recipients</h1>
          <p className="muted mt-1">Choose where Dispatch should deliver processed notifications.</p>
        </div>
        <button
          className={showCreate ? 'btn-secondary' : 'btn'}
          onClick={() => showCreate ? closeCreate() : setShowCreate(true)}
        >
          {showCreate ? <><X className="size-4"/>Cancel</> : <><Plus className="size-4"/>Add destination</>}
        </button>
      </div>

      {showCreate && (
        <form className="panel space-y-6" onSubmit={submit}>
          <div className="border-b border-white/8 pb-4">
            <h2>Add a delivery destination</h2>
            <p className="muted mt-1">No API keys, chat IDs, or technical channel names required.</p>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <label className="space-y-2">
              <span className="text-sm font-medium">Destination name</span>
              <input
                className="field"
                placeholder="For example: My notifications"
                required
                value={name}
                onChange={event => setName(event.target.value)}
              />
            </label>
            <label className="space-y-2">
              <span className="text-sm font-medium">Where should notifications arrive?</span>
              <select
                className="field"
                disabled={Boolean(setup)}
                value={destination}
                onChange={event => changeDestination(event.target.value as Destination)}
              >
                {destinations.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
              </select>
            </label>
          </div>

          <div className="rounded-xl border border-cyan-400/15 bg-cyan-400/[0.035] p-4">
            <div className="flex gap-3">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-cyan-400/10 text-cyan-300">
                {destinationIcon(destination)}
              </div>
              <div>
                <div className="font-medium">{destinations.find(item => item.id === destination)?.name}</div>
                <p className="muted mt-1">{destinations.find(item => item.id === destination)?.help}</p>
              </div>
            </div>
          </div>

          {destination === 'email' && (
            <label className="block space-y-2">
              <span className="text-sm font-medium">Email address</span>
              <input
                autoComplete="email"
                className="field"
                placeholder="you@example.com"
                required
                type="email"
                value={email}
                onChange={event => setEmail(event.target.value)}
              />
              <span className="muted">Works with Gmail, Outlook, Proton Mail, or any other provider when this Dispatch installation has SMTP configured.</span>
            </label>
          )}

          {destination === 'webhook' && (
            <div className="grid gap-4 md:grid-cols-2">
              <label className="space-y-2">
                <span className="text-sm font-medium">Service name</span>
                <input
                  className="field"
                  placeholder="For example: My dashboard"
                  required
                  value={serviceName}
                  onChange={event => setServiceName(event.target.value)}
                />
              </label>
              <label className="space-y-2">
                <span className="text-sm font-medium">Webhook URL</span>
                <input
                  className="field"
                  placeholder="https://example.com/dispatch"
                  required
                  type="url"
                  value={webhookURL}
                  onChange={event => setWebhookURL(event.target.value)}
                />
              </label>
            </div>
          )}

          {destination === 'mailpit' && !setup && (
            <label className="block space-y-2">
              <span className="text-sm font-medium">Mailpit mailbox name</span>
              <div className="flex items-center">
                <input
                  className="field rounded-r-none"
                  pattern="[a-zA-Z0-9][a-zA-Z0-9._-]{1,62}"
                  placeholder="alex"
                  required
                  value={mailpitLogin}
                  onChange={event => setMailpitLogin(event.target.value)}
                />
                <span className="rounded-r-lg border border-l-0 border-white/10 bg-white/5 px-3 py-2.5 text-sm text-slate-500">@dispatch.local</span>
              </div>
              <span className="muted">Dispatch will send a six-digit code to this local mailbox.</span>
            </label>
          )}

          {destination === 'mailpit' && setup && (
            <div className="space-y-4 rounded-xl border border-emerald-400/15 bg-emerald-400/[0.035] p-4">
              <div>
                <div className="font-medium text-emerald-200">Verification message sent</div>
                <p className="muted mt-1">Open Mailpit, find the message sent to {setup.data.target}, and enter its code.</p>
              </div>
              <div className="flex flex-wrap gap-3">
                <a className="btn-secondary" href={setup.mailpit_url} rel="noreferrer" target="_blank">
                  Open Mailpit <ExternalLink className="size-4"/>
                </a>
                <input
                  aria-label="Mailpit verification code"
                  className="field max-w-48 text-center font-mono text-lg tracking-[.35em]"
                  inputMode="numeric"
                  maxLength={6}
                  pattern="[0-9]{6}"
                  placeholder="000000"
                  required
                  value={verificationCode}
                  onChange={event => setVerificationCode(event.target.value.replace(/\D/g, ''))}
                />
                <button
                  className="btn"
                  disabled={verifyMailpit.isPending || verificationCode.length !== 6}
                  onClick={event => {event.preventDefault(); verifyMailpit.mutate()}}
                >
                  Verify mailbox
                </button>
              </div>
            </div>
          )}

          {destination === 'telegram' && !setup && (
            <div className="rounded-xl border border-white/8 bg-black/10 p-4 text-sm text-slate-300">
              Dispatch will create a secure one-time link. Telegram confirms your identity when you press <strong>Start</strong> in the bot chat.
              Your phone number and username are not requested or exposed.
            </div>
          )}

          {destination === 'telegram' && setup && !telegramCompleted && (
            <div className="space-y-4 rounded-xl border border-[#26A5E4]/20 bg-[#26A5E4]/5 p-4">
              <div>
                <div className="font-medium text-sky-200">Finish in Telegram</div>
                <p className="muted mt-1">Open the Dispatch bot and press Start. This page will confirm the connection automatically.</p>
              </div>
              <a className="btn" href={setup.bot_url} rel="noreferrer" target="_blank">
                Open Telegram <ExternalLink className="size-4"/>
              </a>
              <div className="flex items-center gap-2 text-xs text-slate-500">
                <span className="size-2 animate-pulse rounded-full bg-sky-400"/>Waiting for confirmation…
              </div>
            </div>
          )}

          {destination === 'telegram' && telegramCompleted && (
            <div className="flex items-center gap-3 rounded-xl border border-emerald-400/20 bg-emerald-400/5 p-4 text-emerald-200">
              <CheckCircle2 className="size-5"/>
              <div>
                <div className="font-medium">Telegram connected</div>
                <div className="mt-1 text-sm text-slate-400">The Dispatch bot sent a confirmation message to your chat.</div>
              </div>
            </div>
          )}

          {activeError && (
            <div className="rounded-lg border border-red-400/20 bg-red-400/5 p-3 text-sm text-red-300">
              {activeError.message}
            </div>
          )}

          <div className="flex justify-end gap-3">
            <button className="btn-secondary" onClick={closeCreate} type="button">
              {telegramCompleted ? 'Done' : 'Cancel'}
            </button>
            {!setup && (
              <button className="btn" disabled={create.isPending || startTelegram.isPending || startMailpit.isPending}>
                {destination === 'telegram' ? 'Connect Telegram' :
                  destination === 'mailpit' ? 'Send verification code' : 'Add destination'}
              </button>
            )}
          </div>
        </form>
      )}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {query.data!.data.map(item => {
          const type = (item.destination_type || item.preferences.default_channels[0] || 'email') as Destination
          const detail = item.destination_label || item.email || item.webhook_url ||
            (item.telegram_chat_id ? 'Connected Telegram account' : '')
          return (
            <article className="panel" key={item.id}>
              <div className="flex items-start justify-between gap-3">
                <div className="flex min-w-0 items-center gap-3">
                  <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-white/5 text-cyan-300">
                    {destinationIcon(type)}
                  </div>
                  <div className="min-w-0">
                    <h2 className="truncate">{item.name}</h2>
                    <p className="muted mt-1 truncate">{detail}</p>
                  </div>
                </div>
                <button
                  aria-label={`Delete ${item.name}`}
                  className="btn-secondary px-3 text-red-300"
                  onClick={() => remove.mutate(item.id)}
                >
                  <Trash2 className="size-4"/>
                </button>
              </div>
              <div className="mt-5 flex flex-wrap gap-2">
                <span className="badge">{destinations.find(destination => destination.id === type)?.name ?? type}</span>
                <span className="badge text-emerald-300">Ready</span>
              </div>
            </article>
          )
        })}
      </div>

      {query.data!.data.length === 0 && (
        <div className="panel py-12 text-center">
          <Inbox className="mx-auto size-8 text-slate-600"/>
          <h2 className="mt-4">No delivery destinations</h2>
          <p className="muted mt-2">Add Email, Telegram, Dispatch Mailpit, or another service.</p>
        </div>
      )}
    </div>
  )
}
