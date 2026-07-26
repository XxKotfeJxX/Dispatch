import { useQuery } from '@tanstack/react-query'
import { Search } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, Notification } from '../api'
import { Failure, Loading, Status } from '../components/State'

export default function Notifications() {
  const [search, setSearch] = useState('')
  const list = useQuery({
    queryKey: ['notifications'],
    queryFn: () => api<{data: Notification[]; total: number}>('/notifications?limit=100'),
  })

  if (list.isLoading) return <Loading/>
  if (list.error) return <Failure error={list.error}/>

  const normalizedSearch = search.toLowerCase()
  const rows = list.data!.data.filter(item =>
    (item.subject + item.event_type + item.id).toLowerCase().includes(normalizedSearch))

  return (
    <div className="space-y-6">
      <div>
        <h1>Notifications</h1>
        <p className="muted mt-1">{list.data!.total} orchestration records</p>
      </div>

      <div className="panel">
        <div className="relative mb-4">
          <Search
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-500"
            size={16}
          />
          <input
            className="field"
            onChange={event => setSearch(event.target.value)}
            placeholder="Search notifications…"
            style={{paddingLeft: '2.5rem'}}
            value={search}
          />
        </div>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Notification</th>
                <th>Event</th>
                <th>Priority</th>
                <th>Status</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(item => (
                <tr key={item.id}>
                  <td>
                    <Link
                      className="font-medium text-cyan-300 hover:underline"
                      to={`/notifications/${item.id}`}
                    >
                      {item.subject || 'Untitled'}
                    </Link>
                    <div className="mt-1 font-mono text-xs text-slate-600">
                      {item.id.slice(0, 20)}
                    </div>
                  </td>
                  <td>{item.event_type}</td>
                  <td>{item.priority}</td>
                  <td><Status value={item.status}/></td>
                  <td className="text-slate-400">{new Date(item.created_at).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
