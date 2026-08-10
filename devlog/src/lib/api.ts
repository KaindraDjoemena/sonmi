export type JournalEntry = {
  id: number
  day_recap: string
  plan_for_tomorrow: string
  agent_musings: string
  is_stale: boolean
  valid_for_date: string
  img_url: string
  time: string
}

export async function getJournals(): Promise<JournalEntry[]> {
  const url = process.env.SONMI_API_URL
  const key = process.env.SONMI_API_KEY

  if (!url || !key) {
    // Local dev without env vars — return empty array gracefully
    return []
  }

  try {
    const res = await fetch(`${url}/api/journals`, {
      headers: { 'Authentication-Key': key },
      next: { revalidate: 86400, tags: ['journals'] },
    })
    if (!res.ok) return []
    return res.json()
  } catch {
    return []
  }
}
