import {
  siDiscord,
  siGithub,
  siGmail,
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
  google: siGmail,
  telegram: siTelegram,
  youtube: siYoutube,
}

export function BrandLogo({service, className = 'size-24'}: {service: string; className?: string}) {
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
