import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity, Bot, CheckCircle2, CircleAlert, ExternalLink, FlaskConical,
  Github, KeyRound, Link2, MessageCircle, Play, PlugZap, Power,
  RefreshCcw, Search, Trash2, Webhook, Youtube,
} from 'lucide-react'
import { FormEvent, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, Recipient } from '../api'
import { Failure, Loading } from '../components/State'
import Sources from './Sources'

type Field = {
  name:string; label:string; type:string; required:boolean; secret:boolean
  placeholder?:string; help?:string
}
type Manifest = {
  id:string; name:string; summary:string; category:string; auth:string; transport:string
  availability:'available'|'setup_required'|'limited'|'unavailable'
  configured:boolean; capabilities:string[]; fields:Field[]; setup_hint?:string
  documentation?:string; authorization_url?:string
}
type Connection = {
  id:string; connector_id:string; name:string; recipient_id:string
  status:'connected'|'action_required'|'error'|'disabled'; account_label?:string
  config:Record<string,string>; last_tested_at?:string; last_event_at?:string
  last_error?:string; enabled:boolean
}
type CatalogResponse = {data:Manifest[];connections:Connection[]}
type CreateResponse = {data:Connection;test:{message:string};credentials_stored:boolean}

const icons:Record<string,typeof PlugZap>={
  demo:FlaskConical,telegram:MessageCircle,discord:Bot,viber:MessageCircle,
  github:Github,google:Activity,youtube:Youtube,chatgpt:Bot,webhook:Webhook,
}
const availabilityLabel:Record<Manifest['availability'],string>={
  available:'Available',setup_required:'Setup required',limited:'Limited',unavailable:'Unavailable',
}
const statusStyles:Record<Connection['status'],string>={
  connected:'border-emerald-400/20 bg-emerald-400/8 text-emerald-300',
  action_required:'border-amber-400/20 bg-amber-400/8 text-amber-300',
  error:'border-red-400/20 bg-red-400/8 text-red-300',
  disabled:'border-slate-400/20 bg-slate-400/8 text-slate-400',
}
const providerDetails:Record<string,{title:string;body:string;steps:string[]}>={
  telegram:{
    title:'Why does Telegram need a bot?',
    body:'Telegram does not allow Dispatch to read a personal account. The official integration is a bot that users message directly; Telegram then sends those bot updates to Dispatch.',
    steps:['Open @BotFather in Telegram and create a bot.','Paste the bot token here.','For live incoming messages, expose CONNECTOR_PUBLIC_URL through public HTTPS.'],
  },
  viber:{
    title:'Viber requires a chatbot account',
    body:'Viber also does not expose personal chats to third-party applications. Dispatch can only receive messages addressed to an official Viber chatbot, and new chatbot provisioning is commercial.',
    steps:['Obtain a commercially provisioned Viber chatbot.','Paste its auth token here.','Expose CONNECTOR_PUBLIC_URL through public HTTPS.'],
  },
  discord:{
    title:'Configure a Discord application first',
    body:'Dispatch must use an official Discord bot; it cannot silently read a personal Discord account. The current connector uses the allowlisted Gateway bridge.',
    steps:['Create an application and bot in the Discord Developer Portal.','Set DISCORD_CLIENT_ID, DISCORD_BOT_TOKEN, DISCORD_INGRESS_SECRET and DISCORD_ALLOWED_USER_IDS in .env.','Start docker compose with the discord profile.'],
  },
  github:{
    title:'Configure a GitHub App first',
    body:'GitHub authorization needs an App owned by this Dispatch deployment. A slug has not been configured, so there is nowhere safe to send the Install action yet.',
    steps:['Create a GitHub App for this deployment.','Set GITHUB_APP_SLUG in .env.','Restart Dispatch, then return here to install the App.'],
  },
  google:{
    title:'Configure Google OAuth first',
    body:'Google must know which application is requesting Gmail access. This deployment does not yet have an OAuth client ID and secret.',
    steps:['Create a Web OAuth client in Google Cloud Console.','Register the callback shown in docs/connectors.md.','Set GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET, then restart Dispatch.'],
  },
  youtube:{
    title:'YouTube uses a public channel feed',
    body:'No Google login is required for public channel updates. Dispatch subscribes to the channel WebSub feed, which still needs a public HTTPS callback.',
    steps:['Copy the channel ID beginning with UC.','Expose CONNECTOR_PUBLIC_URL through public HTTPS.','Create the connection and verify it with Send sample.'],
  },
}

