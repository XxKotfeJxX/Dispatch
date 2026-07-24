import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '../api'
import { Failure, Loading } from '../components/State'
type Rule={id:string;name:string;priority:number;condition:Record<string,unknown>;action:Record<string,unknown>;enabled:boolean}
export default function Rules(){
 const client=useQueryClient(),query=useQuery({queryKey:['rules'],queryFn:()=>api<{data:Rule[]}>('/rules')}),[event,setEvent]=useState(''),[channels,setChannels]=useState('email')
 const create=useMutation({mutationFn:()=>api('/rules',{method:'POST',body:JSON.stringify({name:`Route ${event}`,priority:100,condition:{event_type:event},action:{channels:channels.split(',').map(x=>x.trim())},enabled:true})}),onSuccess:()=>{setEvent('');client.invalidateQueries({queryKey:['rules']})}})
 const remove=useMutation({mutationFn:(id:string)=>api(`/rules/${id}`,{method:'DELETE'}),onSuccess:()=>client.invalidateQueries({queryKey:['rules']})})
 if(query.isLoading)return <Loading/>;if(query.error)return <Failure error={query.error}/>
 return <div className="space-y-6"><div><h1>Routing rules</h1><p className="muted mt-1">Deterministic policy always runs before advisory AI.</p></div><form className="panel grid gap-3 md:grid-cols-3" onSubmit={e=>{e.preventDefault();create.mutate()}}><input className="field" placeholder="Event type, e.g. payment.failed" value={event} onChange={e=>setEvent(e.target.value)}/><input className="field" placeholder="email,telegram" value={channels} onChange={e=>setChannels(e.target.value)}/><button className="btn">Create rule</button></form><div className="panel table-wrap"><table><thead><tr><th>Rule</th><th>Condition</th><th>Channels</th><th></th></tr></thead><tbody>{query.data!.data.map(x=><tr key={x.id}><td>{x.name}</td><td className="font-mono text-xs">{JSON.stringify(x.condition)}</td><td>{String(x.action.channels)}</td><td><button className="text-red-300" onClick={()=>remove.mutate(x.id)}>Delete</button></td></tr>)}</tbody></table></div></div>
}
