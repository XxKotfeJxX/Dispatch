import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api, apiKey } from '../api'
import { Failure, Loading } from '../components/State'
type Settings={ai:{enabled:boolean;model:string;prompt_version:string};channels:Record<string,boolean>;worker_concurrency:number}
export default function SettingsPage(){
 const query=useQuery({queryKey:['settings'],queryFn:()=>api<{data:Settings}>('/settings')}),[key,setKey]=useState(apiKey())
 if(query.isLoading)return <Loading/>;if(query.error)return <Failure error={query.error}/>
 const value=query.data!.data
 return <div className="space-y-6"><div><h1>Settings</h1><p className="muted mt-1">Runtime state only. Secrets are never returned by the API.</p></div><section className="panel"><h2>Console authentication</h2><div className="mt-4 flex gap-3"><input className="field" type="password" value={key} onChange={e=>setKey(e.target.value)}/><button className="btn" onClick={()=>{localStorage.setItem('dispatch_api_key',key);location.reload()}}>Save key</button></div></section><section className="panel"><h2>Provider readiness</h2><div className="mt-5 grid gap-3 sm:grid-cols-3">{Object.entries(value.channels).map(([name,ready])=><div className="rounded-xl border border-white/8 p-4" key={name}><div className="flex items-center justify-between"><b className="capitalize">{name}</b><span className={`size-2 rounded-full ${ready?'bg-emerald-400':'bg-slate-600'}`}/></div><p className="muted mt-2">{ready?'Configured':'Unavailable'}</p></div>)}</div></section><section className="panel"><h2>AI advisory routing</h2><dl className="mt-4 grid gap-4 sm:grid-cols-3"><div><dt className="muted">Status</dt><dd>{value.ai.enabled?'Enabled':'Fallback only'}</dd></div><div><dt className="muted">Model</dt><dd>{value.ai.model}</dd></div><div><dt className="muted">Prompt</dt><dd>{value.ai.prompt_version}</dd></div></dl></section></div>
}
