import { Link } from 'react-router-dom'

const PROTOCOL_URL = 'https://github.com/biswas-dev/taskai/blob/main/docs/memory-protocol.md'

const CURL = `# Save a lesson (safe to retry: origin_host + origin_id make it idempotent)
curl https://taskai.cc/api/memories \\
  -H "Authorization: ApiKey $TASKAI_KEY" -H "Content-Type: application/json" \\
  -d '{"kind":"gotcha","title":"Partial index needs WHERE",
       "body":"ON CONFLICT on a partial unique index must repeat its WHERE clause.",
       "scope":{"repo":"acme/api"},"origin_host":"my-app","origin_id":"42"}'

# Find it again, from any app or model
curl https://taskai.cc/api/memories/search \\
  -H "Authorization: ApiKey $TASKAI_KEY" -H "Content-Type: application/json" \\
  -d '{"query":"on conflict partial index","repo":"acme/api"}'`

const MCP = `claude mcp add --transport http taskai https://mcp.taskai.cc/mcp \\
  --header "X-API-Key: $TASKAI_KEY"

# Then ask: "search my memories for the deploy gotchas"
# Tools: search_memories, write_memory, supersede_memory, ...`

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

function CodeBlock({ label, code }: { label: string; code: string }) {
  return (
    <figure className="rounded-xl border border-dark-border-subtle bg-dark-bg-primary overflow-hidden text-left">
      <figcaption className="px-4 py-2 text-xs font-medium text-dark-text-tertiary bg-dark-bg-secondary border-b border-dark-border-subtle">
        {label}
      </figcaption>
      <pre className="p-4 text-xs leading-relaxed text-dark-text-secondary overflow-x-auto"><code>{code}</code></pre>
    </figure>
  )
}

// The landing page's lead section: TaskAI Memories, an open protocol any app or LLM can use.
export default function MemoriesHero() {
  return (
    <section id="memories" aria-labelledby="memories-heading" className="relative pt-36 pb-20 md:pt-44 md:pb-28 overflow-hidden">
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute top-1/3 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[900px] h-[600px] bg-primary-500/[0.07] rounded-full blur-[140px]" />
      </div>

      <div className="relative max-w-5xl mx-auto px-6">
        <div className="text-center">
          <div className="inline-flex items-center gap-2 px-3 py-1.5 mb-8 text-xs font-medium text-primary-400 bg-primary-500/10 border border-primary-500/20 rounded-full">
            <span className="w-1.5 h-1.5 rounded-full bg-primary-400 animate-pulse" />
            New · TaskAI Memories, an open protocol for any app or LLM
          </div>
          <h1 id="memories-heading" className="text-5xl md:text-7xl font-bold tracking-tight leading-[1.05]">
            One memory{' '}
            <br />
            <span className="bg-gradient-to-r from-primary-400 via-primary-300 to-secondary-400 bg-clip-text text-transparent">
              for every AI you use
            </span>
          </h1>
          <p className="mt-7 text-lg md:text-xl text-dark-text-tertiary max-w-2xl mx-auto leading-relaxed">
            Your agents keep relearning the same lessons. TaskAI Memories gives them, and you, one store to remember in:
            write a lesson once over REST or MCP, and every model and app you use can find it.
          </p>
          <div className="mt-10 flex flex-col sm:flex-row items-center justify-center gap-4">
            <Link
              to="/signup"
              className="inline-flex items-center px-8 py-3.5 text-base font-medium text-white bg-primary-500 hover:bg-primary-600 rounded-md shadow-linear transition-all duration-150 w-full sm:w-auto justify-center"
            >
              Get your memory store
            </Link>
            <a
              href={PROTOCOL_URL}
              className="inline-flex items-center px-6 py-3.5 text-base font-medium text-dark-text-secondary border border-dark-border-medium hover:border-dark-border-strong hover:bg-dark-bg-secondary rounded-md transition-all duration-150 w-full sm:w-auto justify-center"
            >
              Read the protocol
            </a>
          </div>
        </div>

        <ul className="mt-16 grid gap-6 md:grid-cols-3">
          {points.map(p => (
            <li key={p.title} className="rounded-xl border border-dark-border-subtle bg-dark-bg-primary/60 p-5">
              <h2 className="text-sm font-semibold text-dark-text-primary">{p.title}</h2>
              <p className="mt-2 text-sm text-dark-text-tertiary leading-relaxed">{p.text}</p>
            </li>
          ))}
        </ul>

        <div className="mt-12">
          <h2 className="text-center text-xs font-medium uppercase tracking-wider text-dark-text-quaternary mb-4">Quick start</h2>
          <div className="grid gap-4 md:grid-cols-2">
            <CodeBlock label="REST (curl)" code={CURL} />
            <CodeBlock label="MCP (Claude Code; any MCP client works)" code={MCP} />
          </div>
          <p className="mt-4 text-center text-xs text-dark-text-quaternary">
            Make an API key under Settings → API Keys. Works with Claude, Codex, Gemini, ChatGPT and anything that speaks HTTP or MCP.
          </p>
        </div>

        <section aria-labelledby="memories-connect-heading" className="mt-16">
          <h2 id="memories-connect-heading" className="text-center text-xs font-medium uppercase tracking-wider text-dark-text-quaternary mb-4">
            Works with everything in TaskAI
          </h2>
          <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
            {connections.map(c => (
              <li key={c.title} className="rounded-lg border border-dark-border-subtle bg-dark-bg-primary/60 p-4">
                <h3 className="text-sm font-semibold text-dark-text-primary">{c.title}</h3>
                <p className="mt-1.5 text-xs text-dark-text-tertiary leading-relaxed">{c.text}</p>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </section>
  )
}
