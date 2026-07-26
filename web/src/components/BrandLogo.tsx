import {
  siDiscord,
  siGithub,
  siTelegram,
  siYoutube,
} from 'simple-icons'

type BrandIcon = {
  path: string
  hex: string
  title: string
}

const brands: Record<string, BrandIcon> = {
  discord: siDiscord,
  github: siGithub,
  telegram: siTelegram,
  youtube: siYoutube,
}

export function BrandLogo({service, className = 'size-24'}: {service: string; className?: string}) {
  if (service === 'google') {
    return (
      <svg aria-hidden="true" className={className} role="img" viewBox="0 0 48 48">
        <path fill="#FFC107" d="M43.6 20.5H42V20H24v8h11.3A12 12 0 0 1 12 24c0-1.9.4-3.7 1.2-5.3l-6.5-5A20 20 0 0 0 4 24c0 3.7 1 7.2 2.7 10.3l6.5-5A12 12 0 0 1 12 24Z"/>
        <path fill="#FF3D00" d="M24 4A20 20 0 0 0 6.7 13.7l6.5 5A12 12 0 0 1 32 15.1l5.7-5.7A19.9 19.9 0 0 0 24 4Z"/>
        <path fill="#4CAF50" d="M24 44c5.2 0 10-2 13.6-5.2l-6.2-5.3A11.9 11.9 0 0 1 13.2 29l-6.5 5A20 20 0 0 0 24 44Z"/>
        <path fill="#1976D2" d="M43.6 20.5H42V20H24v8h11.3a12 12 0 0 1-3.9 5.5l6.2 5.3C41.2 35.5 44 30.7 44 24c0-1.2-.1-2.4-.4-3.5Z"/>
      </svg>
    )
  }
  const brand = brands[service]
  if (!brand) return null
  const fill = service === 'github' ? '#ffffff' : `#${brand.hex}`
  return (
    <svg
      aria-hidden="true"
      className={className}
      fill={fill}
      role="img"
      viewBox="0 0 24 24"
    >
      <path d={brand.path}/>
    </svg>
  )
}