export default function Integrations(){
  const client=useQueryClient(),[params]=useSearchParams()
  const advancedRef=useRef<HTMLElement>(null)
  const catalog=useQuery({queryKey:['connectors'],queryFn:()=>api<CatalogResponse>('/connectors')})
  const recipients=useQuery({queryKey:['recipients'],queryFn:()=>api<{data:Recipient[]}>('/recipients')})
  const [query,setQuery]=useState(''),[selected,setSelected]=useState<Manifest|null>(null),[setup,setSetup]=useState<Manifest|null>(null)
  const [advanced,setAdvanced]=useState(false),[notice,setNotice]=useState('')
  const [form,setForm]=useState({name:'',recipient_id:'',values:{} as Record<string,string>})
  const filtered=useMemo(()=>catalog.data?.data.filter(item=>
    `${item.name} ${item.summary} ${item.category}`.toLowerCase().includes(query.toLowerCase())
  )??[],[catalog.data,query])
  const categories=useMemo(()=>Array.from(new Set(filtered.map(item=>item.category))),[filtered])

  const reset=()=>{setSelected(null);setForm({name:'',recipient_id:'',values:{}})}
  const connect=useMutation({
    mutationFn:async()=>{
      if(!selected)throw new Error('Select a connector')
      if(selected.auth==='oauth2'){
        const result=await api<{authorization_url:string}>(`/connectors/${selected.id}/authorize`,{
          method:'POST',body:JSON.stringify({name:form.name,recipient_id:form.recipient_id}),
        })
        location.assign(result.authorization_url)
        return null
      }
      const config:Record<string,string>={},credentials:Record<string,string>={}
      ;(selected.fields??[]).forEach(field=>{(field.secret?credentials:config)[field.name]=form.values[field.name]??''})
      return api<CreateResponse>('/connections',{method:'POST',body:JSON.stringify({
        connector_id:selected.id,name:form.name,recipient_id:form.recipient_id,config,credentials,
      })})
    },
    onSuccess:value=>{
      if(!value)return
      setNotice(value.data.status==='connected'
        ? `${value.data.name} connected successfully.`
        : `${value.test.message} ${value.data.last_error??''}`)
      reset();client.invalidateQueries({queryKey:['connectors']})
    },
  })
  const test=useMutation({
    mutationFn:(id:string)=>api<{data:{message:string};status:string;activation_error?:string}>(`/connections/${id}/test`,{method:'POST',body:'{}'}),
    onSuccess:value=>{setNotice(`${value.data.message}${value.activation_error?` ${value.activation_error}`:''}`);client.invalidateQueries({queryKey:['connectors']})},
  })
  const sample=useMutation({
    mutationFn:(id:string)=>api<{data:{notification_id:string}}>(`/connections/${id}/sample`,{method:'POST',body:'{}'}),
    onSuccess:value=>setNotice(`Test notification ${value.data.notification_id} entered the delivery queue.`),
  })
  const toggle=useMutation({
    mutationFn:(connection:Connection)=>api(`/connections/${connection.id}/enabled`,{method:'POST',body:JSON.stringify({enabled:!connection.enabled})}),
    onSuccess:()=>client.invalidateQueries({queryKey:['connectors']}),
  })
  const remove=useMutation({
    mutationFn:(id:string)=>api(`/connections/${id}`,{method:'DELETE'}),
    onSuccess:()=>client.invalidateQueries({queryKey:['connectors']}),
  })

  const choose=(manifest:Manifest)=>{
    setNotice('')
    if(manifest.id==='webhook'){
      setAdvanced(true)
      window.setTimeout(()=>advancedRef.current?.scrollIntoView({behavior:'smooth',block:'start'}),0)
      return
    }
    if(!manifest.configured&&(manifest.auth==='app_install'||manifest.auth==='oauth2')){
      setSetup(manifest)
      return
    }
    if(manifest.auth==='app_install'){
      if(manifest.authorization_url)window.open(manifest.authorization_url,'_blank','noopener,noreferrer')
      else setSetup(manifest)
      return
    }
    if(manifest.availability==='unavailable'){setNotice(manifest.setup_hint??'Connector unavailable.');return}
    setSelected(manifest)
    setForm({name:`${manifest.name} connection`,recipient_id:recipients.data?.data[0]?.id??'',values:{}})
  }
  const submit=(event:FormEvent)=>{event.preventDefault();connect.mutate()}
  if(catalog.isLoading||recipients.isLoading)return <Loading/>
  if(catalog.error)return <Failure error={catalog.error}/>
  if(recipients.error)return <Failure error={recipients.error}/>
  const callbackMessage=params.get('connected')
    ? `${params.get('connected')} authorization completed.`
    : params.get('integration_error')?`Authorization failed: ${params.get('integration_error')}`:''

  return <div className="space-y-7">
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div><p className="mb-1 text-xs font-semibold uppercase tracking-[.2em] text-cyan-400">Connector platform</p><h1>Integrations</h1><p className="muted mt-1 max-w-2xl">Choose a service, authorize once, and let Dispatch manage event delivery and connection health.</p></div>
      <button className="btn-secondary" onClick={()=>setAdvanced(!advanced)}><Webhook size={16}/>Advanced webhooks</button>
    </div>
    {(notice||callbackMessage)&&<div className="flex items-start gap-3 rounded-xl border border-cyan-400/20 bg-cyan-400/5 p-4 text-sm text-cyan-100"><CheckCircle2 className="mt-0.5 shrink-0 text-cyan-300" size={18}/><span>{notice||callbackMessage}</span></div>}

    {catalog.data!.connections.length>0&&<section className="space-y-3">
      <div><h2>Connected accounts</h2><p className="muted mt-1">Test credentials, send a sample notification, or pause incoming events.</p></div>
      <div className="grid gap-4 lg:grid-cols-2">{catalog.data!.connections.map(connection=>{
        const manifest=catalog.data!.data.find(item=>item.id===connection.connector_id)
        const Icon=icons[connection.connector_id]??PlugZap
        return <article className="panel" key={connection.id}>
          <div className="flex items-start justify-between gap-3"><div className="flex gap-3"><span className="grid size-11 place-items-center rounded-xl bg-white/5 text-cyan-300"><Icon size={21}/></span><div><h2>{connection.name}</h2><p className="mt-1 text-sm text-slate-500">{manifest?.name} · {connection.account_label||'account pending'}</p></div></div><span className={`rounded-full border px-2.5 py-1 text-[11px] uppercase tracking-wide ${statusStyles[connection.status]}`}>{connection.status.replace('_',' ')}</span></div>
          {connection.last_error&&<p className="mt-4 rounded-lg bg-amber-400/5 p-3 text-xs text-amber-200">{connection.last_error}</p>}
          <div className="mt-5 flex flex-wrap gap-2">
            <button className="btn-secondary" disabled={test.isPending} onClick={()=>test.mutate(connection.id)}><RefreshCcw size={14}/>Test</button>
            <button className="btn-secondary" disabled={sample.isPending} onClick={()=>sample.mutate(connection.id)}><Play size={14}/>Send sample</button>
            <button className="btn-secondary" disabled={toggle.isPending} onClick={()=>toggle.mutate(connection)}><Power size={14}/>{connection.enabled?'Pause':'Enable'}</button>
            <button className="btn-secondary text-red-300" disabled={remove.isPending} onClick={()=>{
              if(window.confirm(`Disconnect ${connection.name}? Dispatch will stop receiving events and revoke provider access where supported.`)) remove.mutate(connection.id)
            }}><Trash2 size={14}/>Disconnect</button>
          </div>
        </article>
      })}</div>
    </section>}

    <section className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3"><div><h2>Connector catalog</h2><p className="muted mt-1">Capabilities and deployment requirements are shown before authorization.</p></div><label className="relative block"><Search className="absolute left-3 top-2.5 text-slate-600" size={16}/><input className="field w-72 pl-9" placeholder="Search services…" value={query} onChange={event=>setQuery(event.target.value)}/></label></div>
      {categories.map(category=><div className="space-y-3" key={category}><p className="text-xs font-semibold uppercase tracking-[.16em] text-slate-500">{category}</p><div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{filtered.filter(item=>item.category===category).map(manifest=>{
        const Icon=icons[manifest.id]??PlugZap
        const blocked=manifest.availability==='unavailable'
        const setupOnly=!manifest.configured&&(manifest.auth==='app_install'||manifest.auth==='oauth2')
        return <article className={`panel flex min-h-72 flex-col ${blocked?'opacity-70':''}`} key={manifest.id}>
          <div className="flex items-start justify-between gap-3"><span className="grid size-12 place-items-center rounded-xl bg-gradient-to-br from-cyan-400/15 to-indigo-400/10 text-cyan-300"><Icon size={23}/></span><span className="badge">{availabilityLabel[manifest.availability]}</span></div>
          <h2 className="mt-4">{manifest.name}</h2><p className="muted mt-2 leading-6">{manifest.summary}</p>
          <div className="mt-4 flex flex-wrap gap-1.5">{manifest.capabilities.slice(0,3).map(value=><span className="rounded-md bg-white/5 px-2 py-1 text-[11px] text-slate-400" key={value}>{value}</span>)}</div>
          <div className="mt-auto pt-5"><p className="mb-3 text-xs leading-5 text-slate-500">{manifest.setup_hint}</p><div className="flex gap-2"><button className={blocked||setupOnly?'btn-secondary flex-1':'btn flex-1'} disabled={blocked} onClick={()=>choose(manifest)}>{setupOnly||blocked?<CircleAlert size={15}/>:manifest.auth==='app_install'?<ExternalLink size={15}/>:<Link2 size={15}/>} {manifest.id==='webhook'?'Open builder':setupOnly?'View setup':manifest.auth==='app_install'?'Install':blocked?'Not available':'Connect'}</button>{manifest.documentation&&<a className="btn-secondary px-3" href={manifest.documentation} target="_blank" rel="noreferrer" aria-label={`${manifest.name} documentation`}><ExternalLink size={15}/></a>}</div></div>
        </article>
      })}</div></div>)}
    </section>

    {advanced&&<section ref={advancedRef} className="scroll-mt-20 space-y-3 border-t border-white/8 pt-7"><div className="flex items-center gap-3"><KeyRound className="text-cyan-300" size={20}/><div><h2>Advanced webhook sources</h2><p className="muted">For unsupported services and custom JSON producers.</p></div></div><Sources startOpen/></section>}

    {selected&&<div className="fixed inset-0 z-40 grid place-items-center overflow-y-auto bg-black/75 p-5 backdrop-blur-sm" onMouseDown={event=>{if(event.target===event.currentTarget)reset()}}>
      <form className="panel w-full max-w-xl space-y-5" onSubmit={submit}>
        <div className="flex items-start justify-between gap-4"><div><p className="text-xs font-semibold uppercase tracking-[.16em] text-cyan-400">Connect service</p><h2 className="mt-1">{selected.name}</h2><p className="muted mt-2">{selected.setup_hint}</p></div><button type="button" aria-label="Close" className="text-xl text-slate-500" onClick={reset}>×</button></div>
        <div className="grid gap-3 md:grid-cols-2"><label className="space-y-1.5 text-xs text-slate-400"><span>Connection name</span><input className="field" required value={form.name} onChange={event=>setForm(previous=>({...previous,name:event.target.value}))}/></label><label className="space-y-1.5 text-xs text-slate-400"><span>Send notifications to</span><select className="field" required value={form.recipient_id} onChange={event=>setForm(previous=>({...previous,recipient_id:event.target.value}))}>{recipients.data!.data.map(person=><option key={person.id} value={person.id}>{person.name}</option>)}</select></label></div>
        {providerDetails[selected.id]&&<div className="rounded-xl border border-cyan-400/15 bg-cyan-400/[.04] p-4"><h3 className="text-sm font-semibold text-cyan-200">{providerDetails[selected.id].title}</h3><p className="mt-2 text-xs leading-5 text-slate-400">{providerDetails[selected.id].body}</p></div>}
        {(selected.fields??[]).map(field=><label className="block space-y-1.5 text-xs text-slate-400" key={field.name}><span>{field.label}</span><input className="field" type={field.type} required={field.required} autoComplete={field.secret?'new-password':'off'} placeholder={field.placeholder} value={form.values[field.name]??''} onChange={event=>setForm(previous=>({...previous,values:{...previous.values,[field.name]:event.target.value}}))}/>{field.help&&<span className="block leading-5 text-slate-600">{field.help}</span>}</label>)}
        {recipients.data!.data.length===0&&<p className="rounded-lg bg-amber-400/8 p-3 text-sm text-amber-200">Create a recipient first. A connector needs to know where Dispatch should deliver its notifications.</p>}
        {connect.error&&<p className="rounded-lg bg-red-400/8 p-3 text-sm text-red-300">{connect.error.message}</p>}
        <div className="flex justify-end gap-2"><button type="button" className="btn-secondary" onClick={reset}>Cancel</button><button className="btn" disabled={connect.isPending||!form.recipient_id}>{connect.isPending?'Connecting…':selected.auth==='oauth2'?'Continue to authorization':'Verify and connect'}</button></div>
      </form>
    </div>}
    {setup&&<div className="fixed inset-0 z-40 grid place-items-center overflow-y-auto bg-black/75 p-5 backdrop-blur-sm" onMouseDown={event=>{if(event.target===event.currentTarget)setSetup(null)}}>
      <section className="panel w-full max-w-xl">
        <div className="flex items-start justify-between gap-4"><div><p className="text-xs font-semibold uppercase tracking-[.16em] text-amber-300">Deployment setup required</p><h2 className="mt-1">{setup.name}</h2></div><button type="button" aria-label="Close setup" className="text-xl text-slate-500" onClick={()=>setSetup(null)}>×</button></div>
        <h3 className="mt-5 font-semibold text-slate-200">{providerDetails[setup.id]?.title??'Configure this provider first'}</h3>
        <p className="muted mt-2 leading-6">{providerDetails[setup.id]?.body??setup.setup_hint}</p>
        <ol className="mt-5 space-y-3">{(providerDetails[setup.id]?.steps??[setup.setup_hint??'Complete provider configuration.']).map((step,index)=><li className="flex gap-3 text-sm text-slate-300" key={step}><span className="grid size-6 shrink-0 place-items-center rounded-full bg-white/6 text-xs text-cyan-300">{index+1}</span><span className="pt-0.5">{step}</span></li>)}</ol>
        <div className="mt-6 flex justify-end gap-2">{setup.documentation&&<a className="btn-secondary" href={setup.documentation} target="_blank" rel="noreferrer"><ExternalLink size={15}/>Provider docs</a>}<button className="btn" onClick={()=>setSetup(null)}>Got it</button></div>
      </section>
    </div>}
  </div>
}
