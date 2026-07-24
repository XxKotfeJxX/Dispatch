import { beforeEach, describe, expect, it } from 'vitest'
import { apiKey } from './api'

describe('apiKey', () => {
  beforeEach(() => localStorage.clear())
  it('uses a local development default', () => {
    expect(apiKey()).toBe('dispatch-local-development-key')
  })
  it('uses the configured console key', () => {
    localStorage.setItem('dispatch_api_key', 'configured-key')
    expect(apiKey()).toBe('configured-key')
  })
})
