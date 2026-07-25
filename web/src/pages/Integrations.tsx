import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2, CircleAlert, ExternalLink, FlaskConical, Info,
  KeyRound, Play, Power, RefreshCcw, Trash2, Webhook, X,
} from 'lucide-react'
import { FormEvent, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, Recipient } from '../api'
import { BrandLogo } from '../components/BrandLogo'
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

const consumerServices = new Set(['discord', 'github', 'google', 'youtube'])
const statusStyles:Record<Connection['status'],string>={
  connected:'border-emerald-400/20 bg-emerald-400/8 text-emerald-300',
  action_required:'border-amber-400/20 bg-amber-400/8 text-amber-300',
  error:'border-red-400/20 bg-red-400/8 text-red-300',
  disabled:'border-slate-400/20 bg-slate-400/8 text-slate-400',
}

export default function Integrations(){
  const client=useQueryClient(),[params]=useSearchParams()
  const advancedRef=useRef<HTMLElement>(null)
  const catalog=useQuery({queryKey:['connectors'],queryFn:()=>api<CatalogResponse>('/connectors')})
  const recipients=useQuery({queryKey:['recipients'],queryFn:()=>api<{data:Recipient[]}>('/recipients')})
  const [selected,setSelected]=useState<Manifest|null>(null)
  const [unavailable,setUnavailable]=useState<Manifest|null>(null)
  const [advanced,setAdvanced]=useState(false),[notice,setNotice]=useState('')
  const [form,setForm]=useState({name:'',recipient_id:'',values:{} as Record<string,string>})

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

  const openAdvanced=()=>{
    setAdvanced(true)
    window.setTimeout(()=>advancedRef.current?.scrollIntoView({behavior:'smooth',block:'start'}),0)
  }
  const choose=(manifest:Manifest)=>{
    setNotice('')
    if(!manifest.configured){
      setUnavailable(manifest)
      return
    }
    if(manifest.auth==='app_install'){
      if(manifest.authorization_url)location.assign(manifest.authorization_url)
      else setUnavailable(manifest)
      return
    }
    setSelected(manifest)
    setForm({name:`${manifest.name} connection`,recipient_id:recipients.data?.data[0]?.id??'',values:{}})
  }
  const submit=(event:FormEvent)=>{event.preventDefault();connect.mutate()}
  if(catalog.isLoading||recipients.isLoading)return <Loading/>
  if(catalog.error)return <Failure error={catalog.error}/>
  if(recipients.error)return <Failure error={recipients.error}/>

  const manifests=catalog.data!.data.filter(item=>consumerServices.has(item.id))
  const demo=catalog.data!.data.find(item=>item.id==='demo')
  const callbackMessage=params.get('connected')
    ? `${params.get('connected')} authorization completed.`
    : params.get('integration_error')?`Authorization failed: ${params.get('integration_error')}`:''

  return <div className="space-y-8">
    <header className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p className="mb-1 text-xs font-semibold uppercase tracking-[.2em] text-cyan-400">Your services</p>
        <h1>Integrations</h1>
        <p className="muted mt-1 max-w-2xl">Choose a service and sign in. Dispatch handles the technical setup after authorization.</p>
      </div>
      <button className="btn-secondary" onClick={openAdvanced}><Webhook size={16}/>Developer webhooks</button>
    </header>

    {(notice||callbackMessage)&&<div className="flex items-start gap-3 rounded-xl border border-cyan-400/20 bg-cyan-400/5 p-4 text-sm text-cyan-100"><CheckCircle2 className="mt-0.5 shrink-0 text-cyan-300" size={18}/><span>{notice||callbackMessage}</span></div>}

    {catalog.data!.connections.length>0&&<section className="space-y-3">
      <div><h2>Connected accounts</h2><p className="muted mt-1">Check a connection, send a sample, pause it, or disconnect it.</p></div>
      <div className="grid gap-4 lg:grid-cols-2">{catalog.data!.connections.map(connection=>{
        const manifest=catalog.data!.data.find(item=>item.id===connection.connector_id)
        return <article className="panel" key={connection.id}>
          <div className="flex items-start justify-between gap-3">
            <div className="flex gap-3"><span className="grid size-11 place-items-center rounded-xl bg-white/5"><BrandLogo service={connection.connector_id} className="size-5"/></span><div><h2>{connection.name}</h2><p className="mt-1 text-sm text-slate-500">{manifest?.name} · {connection.account_label||'account pending'}</p></div></div>
            <span className={`rounded-full border px-2.5 py-1 text-[11px] uppercase tracking-wide ${statusStyles[connection.status]}`}>{connection.status.replace('_',' ')}</span>
          </div>
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

    <section className="space-y-4" aria-labelledby="service-catalog-title">
      <div><h2 id="service-catalog-title">Connect a service</h2><p className="muted mt-1">Click a card to begin. Hover the info icon for supported events and connection details.</p></div>
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-4">
        {manifests.map(manifest=><article
          className="group relative isolate h-64 overflow-visible rounded-3xl border border-white/10 bg-gradient-to-br from-white/[.08] to-white/[.025] shadow-xl shadow-black/10 transition duration-300 hover:z-20 hover:-translate-y-1 hover:border-cyan-300/35 hover:shadow-cyan-950/30 focus-within:z-20 focus-within:border-cyan-300/50"
          key={manifest.id}
        >
          <button
            aria-label={`Connect ${manifest.name}`}
            className="absolute inset-0 z-0 rounded-3xl outline-none focus-visible:ring-2 focus-visible:ring-cyan-300"
            onClick={()=>choose(manifest)}
          />
          <div className="pointer-events-none absolute inset-0 grid place-items-center p-12 transition duration-300 group-hover:scale-105">
            <BrandLogo service={manifest.id} className="h-24 w-32 max-w-full"/>
          </div>
          <div className="absolute left-4 top-4 z-20">
            <span
              aria-label={`About ${manifest.name}`}
              className="peer grid size-9 cursor-help place-items-center rounded-full border border-white/10 bg-slate-950/75 text-slate-300 backdrop-blur transition hover:border-cyan-300/40 hover:text-cyan-200 focus:border-cyan-300/40 focus:text-cyan-200 focus:outline-none"
              role="button"
              tabIndex={0}
            ><Info size={17}/></span>
            <aside className="pointer-events-none absolute left-0 top-12 z-30 w-72 translate-y-1 rounded-2xl border border-white/10 bg-slate-950/95 p-4 text-left opacity-0 shadow-2xl backdrop-blur transition peer-hover:translate-y-0 peer-hover:opacity-100 peer-focus:translate-y-0 peer-focus:opacity-100">
              <h3 className="font-semibold text-white">{manifest.name}</h3>
              <p className="mt-2 text-sm leading-5 text-slate-400">{manifest.summary}</p>
              <ul className="mt-3 space-y-1.5 text-xs text-slate-300">{manifest.capabilities.slice(0,4).map(capability=><li key={capability}>• {capability}</li>)}</ul>
              <p className={`mt-3 text-xs ${manifest.configured?'text-emerald-300':'text-amber-300'}`}>{manifest.configured?'Ready to connect':'Not enabled on this installation'}</p>
            </aside>
          </div>
          <div className="pointer-events-none absolute inset-x-0 bottom-0 z-10 translate-y-2 bg-gradient-to-t from-slate-950 via-slate-950/80 to-transparent px-6 pb-5 pt-12 opacity-0 transition duration-300 group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100">
            <p className="text-center text-xl font-semibold text-white">{manifest.name}</p>
          </div>
        </article>)}
      </div>
    </section>

    <section className="grid gap-4 md:grid-cols-2">
      {demo&&<button className="panel flex items-center gap-4 text-left transition hover:border-cyan-300/30" onClick={()=>choose(demo)}>
        <span className="grid size-12 shrink-0 place-items-center rounded-xl bg-cyan-400/10 text-cyan-300"><FlaskConical size={22}/></span>
        <span><strong className="block text-slate-100">Test Dispatch</strong><span className="muted mt-1 block text-sm">Generate a safe sample event without connecting an account.</span></span>
      </button>}
      <button className="panel flex items-center gap-4 text-left transition hover:border-cyan-300/30" onClick={openAdvanced}>
        <span className="grid size-12 shrink-0 place-items-center rounded-xl bg-indigo-400/10 text-indigo-300"><KeyRound size={22}/></span>
        <span><strong className="block text-slate-100">Developer tools</strong><span className="muted mt-1 block text-sm">Connect an unsupported service using a signed webhook.</span></span>
      </button>
    </section>

    {advanced&&<section ref={advancedRef} className="scroll-mt-20 space-y-3 border-t border-white/8 pt-7"><div className="flex items-center gap-3"><KeyRound className="text-cyan-300" size={20}/><div><h2>Developer webhook sources</h2><p className="muted">Advanced tools for custom systems and unsupported services.</p></div></div><Sources startOpen/></section>}

    {selected&&<div className="fixed inset-0 z-40 grid place-items-center overflow-y-auto bg-black/75 p-5 backdrop-blur-sm" onMouseDown={event=>{if(event.target===event.currentTarget)reset()}}>
      <form className="panel w-full max-w-xl space-y-5" onSubmit={submit}>
        <div className="flex items-start justify-between gap-4">
          <div><p className="text-xs font-semibold uppercase tracking-[.16em] text-cyan-400">Connect service</p><h2 className="mt-1">{selected.name}</h2><p className="muted mt-2">{selected.summary}</p></div>
          <button type="button" aria-label="Close" className="text-slate-500 hover:text-white" onClick={reset}><X/></button>
        </div>
        <div className="grid gap-3 md:grid-cols-2">
          <label className="space-y-1.5 text-xs text-slate-400"><span>Connection name</span><input className="field" required value={form.name} onChange={event=>setForm(previous=>({...previous,name:event.target.value}))}/></label>
          <label className="space-y-1.5 text-xs text-slate-400"><span>Send notifications to</span><select className="field" required value={form.recipient_id} onChange={event=>setForm(previous=>({...previous,recipient_id:event.target.value}))}>{recipients.data!.data.map(person=><option key={person.id} value={person.id}>{person.name}</option>)}</select></label>
        </div>
        {(selected.fields??[]).map(field=><label className="block space-y-1.5 text-xs text-slate-400" key={field.name}><span>{field.label}</span><input className="field" type={field.type} required={field.required} autoComplete={field.secret?'new-password':'off'} placeholder={field.placeholder} value={form.values[field.name]??''} onChange={event=>setForm(previous=>({...previous,values:{...previous.values,[field.name]:event.target.value}}))}/>{field.help&&<span className="block leading-5 text-slate-600">{field.help}</span>}</label>)}
        {recipients.data!.data.length===0&&<p className="rounded-lg bg-amber-400/8 p-3 text-sm text-amber-200">Create a recipient first so Dispatch knows where to deliver notifications.</p>}
        {connect.error&&<p className="rounded-lg bg-red-400/8 p-3 text-sm text-red-300">{connect.error.message}</p>}
        <div className="flex justify-end gap-2"><button type="button" className="btn-secondary" onClick={reset}>Cancel</button><button className="btn" disabled={connect.isPending||!form.recipient_id}>{connect.isPending?'Connecting…':selected.auth==='oauth2'?'Continue to sign in':'Connect'}</button></div>
      </form>
    </div>}

    {unavailable&&<div className="fixed inset-0 z-40 grid place-items-center bg-black/75 p-5 backdrop-blur-sm" onMouseDown={event=>{if(event.target===event.currentTarget)setUnavailable(null)}}>
      <section className="panel w-full max-w-lg">
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-center gap-3"><span className="grid size-12 place-items-center rounded-xl bg-white/5"><BrandLogo service={unavailable.id} className="size-7"/></span><div><p className="text-xs font-semibold uppercase tracking-[.16em] text-amber-300">Temporarily unavailable</p><h2>{unavailable.name}</h2></div></div>
          <button aria-label="Close availability message" className="text-slate-500 hover:text-white" onClick={()=>setUnavailable(null)}><X/></button>
        </div>
        <div className="mt-5 flex gap-3 rounded-xl border border-amber-300/15 bg-amber-300/[.04] p-4"><CircleAlert className="mt-0.5 shrink-0 text-amber-300" size={20}/><p className="text-sm leading-6 text-slate-300">This service has not been enabled by the administrator of this Dispatch installation yet. You do not need to enter tokens or configure anything yourself.</p></div>
        <div className="mt-6 flex justify-end gap-2">{unavailable.documentation&&<a className="btn-secondary" href={unavailable.documentation} target="_blank" rel="noreferrer"><ExternalLink size={15}/>Learn more</a>}<button className="btn" onClick={()=>setUnavailable(null)}>Close</button></div>
      </section>
    </div>}
  </div>
}
