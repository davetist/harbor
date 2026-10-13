import { afterEach, expect, test, vi } from 'vitest'
import { APIClient, APIError } from './client'
afterEach(() => vi.unstubAllGlobals())
test('a failed production API never substitutes preview ships', async () => {
 vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('service offline')))
 await expect(new APIClient().listShips()).rejects.toThrow('service offline')
})
test('structured operation errors reach the caller', async () => {
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({error:{code:'recovery_hold',message:'Uncertain operation; pier held.'}}),{status:409})))
 await expect(new APIClient().action('test','start')).rejects.toThrow('Uncertain operation; pier held.')
})
test('Open keeps the error code so the UI can offer manual recovery', async () => {
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({error:{code:'ship_unresponsive',message:'The ship may be busy.'}}),{status:503})))
 const error = await new APIClient().openURL('test').catch(error => error)
 expect(error).toMatchObject({code:'ship_unresponsive',message:'The ship may be busy.'})
 expect(error).toBeInstanceOf(APIError)
})
