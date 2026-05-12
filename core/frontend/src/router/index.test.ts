import { describe, expect, it } from 'vitest'

import { whitePathList } from './public'

describe('router guard public paths', () => {
	it('allows landing, login, and signup without a session', () => {
		expect(whitePathList).toEqual(expect.arrayContaining(['/', '/login', '/signup']))
	})
})
