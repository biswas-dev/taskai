// Each claim is checked against the llm-brain README and docs (hive, cluster, memory, insights, desktop app).
const features = [
  {
    title: 'One set of skills for every CLI',
    body: 'Store skills, rules, agents and MCP servers once in the hub. The hub installs them for Claude Code, Codex and Antigravity on every machine.',
  },
  {
    title: 'Many hubs, many machines',
    body: 'Each machine runs its own hub, and the hubs copy to each other. Start a session on any machine. Move a Claude Code or Codex session to another machine with its full conversation.',
  },
  {
    title: 'Shared agent memory',
    body: 'Agents save lessons, decisions and gotchas, and read them before they work. The hubs copy memories to each other every minute, and to TaskAI, where you can search them.',
  },
  {
    title: 'Encrypted sync through TaskAI (in progress)',
    body: 'TaskAI has a storage API for encrypted objects. Brain Hub will use it to sync skills, rules and settings between your machines. TaskAI keeps only encrypted bytes and cannot read them.',
  },
  {
    title: 'Activity and insights',
    body: 'See every session from every machine in one place, with full-text search, token use, quota and cost estimates, and a weekday by hour heatmap of your work.',
  },
  {
    title: 'Reviews on every change',
    body: 'The other CLIs and an API reviewer (OpenAI and Jev) review each change, read-only. The fleet turns the issues assigned to you into reviewed PRs.',
  },
  {
    title: 'Tasks in TaskAI',
    body: 'Agents read and update TaskAI tasks, comments and wiki pages through the TaskAI MCP server. The activity log of each project shows what each agent did.',
  },
  {
    title: 'Desktop apps',
    body: 'A native macOS app, and apps for Windows and Linux: live terminals, a hub switcher, quick search and notifications.',
  },
]

export default function BrainHubFeatures() {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      {features.map(f => (
        <div key={f.title} className="p-5 rounded-xl bg-dark-bg-primary border border-dark-border-subtle">
          <h3 className="font-semibold">{f.title}</h3>
          <p className="mt-2 text-sm text-dark-text-tertiary leading-relaxed">{f.body}</p>
        </div>
      ))}
    </div>
  )
}
