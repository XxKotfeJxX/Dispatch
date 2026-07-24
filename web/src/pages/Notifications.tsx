import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Search } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { Link } from 'react-router-dom'
import { z } from 'zod'
import { api, Notification, Recipient } from '../api'
import { Failure, Loading, Status } from '../components/State'

const schema=z.object({recipient_id:z.string().min(1),event_type:z.string().min(2),subject:z.string().max(200),body:z.string().min(1).max(100000),channels:z.string()})
type Form=z.infer<typeof schema>
export default function Notifications(){
 const client=useQueryClient(), [compose,setCompose]=useState(false), [search,setSearch]=useState('')
 const list=useQuery({queryKey:['notifications'],queryFn:()=>api<{data:Notification[];total:number}>('/notifications?limit=100')})
 const people=useQuery({queryKey:['recipients'],queryFn:()=>api<{data:Recipient[]}>('/recipients')})
 const form=useForm<Form>({resolver:zodResolver(schema),defaultValues:{event_type:'system.notice',subject:'',body:'',channels:''}})
 const create=useMutation({mutationFn:(v:Form)=>api('/notifications',{method:'POST',body:JSON.stringify({idempotency_key:crypto.randomUUID(),recipient_id:v.recipient_id,event_type:v.event_type,subject:v.subject,body:v.body,requested_channels:v.channels? v.channels.split(',').map(x=>x.trim()).filter(Boolean):[],metadata:{environment:'production'}})}),onSuccess:()=>{client.invalidateQueries({queryKey:['notifications']});setCompose(false);form.reset()}})
 if(list.isLoading||people.isLoading)return <Loading/>;if(list.error)return <Failure error={list.error}/>
 const rows=list.data!.data.filter(x=>(x.subject+x.event_type+x.id).toLowerCase().includes(search.toLowerCase()))
 return <div className="space-y-6"><div className="flex flex-wrap items-end justify-between gap-4"><div><h1>Notifications</h1><p className="muted mt-1">{list.data!.total} orchestration records</p></div><button className="btn" onClick={()=>setCompose(!compose)}><Plus size={16}/>Compose</button></div>
 {compose&&<form className="panel grid gap-4 md:grid-cols-2" onSubmit={form.handleSubmit(v=>create.mutate(v))}><select className="field" {...form.register('recipient_id')}><option value="">Select recipient</option>{people.data!.data.map(x=><option key={x.id} value={x.id}>{x.name}</option>)}</select><input className="field" placeholder="Event type" {...form.register('event_type')}/><input className="field md:col-span-2" placeholder="Subject" {...form.register('subject')}/><textarea className="field min-h-28 md:col-span-2" placeholder="Message body" {...form.register('body')}/><input className="field" placeholder="Optional channels: email,telegram" {...form.register('channels')}/><button className="btn" disabled={create.isPending}>Queue notification</button>{create.error&&<p className="text-red-300">{create.error.message}</p>}</form>}
 <div className="panel"><div className="relative mb-4"><Search className="absolute left-3 top-3 text-slate-500" size={16}/><input className="field pl-10" placeholder="Search notifications…" value={search} onChange={e=>setSearch(e.target.value)}/></div><div className="table-wrap"><table><thead><tr><th>Notification</th><th>Event</th><th>Priority</th><th>Status</th><th>Created</th></tr></thead><tbody>{rows.map(x=><tr key={x.id}><td><Link className="font-medium text-cyan-300 hover:underline" to={`/notifications/${x.id}`}>{x.subject||'Untitled'}</Link><div className="mt-1 font-mono text-xs text-slate-600">{x.id.slice(0,20)}</div></td><td>{x.event_type}</td><td>{x.priority}</td><td><Status value={x.status}/></td><td className="text-slate-400">{new Date(x.created_at).toLocaleString()}</td></tr>)}</tbody></table></div></div></div>
}
