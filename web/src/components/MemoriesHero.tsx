const PROTOCOL_URL = 'https://github.com/biswas-dev/taskai/blob/main/docs/memory-protocol.md'

const points = [
  { title: 'Write once, recall anywhere', text: 'A memory is a short lesson: a fact, a decision, a gotcha, a runbook or a handoff. Save it from one tool, find it from every other.' },
  { title: 'One store per person', text: 'Your memories belong to you, not to a project. Claude, Codex, Gemini, your own apps and your teammates you share with all read the same store.' },
  { title: 'Fast, bounded, tidy', text: 'Hybrid search answers in milliseconds. A quota keeps it bounded, and compaction drops what was replaced or expired without losing a lesson.' },
]

// Each line is something the code does today; keep it that way.
const connections = [
  { title: 'Tasks, issues and PRs', text: 'Scope a memory to a repo, issue or PR. An agent picking up that work searches with the same filters and gets its lessons first.' },
  { title: 'GitHub sync', text: 'Scope uses the owner/repo names and GitHub issue and PR numbers that GitHub sync imports your tasks from.' },
  { title: 'Wiki and knowledge graph', text: 'Long docs and linked tasks stay in the wiki and graph; memories keep the short lessons. Search runs on the same full-text and vector engine as the wiki, and old "Agent memory" wiki pages import in one call.' },
  { title: 'One MCP server, one API key', text: 'The MCP server and API keys that drive your tasks and wiki also write and search memories.' },
  { title: 'Teams and projects', text: 'Memories are private by default. Share them, read-only, with people on your teams and projects.' },
]

// A landing page section (below the hero): TaskAI Memories, an open protocol any app or LLM can use.
export default function MemoriesHero() {
  return (
    <section id="memories" aria-labelledby="memories-heading" className="py-16 md:py-24 border-t border-dark-border-subtle">
      <div className="max-w-5xl mx-auto px-6">
        <div className="max-w-2xl">
          <h2 id="memories-heading" className="text-3xl md:text-4xl font-bold tracking-tight">
            One memory for every AI you use
          </h2>
          <p className="mt-5 text-base md:text-lg text-dark-text-tertiary leading-relaxed">
            Your agents keep relearning the same lessons. TaskAI Memories gives them, and you, one place to keep them.
            Write a lesson once over REST or MCP. Every model and app you use can then find it.
          </p>
          <p className="mt-4 text-sm">
            <a href={PROTOCOL_URL} className="font-medium text-primary-400 hover:text-primary-300 transition-colors">
              Read the memory protocol →
            </a>
          </p>
        </div>

        <ul className="mt-12 grid gap-8 md:grid-cols-3">
          {points.map(p => (
            <li key={p.title}>
              <h3 className="text-base font-semibold text-dark-text-primary">{p.title}</h3>
              <p className="mt-2 text-sm text-dark-text-tertiary leading-relaxed">{p.text}</p>
            </li>
          ))}
        </ul>

        <section aria-labelledby="memories-connect-heading" className="mt-14 pt-10 border-t border-dark-border-subtle">
          <h3 id="memories-connect-heading" className="text-lg font-semibold text-dark-text-primary">
            Works with everything in TaskAI
          </h3>
          <ul className="mt-6 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {connections.map(c => (
              <li key={c.title}>
                <h4 className="text-sm font-semibold text-dark-text-primary">{c.title}</h4>
                <p className="mt-1.5 text-sm text-dark-text-tertiary leading-relaxed">{c.text}</p>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </section>
  )
}
