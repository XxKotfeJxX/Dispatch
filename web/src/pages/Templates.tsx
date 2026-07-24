import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '../api'
import { Failure, Loading } from '../components/State'
type Template={id:string;name:string;channel:string;subject_template:string;body_template:string}
export default function Templates(){
 const client=useQueryClient(),query=useQuery({queryKey:['templates'],queryFn:()=>api<{data:Template[]}>('/templates')}),[name,setName]=useState(''),[body,setBody]=useState('{{body}}')
 const create=useMutation({mutationFn:()=>api('/templates',{method:'POST',body:JSON.stringify({name,channel:'email',subject_template:'{{subject}}',body_template:body})}),onSuccess:()=>{setName('');client.invalidateQueries({queryKey:['templates']})}})
 if(query.isLoading)return <Loading/>;if(query.error)return <Failure error={query.error}/>
 return <div className="space-y-6"><div><h1>Templates</h1><p className="muted mt-1">Reusable channel-specific message formats.</p></div><form className="panel grid gap-3 md:grid-cols-[1fr_2fr_auto]" onSubmit={e=>{e.preventDefault();create.mutate()}}><input className="field" placeholder="Template name" value={name} onChange={e=>setName(e.target.value)}/><input className="field font-mono" value={body} onChange={e=>setBody(e.target.value)}/><button className="btn">Save template</button></form><div className="grid gap-4 md:grid-cols-2">{query.data!.data.map(x=><article className="panel" key={x.id}><div className="flex justify-between"><h2>{x.name}</h2><span className="badge">{x.channel}</span></div><pre className="mt-5 overflow-auto rounded-lg bg-black/20 p-4 text-xs text-slate-400">{x.body_template}</pre></article>)}</div></div>
}
