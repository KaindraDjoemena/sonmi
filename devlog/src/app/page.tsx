import { getJournals } from '@/lib/api'
import { JournalView } from '@/components/JournalView'

export default async function Home() {
  const journals = await getJournals()
  return <JournalView journals={journals} />
}
