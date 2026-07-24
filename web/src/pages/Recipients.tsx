import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, Recipient } from '../api'
import { Failure, Loading } from '../components/State'

export default function Recipients(){
 const client=useQueryClient(),query=useQuery({queryKey:['recipients'],queryFn:()=>api<{data:Recipient[]}>('/recipients')})
 const [name,setName]=useState(''),[email,setEmail]=useState(''),[webhook,setWebhook]=useState('')
 const create=useMutation({mutationFn:()=>api('/recipients',{method:'POST',body:JSON.stringify({name,email,webhook_url:webhook,preferences:{default_channels:[email?'email':'webhook'],disabled_channels:[],time_zone:'UTC'}})}),onSuccess:()=>{setName('');setEmail('');setWebhook('');client.invalidateQueries({queryKey:['recipients']})}})
 const remove=useMutation({mutationFn:(id:string)=>api(`/recipients/${id}`,{method:'DELETE'}),onSuccess:()=>client.invalidateQueries({queryKey:['recipients']})})
 if(query.isLoading)return <Loading/>;if(query.error)return <Failure error={query.error}/>
 return <div className="space-y-6"><div><h1>Recipients</h1><p className="muted mt-1">Destinations, channel preferences and quiet hours.</p></div><form className="panel grid gap-3 md:grid-cols-4" onSubmit={e=>{e.preventDefault();create.mutate()}}><input className="field" placeholder="Name" value={name} onChange={e=>setName(e.target.value)}/><input className="field" placeholder="Email" value={email} onChange={e=>setEmail(e.target.value)}/><input className="field" placeholder="Webhook URL" value={webhook} onChange={e=>setWebhook(e.target.value)}/><button className="btn">Add recipient</button></form><div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{query.data!.data.map(x=><article className="panel" key={x.id}><div className="flex items-start justify-between"><div><h2>{x.name}</h2><p className="muted mt-1">{x.email||x.webhook_url||x.telegram_chat_id}</p></div><button className="text-xs text-red-300" onClick={()=>remove.mutate(x.id)}>Delete</button></div><div className="mt-5 flex flex-wrap gap-2">{x.preferences.default_channels.map(c=><span className="badge" key={c}>{c}</span>)}</div></article>)}</div></div>
}
