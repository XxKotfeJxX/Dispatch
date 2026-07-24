export function Loading() { return <div className="panel animate-pulse text-slate-500">Loading current state…</div> }
export function Failure({error}:{error: Error}) { return <div className="panel border-red-500/20 text-red-300">{error.message}</div> }
export function Status({value}:{value:string}) {
  const color = value === 'delivered' ? 'text-emerald-300 bg-emerald-500/10' : value.includes('fail') || value === 'dead_letter' ? 'text-red-300 bg-red-500/10' : 'text-amber-300 bg-amber-500/10'
  return <span className={`rounded-full px-2 py-1 text-xs ${color}`}>{value.replaceAll('_',' ')}</span>
}
