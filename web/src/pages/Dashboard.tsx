import { useQuery } from '@tanstack/react-query'
import { Activity, CheckCircle2, Clock3, OctagonAlert } from 'lucide-react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../api'
import { Failure, Loading } from '../components/State'

type Data = { notifications_24h:number; delivered:number; failed:number; waiting_retry:number; dead_letter:number; ai_fallback_rate:number; deliveries_by_channel:Record<string,number>; recent_failures:Array<{notification_id:string;subject:string;channel:string;error:string}> }
export default function Dashboard() {
  const query = useQuery({ queryKey:['dashboard'], queryFn:()=>api<{data:Data}>('/dashboard') })
  if(query.isLoading) return <Loading/>; if(query.error) return <Failure error={query.error}/>
  const value=query.data!.data
  const cards=[['Last 24 hours',value.notifications_24h,Activity],['Delivered',value.delivered,CheckCircle2],['Waiting retry',value.waiting_retry,Clock3],['Dead letter',value.dead_letter,OctagonAlert]] as const
  const chart=Object.entries(value.deliveries_by_channel).map(([channel,count])=>({channel,count}))
  return <div className="space-y-6"><div><p className="mb-1 text-xs font-semibold uppercase tracking-[.2em] text-cyan-400">Control plane</p><h1>Delivery overview</h1><p className="muted mt-1">Live health across every notification channel.</p></div>
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{cards.map(([label,count,Icon])=><div className="panel" key={label}><div className="mb-5 flex justify-between"><span className="muted">{label}</span><Icon size={18} className="text-cyan-300"/></div><strong className="text-3xl">{count}</strong></div>)}</div>
    <div className="grid gap-5 lg:grid-cols-[1.5fr_1fr]"><section className="panel"><div className="mb-5 flex justify-between"><h2>Deliveries by channel</h2><span className="badge">all time</span></div><div className="h-64"><ResponsiveContainer width="100%" height="100%"><BarChart data={chart}><CartesianGrid stroke="#ffffff0b" vertical={false}/><XAxis dataKey="channel" stroke="#64748b"/><YAxis stroke="#64748b"/><Tooltip contentStyle={{background:'#0b111c',border:'1px solid #ffffff14'}}/><Bar dataKey="count" fill="#22d3ee" radius={[6,6,0,0]}/></BarChart></ResponsiveContainer></div></section>
      <section className="panel"><h2>Routing intelligence</h2><p className="muted mt-1">Deterministic fallback remains authoritative.</p><div className="mt-8 text-5xl font-semibold">{Math.round(value.ai_fallback_rate*100)}%</div><p className="mt-2 text-sm text-slate-400">AI fallback rate</p><div className="mt-8 h-2 overflow-hidden rounded-full bg-white/5"><div className="h-full bg-cyan-400" style={{width:`${value.ai_fallback_rate*100}%`}}/></div></section></div>
    <section className="panel"><h2 className="mb-4">Recent failures</h2>{value.recent_failures.length===0?<p className="muted">No delivery failures.</p>:<div className="table-wrap"><table><thead><tr><th>Subject</th><th>Channel</th><th>Error</th></tr></thead><tbody>{value.recent_failures.map(x=><tr key={x.notification_id+x.channel}><td>{x.subject}</td><td>{x.channel}</td><td className="text-red-300">{x.error}</td></tr>)}</tbody></table></div>}</section>
  </div>
}
