import { afterEach, expect, test, vi } from 'vitest'
import { APIClient } from './client'
afterEach(() => vi.unstubAllGlobals())
test('a failed production API never substitutes preview ships', async () => {
 vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('service offline')))
 await expect(new APIClient().listShips()).rejects.toThrow('service offline')
})
test('structured operation errors reach the caller', async () => {
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({error:{code:'recovery_hold',message:'Uncertain operation; pier held.'}}),{status:409})))
 await expect(new APIClient().action('test','start')).rejects.toThrow('Uncertain operation; pier held.')
})
