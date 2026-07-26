import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, RotateCcw, XCircle } from 'lucide-react'
import { Link, useParams } from 'react-router-dom'
import { api, Notification } from '../api'
import { Failure, Loading, Status } from '../components/State'

type Detail={data:Notification;deliveries:Array<Record<string,unknown>>;attempts:Array<Record<string,unknown>>;ai_decisions:Array<Record<string,unknown>>;events:Array<{id:number;event_type:string;created_at:string;details:Record<string,unknown>}>}
export default function NotificationDetail(){
 const {id}=useParams(),client=useQueryClient(),query=useQuery({queryKey:['notification',id],queryFn:()=>api<Detail>(`/notifications/${id}`)})
 const action=useMutation({mutationFn:(name:string)=>api(`/notifications/${id}/${name}`,{method:'POST'}),onSuccess:()=>client.invalidateQueries({queryKey:['notification',id]})})
 if(query.isLoading)return <Loading/>;if(query.error)return <Failure error={query.error}/>
 const x=query.data!
 return <div className="space-y-6"><Link to="/notifications" className="inline-flex items-center gap-2 text-sm text-slate-400 hover:text-white"><ArrowLeft size={15}/>Notifications</Link><div className="flex flex-wrap items-start justify-between gap-4"><div><div className="mb-3 flex items-center gap-3"><Status value={x.data.status}/><span className="badge">{x.data.priority}</span></div><h1>{x.data.subject}</h1><p className="mt-2 font-mono text-xs text-slate-600">{x.data.id}</p></div><div className="flex gap-2"><button className="btn-secondary" onClick={()=>action.mutate('retry')}><RotateCcw size={15}/>Retry</button><button className="btn-secondary text-red-300" onClick={()=>action.mutate('cancel')}><XCircle size={15}/>Cancel</button></div></div>
 <div className="grid gap-5 lg:grid-cols-[1.2fr_.8fr]"><section className="panel"><h2>Payload</h2><div className="mt-5 rounded-xl bg-black/20 p-4 whitespace-pre-wrap text-sm leading-7 text-slate-300">{x.data.body}</div></section><section className="panel"><h2>Analysis</h2><dl className="mt-5 space-y-4 text-sm"><div><dt className="text-slate-500">Event</dt><dd>{x.data.event_type}</dd></div><div><dt className="text-slate-500">Category</dt><dd>{x.data.category||'Pending'}</dd></div><div><dt className="text-slate-500">Priority</dt><dd>{x.data.priority}</dd></div><div><dt className="text-slate-500">Summary</dt><dd>{x.data.summary||'Pending'}</dd></div></dl></section></div>
 <section className="panel"><h2 className="mb-4">Deliveries</h2><pre className="overflow-auto text-xs text-slate-400">{JSON.stringify(x.deliveries,null,2)}</pre></section>
 <section className="panel"><h2 className="mb-4">Decision & audit trail</h2><div className="space-y-4">{x.events.map(e=><div key={e.id} className="flex gap-4 border-l border-cyan-400/30 pl-4"><span className="size-2 -translate-x-[21px] translate-y-1.5 rounded-full bg-cyan-400"/><div><b className="text-sm">{e.event_type}</b><p className="mt-1 text-xs text-slate-500">{new Date(e.created_at).toLocaleString()}</p></div></div>)}</div></section></div>
}
