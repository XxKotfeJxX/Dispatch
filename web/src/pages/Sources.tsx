import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Clipboard, KeyRound, Plus, Power, RotateCcw, Trash2, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { api, Recipient } from '../api'
import { Failure, Loading } from '../components/State'

type Mapping = {
  id_path:string; event_type_path:string; subject_path:string; body_path:string
  default_event_type:string; default_subject:string; requested_channels:string[]
}
type Source = {
  id:string; name:string; slug:string; provider:string; recipient_id:string
  auth_mode:string; auth_header?:string; signature_header?:string
  mapping:Mapping; enabled:boolean; created_at:string
}
type Created = {data:{source:Source;secret:string};endpoint:string;warning:string}
const providerHelp:Record<string,string>={
  generic:'Any JSON-producing service',
  github:'GitHub repository webhooks',
  gitlab:'GitLab project webhooks',
  discord:'Discord bot or bridge events',
  slack:'Slack Events API',
  stripe:'Stripe webhook events',
  sentry:'Sentry issue webhooks',
  grafana:'Grafana alert webhooks',
}

export default function Sources({startOpen=false}:{startOpen?:boolean}){
  const client=useQueryClient()
  const sources=useQuery({queryKey:['sources'],queryFn:()=>api<{data:Source[];providers:string[]}>('/sources')})
  const recipients=useQuery({queryKey:['recipients'],queryFn:()=>api<{data:Recipient[]}>('/recipients')})
  const [showCreate,setShowCreate]=useState(startOpen),[created,setCreated]=useState<Created|null>(null),[copied,setCopied]=useState(false)
  const [form,setForm]=useState({name:'',slug:'',provider:'generic',recipient_id:'',channels:'email',secret:'',id_path:'',event_type_path:'',subject_path:'',body_path:''})
  const update=(key:string,value:string)=>setForm(previous=>({...previous,[key]:value}))
  const create=useMutation({
    mutationFn:()=>api<Created>('/sources',{method:'POST',body:JSON.stringify({
      name:form.name,slug:form.slug,provider:form.provider,recipient_id:form.recipient_id,secret:form.secret,
      mapping:{
        id_path:form.id_path,event_type_path:form.event_type_path,
        subject_path:form.subject_path,body_path:form.body_path,
        requested_channels:form.channels.split(',').map(x=>x.trim()).filter(Boolean),
      },
    })}),
    onSuccess:value=>{setCreated(value);setShowCreate(false);client.invalidateQueries({queryKey:['sources']})},
  })
  const remove=useMutation({mutationFn:(id:string)=>api(`/sources/${id}`,{method:'DELETE'}),onSuccess:()=>client.invalidateQueries({queryKey:['sources']})})
  const toggle=useMutation({mutationFn:(source:Source)=>api(`/sources/${source.id}/enabled`,{method:'POST',body:JSON.stringify({enabled:!source.enabled})}),onSuccess:()=>client.invalidateQueries({queryKey:['sources']})})
  const rotate=useMutation({mutationFn:({source,secret}:{source:Source;secret:string})=>api<{data:{secret:string};warning:string}>(`/sources/${source.id}/rotate-secret`,{method:'POST',body:JSON.stringify({secret})}),onSuccess:(value,{source})=>{setCreated({data:{source,secret:value.data.secret},endpoint:`/ingest/v1/${source.slug}`,warning:value.warning})}})
  const endpoint=created?`${location.origin}${created.endpoint}`:''
  const curl=useMemo(()=>created?`curl -X POST "${endpoint}" \\\n  -H "Authorization: Bearer ${created.data.secret}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"id":"test-001","event_type":"manual.test","subject":"Ingress works","body":"Hello from any source"}'`:'',[created,endpoint])
  if(sources.isLoading||recipients.isLoading)return <Loading/>
  if(sources.error)return <Failure error={sources.error}/>
  if(recipients.error)return <Failure error={recipients.error}/>
  return <div className="space-y-6">
    <div className="flex flex-wrap items-end justify-between gap-4"><div><p className="mb-1 text-xs font-semibold uppercase tracking-[.2em] text-cyan-400">Universal ingress</p><h1>Sources</h1><p className="muted mt-1">Accept authenticated events from almost any webhook-capable system.</p></div><button className={showCreate?'btn-secondary':'btn'} onClick={()=>setShowCreate(previous=>!previous)}>{showCreate?<X size={16}/>:<Plus size={16}/>} {showCreate?'Cancel':'New source'}</button></div>
    {created&&<section className="panel border-cyan-400/25 bg-cyan-400/[.04]"><div className="flex items-start justify-between gap-4"><div><div className="flex items-center gap-2"><KeyRound size={18} className="text-cyan-300"/><h2>{created.data.secret?'Save this secret now':'Source configured'}</h2></div><p className="muted mt-2">{created.warning}</p></div><button aria-label="Dismiss secret" className="text-slate-500" onClick={()=>setCreated(null)}>×</button></div><div className="mt-5 grid gap-3"><div><label className="text-xs text-slate-500">Endpoint</label><code className="mt-1 block overflow-auto rounded-lg bg-black/25 p-3 text-sm text-cyan-200">{endpoint}</code></div>{created.data.secret&&<div><label className="text-xs text-slate-500">Secret</label><div className="mt-1 flex gap-2"><code className="block flex-1 overflow-auto rounded-lg bg-black/25 p-3 text-sm text-amber-200">{created.data.secret}</code><button className="btn-secondary" onClick={async()=>{await navigator.clipboard.writeText(created.data.secret);setCopied(true)}}>{copied?<Check size={16}/>:<Clipboard size={16}/>}</button></div></div>}{created.data.source.auth_mode==='bearer'&&created.data.secret&&<div><label className="text-xs text-slate-500">Ready-to-run test</label><pre className="mt-1 overflow-auto rounded-lg bg-black/25 p-4 text-xs leading-6 text-slate-300">{curl}</pre></div>}</div></section>}
    {showCreate&&<form className="panel space-y-5" onSubmit={event=>{event.preventDefault();create.mutate()}}><div><h2>Create ingress source</h2><p className="muted mt-1">Choose a preset, then override JSON paths only when the payload differs.</p></div><div className="grid gap-3 md:grid-cols-2"><input className="field" required placeholder="Display name" value={form.name} onChange={e=>{update('name',e.target.value);if(!form.slug)update('slug',e.target.value.toLowerCase().replace(/[^a-z0-9]+/g,'-').replace(/^-|-$/g,''))}}/><input className="field" required pattern="[a-z0-9][a-z0-9-]{2,62}" placeholder="endpoint-slug" value={form.slug} onChange={e=>update('slug',e.target.value)}/><select className="field" value={form.provider} onChange={e=>setForm(previous=>({...previous,provider:e.target.value,secret:''}))}>{sources.data!.providers.map(provider=><option key={provider} value={provider}>{provider} — {providerHelp[provider]}</option>)}</select><select className="field" required value={form.recipient_id} onChange={e=>update('recipient_id',e.target.value)}><option value="">Route to recipient…</option>{recipients.data!.data.map(person=><option key={person.id} value={person.id}>{person.name}</option>)}</select><input className="field md:col-span-2" placeholder="Delivery channels: email,telegram,webhook" value={form.channels} onChange={e=>update('channels',e.target.value)}/>{(form.provider==='slack'||form.provider==='stripe')&&<input className="field md:col-span-2" type="password" required autoComplete="off" placeholder={`${form.provider} signing secret`} value={form.secret} onChange={e=>update('secret',e.target.value)}/>}</div><details className="rounded-xl border border-white/8 p-4"><summary className="cursor-pointer text-sm text-slate-300">Advanced JSON mapping</summary><div className="mt-4 grid gap-3 md:grid-cols-2"><input className="field font-mono" placeholder="External ID path, e.g. event.id" value={form.id_path} onChange={e=>update('id_path',e.target.value)}/><input className="field font-mono" placeholder="Event type path" value={form.event_type_path} onChange={e=>update('event_type_path',e.target.value)}/><input className="field font-mono" placeholder="Subject path" value={form.subject_path} onChange={e=>update('subject_path',e.target.value)}/><input className="field font-mono" placeholder="Body path" value={form.body_path} onChange={e=>update('body_path',e.target.value)}/></div></details><button className="btn" disabled={create.isPending}>Create source</button>{create.error&&<p className="text-sm text-red-300">{create.error.message}</p>}</form>}
    <div className="grid gap-4 lg:grid-cols-2">{sources.data!.data.map(source=><article className="panel" key={source.id}><div className="flex items-start justify-between gap-4"><div><div className="flex items-center gap-2"><h2>{source.name}</h2><span className="badge">{source.provider}</span></div><p className="mt-2 font-mono text-xs text-cyan-300">/ingest/v1/{source.slug}</p></div><span className={`size-2 rounded-full ${source.enabled?'bg-emerald-400':'bg-slate-600'}`}/></div><dl className="mt-5 grid grid-cols-2 gap-3 text-sm"><div><dt className="text-slate-500">Authentication</dt><dd>{source.auth_mode}</dd></div><div><dt className="text-slate-500">Channels</dt><dd>{source.mapping.requested_channels.join(', ')||'automatic'}</dd></div><div><dt className="text-slate-500">Event mapping</dt><dd className="font-mono text-xs">{source.mapping.event_type_path||source.mapping.default_event_type}</dd></div><div><dt className="text-slate-500">Recipient</dt><dd>{recipients.data!.data.find(x=>x.id===source.recipient_id)?.name||source.recipient_id}</dd></div></dl><div className="mt-5 flex flex-wrap gap-2"><button className="btn-secondary" onClick={()=>toggle.mutate(source)}><Power size={14}/>{source.enabled?'Disable':'Enable'}</button><button className="btn-secondary" onClick={()=>{let secret='';if(source.auth_mode==='slack_signature'||source.auth_mode==='stripe_signature'){secret=window.prompt(`Paste the new ${source.provider} signing secret`)?.trim()||'';if(!secret)return}rotate.mutate({source,secret})}}><RotateCcw size={14}/>Rotate secret</button><button className="btn-secondary text-red-300" onClick={()=>remove.mutate(source.id)}><Trash2 size={14}/>Delete</button></div></article>)}</div>
    {sources.data!.data.length===0&&!showCreate&&<div className="panel py-14 text-center"><h2>No sources yet</h2><p className="muted mt-2">Create a generic source to connect your first webhook.</p></div>}
  </div>
}
