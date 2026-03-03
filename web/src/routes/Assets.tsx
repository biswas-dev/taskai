import { useParams } from 'react-router-dom'
import MyAssets from '../components/MyAssets'

export default function Assets() {
  const { projectId } = useParams<{ projectId: string }>()
  const projectIdNum = Number(projectId)

  if (!projectIdNum) return null

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <MyAssets projectId={projectIdNum} />
      </div>
    </div>
  )
}
