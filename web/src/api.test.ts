import { describe, expect, it } from 'vitest'
import { apiKey, setApiKey } from './api'

describe('apiKey', () => {
  it('uses a local development default', () => {
    expect(apiKey()).toBe('dispatch-local-development-key')
  })
  it('keeps a configured key only in process memory', () => {
    setApiKey('configured-key')
    expect(apiKey()).toBe('configured-key')
    expect(localStorage.getItem('dispatch_api_key')).toBeNull()
  })
})
