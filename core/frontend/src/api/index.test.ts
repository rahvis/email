import { describe, it, expect, vi } from 'vitest'

// Mock all external dependencies before importing
vi.mock('@/router', () => ({
  default: { push: vi.fn(), replace: vi.fn(), currentRoute: { value: { path: '/' } } },
}))

vi.mock('@/store', () => ({
  useUserStore: vi.fn(() => ({
    login: { token: 'test-token', refresh_token: 'refresh-token' },
    resetLoginInfo: vi.fn(),
    setLoginInfo: vi.fn(),
  })),
  useTenantStore: vi.fn(() => ({
    currentTenantID: 42,
  })),
}))

vi.mock('@/utils', () => ({
  apiUrlPrefix: '/test-prefix',
  isObject: (val: unknown) => val !== null && typeof val === 'object' && !Array.isArray(val),
  Message: {
    error: vi.fn(),
    success: vi.fn(),
    loading: vi.fn(() => ({ close: vi.fn() })),
  },
}))

import { instance, clearPendingRequests } from './index'

describe('API instance', () => {
  it('is an axios instance with expected defaults', () => {
    expect(instance.defaults.timeout).toBe(600000)
    expect(instance.defaults.headers['Content-Type']).toBe('application/json')
  })

  it('has request interceptors registered', () => {
    // Axios stores interceptors internally
    const reqInterceptors = (instance.interceptors.request as any).handlers
    expect(reqInterceptors.length).toBeGreaterThanOrEqual(3)
  })

  it('has response interceptors registered', () => {
    const resInterceptors = (instance.interceptors.response as any).handlers
    expect(resInterceptors.length).toBeGreaterThanOrEqual(1)
  })

  it('does not add auth or tenant headers to public paths after prefixing', async () => {
    const oldAdapter = instance.defaults.adapter
    instance.defaults.adapter = async config => ({
      data: { code: 0, success: true, data: config.headers },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
      request: {},
    })

    const headers = (await instance.get('/login')) as Record<string, string>

    expect(headers.Authorization).toBeUndefined()
    expect(headers['X-Tenant-ID']).toBeUndefined()
    instance.defaults.adapter = oldAdapter
  })

  it('does not add tenant header to tenant-control paths after prefixing', async () => {
    const oldAdapter = instance.defaults.adapter
    instance.defaults.adapter = async config => ({
      data: { code: 0, success: true, data: config.headers },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
      request: {},
    })

    const headers = (await instance.get('/tenants/current')) as Record<string, string>

    expect(headers.Authorization).toBe('Bearer test-token')
    expect(headers['X-Tenant-ID']).toBeUndefined()
    instance.defaults.adapter = oldAdapter
  })

  it('adds tenant header to tenant-scoped API paths after prefixing', async () => {
    const oldAdapter = instance.defaults.adapter
    instance.defaults.adapter = async config => ({
      data: { code: 0, success: true, data: config.headers },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
      request: {},
    })

    const headers = (await instance.get('/kumo/runtime')) as Record<string, string>

    expect(headers.Authorization).toBe('Bearer test-token')
    expect(headers['X-Tenant-ID']).toBe('42')
    instance.defaults.adapter = oldAdapter
  })
})

describe('clearPendingRequests', () => {
  it('is a function', () => {
    expect(typeof clearPendingRequests).toBe('function')
  })

  it('does not throw when called with no pending requests', () => {
    expect(() => clearPendingRequests()).not.toThrow()
  })
})
