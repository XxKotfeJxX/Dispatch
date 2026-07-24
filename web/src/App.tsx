import { NavLink, Route, Routes } from 'react-router-dom'
import { Activity, Bell, Bot, LayoutDashboard, Settings, Users, Waypoints } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { setApiKey } from './api'
import Dashboard from './pages/Dashboard'
import Notifications from './pages/Notifications'
import NotificationDetail from './pages/NotificationDetail'
import Recipients from './pages/Recipients'
import Rules from './pages/Rules'
import Templates from './pages/Templates'
import SettingsPage from './pages/Settings'

const navigation = [
  ['/', 'Overview', LayoutDashboard], ['/notifications', 'Notifications', Bell],
  ['/recipients', 'Recipients', Users], ['/rules', 'Routing rules', Waypoints],
  ['/templates', 'Templates', Activity], ['/settings', 'Settings', Settings],
] as const

export default function App() {
  const [authRequired,setAuthRequired]=useState(false),[key,setKey]=useState(''),client=useQueryClient()
  useEffect(()=>{const show=()=>setAuthRequired(true);window.addEventListener('dispatch:unauthorized',show);return()=>window.removeEventListener('dispatch:unauthorized',show)},[])
  return <div className="min-h-screen bg-[#070b12] text-slate-100">
    <aside className="fixed inset-y-0 left-0 z-20 hidden w-64 border-r border-white/8 bg-[#0a0f18]/95 p-5 md:block">
      <div className="mb-9 flex items-center gap-3"><span className="grid size-10 place-items-center rounded-xl bg-cyan-400 text-slate-950"><Bot size={22}/></span><div><b className="text-lg">Dispatch</b><p className="text-xs text-slate-500">Operations console</p></div></div>
      <nav className="space-y-1">{navigation.map(([to, label, Icon]) =>
        <NavLink key={to} to={to} end={to === '/'} className={({isActive}) => `flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm ${isActive ? 'bg-cyan-400/12 text-cyan-300' : 'text-slate-400 hover:bg-white/5 hover:text-white'}`}><Icon size={17}/>{label}</NavLink>
      )}</nav>
      <div className="absolute bottom-5 left-5 right-5 rounded-xl border border-emerald-500/15 bg-emerald-500/5 p-3 text-xs text-emerald-300"><span className="mr-2 inline-block size-2 rounded-full bg-emerald-400"/>Self-hosted · online</div>
    </aside>
    <main className="md:ml-64"><header className="sticky top-0 z-10 flex h-16 items-center justify-between border-b border-white/8 bg-[#070b12]/85 px-5 backdrop-blur md:px-8"><span className="text-sm text-slate-500">Notification infrastructure</span><span className="badge">single tenant</span></header><div className="mx-auto max-w-7xl p-5 md:p-8"><Routes>
      <Route path="/" element={<Dashboard/>}/><Route path="/notifications" element={<Notifications/>}/><Route path="/notifications/:id" element={<NotificationDetail/>}/>
      <Route path="/recipients" element={<Recipients/>}/><Route path="/rules" element={<Rules/>}/><Route path="/templates" element={<Templates/>}/><Route path="/settings" element={<SettingsPage/>}/>
    </Routes></div></main>
    {authRequired&&<div className="fixed inset-0 z-50 grid place-items-center bg-black/75 p-5 backdrop-blur-sm"><form className="panel w-full max-w-md" onSubmit={e=>{e.preventDefault();setApiKey(key);setKey('');setAuthRequired(false);client.invalidateQueries()}}><h2>Connect to Dispatch</h2><p className="muted mt-2">Enter the API key for this server. It stays only in this tab's memory.</p><input autoFocus autoComplete="off" type="password" className="field mt-5" placeholder="API key" value={key} onChange={e=>setKey(e.target.value)}/><button className="btn mt-3 w-full" disabled={!key.trim()}>Connect</button></form></div>}
  </div>
}
