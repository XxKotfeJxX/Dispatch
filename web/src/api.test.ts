import { beforeEach, describe, expect, it } from 'vitest'
import { apiKey, setApiKey } from './api'

describe('apiKey', () => {
  beforeEach(() => setApiKey(''))

  it('does not send a console credential by default', () => {
    expect(apiKey()).toBe('')
  })
  it('keeps a configured key only in process memory', () => {
    setApiKey('configured-key')
    expect(apiKey()).toBe('configured-key')
    expect(localStorage.getItem('dispatch_api_key')).toBeNull()
  })
})
