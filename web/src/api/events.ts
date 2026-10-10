export function subscribeEvents(refresh: () => void): () => void {
  const stream = new EventSource('/api/v1/events')
  stream.onerror = refresh
  for (const kind of ['resync', 'ship', 'job', 'schedule']) stream.addEventListener(kind, refresh)
  return () => stream.close()
}
