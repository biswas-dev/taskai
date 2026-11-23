import { Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import MemoriesHero from '../components/MemoriesHero'

interface BlogPost {
  title: string
  slug: string
  excerpt: string
  date: string
  cover: string
  author: string
}

export default function Landing() {
  const [posts, setPosts] = useState<BlogPost[]>([])

  useEffect(() => {
    fetch('/blog/api/posts')
      .then(r => r.ok ? r.json() : { posts: [] })
      .then((data: { posts: BlogPost[] }) => setPosts((data.posts || []).slice(0, 3)))
      .catch(() => {})
  }, [])

  return (
    <div className="min-h-screen bg-dark-bg-base text-dark-text-primary">

      {/* ── Navigation ── */}
      <nav className="fixed top-0 left-0 right-0 z-50 bg-dark-bg-base/80 backdrop-blur-lg border-b border-dark-border-subtle">
        <div className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
          <Link to="/" className="flex items-center gap-2.5">
            <img src="/logo.svg" alt="TaskAI" className="w-7 h-7" />
            <span className="text-base font-semibold tracking-tight">TaskAI</span>
          </Link>
          <div className="hidden md:flex items-center gap-8 text-sm text-dark-text-tertiary">
            <a href="#memories" className="hover:text-dark-text-primary transition-colors">Memories</a>
            <a href="#features" className="hover:text-dark-text-primary transition-colors">Features</a>
            <a href="#ai" className="hover:text-dark-text-primary transition-colors">AI agents</a>
            <a href="#integrations" className="hover:text-dark-text-primary transition-colors">Integrations</a>
            <a href="/blog/" className="hover:text-dark-text-primary transition-colors">Blog</a>
            <a href="/docs/" className="hover:text-dark-text-primary transition-colors">Docs</a>
          </div>
          <div className="flex items-center gap-3">
            <Link to="/login" className="text-sm text-dark-text-tertiary hover:text-dark-text-primary transition-colors">
              Sign in
            </Link>
            <Link
              to="/signup"
              className="inline-flex items-center px-4 py-2 text-sm font-medium text-white bg-primary-500 hover:bg-primary-600 rounded-md shadow-linear-sm transition-all duration-150"
            >
              Get started free
            </Link>
          </div>
        </div>
      </nav>

      <MemoriesHero />

      {/* ── Project management ── */}
      <section className="relative pt-20 pb-24 md:pt-28 md:pb-32 border-t border-dark-border-subtle overflow-hidden">
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[900px] h-[700px] bg-primary-500/[0.06] rounded-full blur-[140px]" />
          <div className="absolute top-1/4 right-1/4 w-[500px] h-[500px] bg-secondary-500/[0.04] rounded-full blur-[120px]" />
          <div className="absolute bottom-0 left-1/3 w-[400px] h-[300px] bg-primary-500/[0.04] rounded-full blur-[100px]" />
        </div>

        <div className="relative max-w-4xl mx-auto px-6 text-center">
          <div className="inline-flex items-center gap-2 px-3 py-1.5 mb-8 text-xs font-medium text-primary-400 bg-primary-500/10 border border-primary-500/20 rounded-full">
            <span className="w-1.5 h-1.5 rounded-full bg-primary-400 animate-pulse" />
            MCP Server built-in — works with Claude, Cursor, and any MCP client
          </div>

          <h2 className="text-5xl md:text-7xl font-bold tracking-tight leading-[1.05]">
            Project management
            <br />
            <span className="bg-gradient-to-r from-primary-400 via-primary-300 to-secondary-400 bg-clip-text text-transparent">
              built for AI agents
            </span>
          </h2>

          <p className="mt-7 text-lg md:text-xl text-dark-text-tertiary max-w-2xl mx-auto leading-relaxed">
            The complete workspace where AI agents and humans collaborate. Tasks, roadmaps, wiki, knowledge graph, GitHub sync — all accessible via MCP, REST API, or the visual UI.
          </p>

          <div className="mt-10 flex flex-col sm:flex-row items-center justify-center gap-4">
            <Link
              to="/signup"
              className="inline-flex items-center px-8 py-3.5 text-base font-medium text-white bg-primary-500 hover:bg-primary-600 rounded-md shadow-linear transition-all duration-150 w-full sm:w-auto justify-center"
            >
              Start for free
              <svg className="ml-2 w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7l5 5m0 0l-5 5m5-5H6" />
              </svg>
            </Link>
            <a
              href="/docs/"
              className="inline-flex items-center px-6 py-3.5 text-base font-medium text-dark-text-secondary border border-dark-border-medium hover:border-dark-border-strong hover:bg-dark-bg-secondary rounded-md transition-all duration-150 gap-2 w-full sm:w-auto justify-center"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
              </svg>
              Read the docs
            </a>
          </div>

          {/* Compatibility pills */}
          <div className="mt-12 flex flex-wrap items-center justify-center gap-2">
            <span className="text-xs text-dark-text-quaternary mr-2">Works with</span>
            {['Claude', 'Cursor', 'ChatGPT', 'GitHub Copilot', 'Any MCP client'].map(tool => (
              <span key={tool} className="px-2.5 py-1 text-xs font-medium text-dark-text-tertiary bg-dark-bg-primary border border-dark-border-subtle rounded-full">
                {tool}
              </span>
            ))}
          </div>
        </div>

        {/* Fake UI Preview */}
        <div className="relative mt-20 max-w-5xl mx-auto px-6">
          <div className="rounded-xl border border-dark-border-subtle bg-dark-bg-primary shadow-linear-xl overflow-hidden">
            {/* Window chrome */}
            <div className="flex items-center gap-2 px-4 py-3 bg-dark-bg-secondary border-b border-dark-border-subtle">
              <div className="flex gap-1.5">
                <div className="w-3 h-3 rounded-full bg-red-500/60" />
                <div className="w-3 h-3 rounded-full bg-yellow-500/60" />
                <div className="w-3 h-3 rounded-full bg-green-500/60" />
              </div>
              <div className="flex-1 flex justify-center">
                <div className="px-3 py-0.5 text-xs text-dark-text-quaternary bg-dark-bg-tertiary rounded border border-dark-border-subtle w-48 text-center">
                  taskai.cc/app/projects/1
                </div>
              </div>
            </div>

            {/* App layout */}
            <div className="flex h-[340px] md:h-[420px]">
              {/* Sidebar */}
              <div className="w-48 flex-shrink-0 bg-dark-bg-secondary border-r border-dark-border-subtle p-3 hidden md:block">
                <div className="flex items-center gap-2 px-2 py-1.5 mb-4">
                  <div className="w-5 h-5 rounded bg-primary-500/20" />
                  <span className="text-xs font-semibold text-dark-text-secondary">My workspace</span>
                </div>
                {['Board', 'Roadmap', 'Wiki', 'Sprints', 'Graph'].map((item, i) => (
                  <div key={item} className={`flex items-center gap-2 px-2 py-1.5 rounded text-xs mb-0.5 ${i === 0 ? 'bg-primary-500/10 text-primary-400' : 'text-dark-text-tertiary'}`}>
                    <div className={`w-1.5 h-1.5 rounded-full ${i === 0 ? 'bg-primary-400' : 'bg-dark-border-medium'}`} />
                    {item}
                  </div>
                ))}
                <div className="mt-4 pt-3 border-t border-dark-border-subtle">
                  <div className="text-[10px] uppercase tracking-wider text-dark-text-quaternary px-2 mb-1.5">Projects</div>
                  {['AI Dashboard', 'Mobile App', 'API v2'].map(p => (
                    <div key={p} className="flex items-center gap-2 px-2 py-1 text-xs text-dark-text-tertiary">
                      <div className="w-1.5 h-1.5 rounded-full bg-primary-500/40" />
                      {p}
                    </div>
                  ))}
                </div>
              </div>

              {/* Board content */}
              <div className="flex-1 overflow-hidden p-4 bg-dark-bg-base">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-sm font-semibold text-dark-text-primary">Sprint 3 · AI Dashboard</h3>
                  <div className="flex gap-2">
                    <div className="px-2 py-0.5 text-xs text-primary-400 bg-primary-500/10 rounded-full border border-primary-500/20">MCP connected</div>
                  </div>
                </div>

                {/* Kanban columns */}
                <div className="flex gap-3 h-[270px] md:h-[350px]">
                  {[
                    { label: 'Todo', color: 'text-dark-text-tertiary', tasks: [
                      { title: 'Set up auth flow', tag: 'backend', priority: 'high' },
                      { title: 'Design system tokens', tag: 'frontend', priority: 'medium' },
                    ]},
                    { label: 'In Progress', color: 'text-yellow-400', tasks: [
                      { title: 'MCP server integration', tag: 'ai', priority: 'high' },
                      { title: 'Kanban drag-and-drop', tag: 'frontend', priority: 'medium' },
                    ]},
                    { label: 'Done', color: 'text-success-400', tasks: [
                      { title: 'GitHub sync setup', tag: 'backend', priority: 'low' },
                      { title: 'Wiki collaborative editor', tag: 'docs', priority: 'medium' },
                    ]},
                  ].map(col => (
                    <div key={col.label} className="flex-1 min-w-0">
                      <div className={`text-xs font-medium mb-2 ${col.color}`}>{col.label}</div>
                      <div className="space-y-2">
                        {col.tasks.map(task => (
                          <div key={task.title} className="p-2.5 rounded-lg bg-dark-bg-primary border border-dark-border-subtle hover:border-dark-border-medium transition-colors">
                            <p className="text-xs text-dark-text-primary leading-snug mb-2">{task.title}</p>
                            <div className="flex items-center justify-between">
                              <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium ${
                                task.tag === 'ai' ? 'bg-primary-500/15 text-primary-400' :
                                task.tag === 'backend' ? 'bg-success-500/15 text-success-400' :
                                task.tag === 'docs' ? 'bg-blue-500/15 text-blue-400' :
                                'bg-dark-bg-tertiary text-dark-text-quaternary'
                              }`}>{task.tag}</span>
                              <span className={`text-[10px] ${task.priority === 'high' ? 'text-red-400' : task.priority === 'medium' ? 'text-yellow-400' : 'text-dark-text-quaternary'}`}>
                                {task.priority}
                              </span>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
          {/* Glow below */}
          <div className="absolute -bottom-8 left-1/2 -translate-x-1/2 w-3/4 h-16 bg-primary-500/10 blur-2xl rounded-full pointer-events-none" />
        </div>
      </section>

      {/* ── Feature pillars ── */}
      <section id="features" className="py-24 md:py-32 border-t border-dark-border-subtle">
        <div className="max-w-6xl mx-auto px-6">
          <div className="text-center mb-20">
            <h2 className="text-3xl md:text-5xl font-bold tracking-tight">
              Everything your team needs.
              <br />
              <span className="text-dark-text-tertiary font-normal">Nothing you don't.</span>
            </h2>
            <p className="mt-5 text-base text-dark-text-tertiary max-w-xl mx-auto">
              A complete toolkit for modern software teams — built to be controlled by both humans and AI agents.
            </p>
          </div>

          {/* Feature grid: 2-column alternating large feature + detail */}
          <div className="space-y-6">

            {/* Row 1: Tasks + Roadmap + Wiki */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              {/* Tasks */}
              <div className="group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-primary-500/30 transition-all duration-300 overflow-hidden">
                <div className="absolute inset-0 bg-gradient-to-br from-primary-500/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
                <div className="relative">
                  <div className="w-11 h-11 bg-primary-500/10 rounded-xl flex items-center justify-center mb-5">
                    <svg className="w-5 h-5 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 17V7m0 10a2 2 0 01-2 2H5a2 2 0 01-2-2V7a2 2 0 012-2h2a2 2 0 012 2m0 10a2 2 0 002 2h2a2 2 0 002-2M9 7a2 2 0 012-2h2a2 2 0 012 2m0 10V7m0 10a2 2 0 002 2h2a2 2 0 002-2V7a2 2 0 00-2-2h-2a2 2 0 00-2 2" />
                    </svg>
                  </div>
                  <h3 className="text-xl font-semibold mb-3 tracking-tight">Kanban boards</h3>
                  <p className="text-sm text-dark-text-tertiary leading-relaxed mb-4">
                    Drag-and-drop tasks across fully customizable swim lanes. Set priorities, assignees, due dates, tags. Real-time sync so your whole team sees changes instantly.
                  </p>
                  <ul className="space-y-1.5">
                    {['Custom swim lanes', 'Multi-assignee support', 'Tags & labels', 'Priority levels', 'Due dates & start dates'].map(f => (
                      <li key={f} className="flex items-center gap-2 text-xs text-dark-text-secondary">
                        <svg className="w-3.5 h-3.5 text-success-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 13l4 4L19 7" />
                        </svg>
                        {f}
                      </li>
                    ))}
                  </ul>
                </div>
              </div>

              {/* Roadmap */}
              <div className="group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-teal-500/30 transition-all duration-300 overflow-hidden">
                <div className="absolute inset-0 bg-gradient-to-br from-teal-500/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
                <div className="relative">
                  <div className="w-11 h-11 bg-teal-500/10 rounded-xl flex items-center justify-center mb-5">
                    <svg className="w-5 h-5 text-teal-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                    </svg>
                  </div>
                  <h3 className="text-xl font-semibold mb-3 tracking-tight">Roadmap planning</h3>
                  <p className="text-sm text-dark-text-tertiary leading-relaxed mb-4">
                    Plan milestones, track dependencies, and visualize progress across your project. Timeline, board, and list views with AI-powered capacity analysis.
                  </p>
                  <ul className="space-y-1.5">
                    {['Milestone tracking', 'Task dependencies', 'Timeline & Gantt view', 'Progress analytics', 'AI capacity planning'].map(f => (
                      <li key={f} className="flex items-center gap-2 text-xs text-dark-text-secondary">
                        <svg className="w-3.5 h-3.5 text-success-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 13l4 4L19 7" />
                        </svg>
                        {f}
                      </li>
                    ))}
                  </ul>
                </div>
              </div>

              {/* Wiki */}
              <div className="group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-blue-500/30 transition-all duration-300 overflow-hidden">
                <div className="absolute inset-0 bg-gradient-to-br from-blue-500/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
                <div className="relative">
                  <div className="w-11 h-11 bg-blue-500/10 rounded-xl flex items-center justify-center mb-5">
                    <svg className="w-5 h-5 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
                    </svg>
                  </div>
                  <h3 className="text-xl font-semibold mb-3 tracking-tight">Collaborative wiki</h3>
                  <p className="text-sm text-dark-text-tertiary leading-relaxed mb-4">
                    A full markdown wiki with real-time multiplayer editing powered by Yjs. Embed Figma designs, interactive drawings, images, and code. Annotate directly on pages.
                  </p>
                  <ul className="space-y-1.5">
                    {['Real-time multiplayer editing', 'Figma & drawing embeds', 'Page version history', 'Inline annotations', 'Full markdown + code blocks'].map(f => (
                      <li key={f} className="flex items-center gap-2 text-xs text-dark-text-secondary">
                        <svg className="w-3.5 h-3.5 text-success-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 13l4 4L19 7" />
                        </svg>
                        {f}
                      </li>
                    ))}
                  </ul>
                </div>
              </div>
            </div>

            {/* Row 2: Sprints (wide) + Knowledge Graph */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              {/* Sprints — 2 cols */}
              <div className="md:col-span-2 group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-secondary-500/30 transition-all duration-300 overflow-hidden">
                <div className="absolute inset-0 bg-gradient-to-br from-secondary-500/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
                <div className="relative flex flex-col md:flex-row gap-8">
                  <div className="md:w-64 flex-shrink-0">
                    <div className="w-11 h-11 bg-secondary-500/10 rounded-xl flex items-center justify-center mb-5">
                      <svg className="w-5 h-5 text-secondary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M13 10V3L4 14h7v7l9-11h-7z" />
                      </svg>
                    </div>
                    <h3 className="text-xl font-semibold mb-3 tracking-tight">Sprint planning</h3>
                    <p className="text-sm text-dark-text-tertiary leading-relaxed">
                      Plan and track sprints with start/end dates, sprint goals, and velocity tracking. Assign tasks to sprints and monitor progress in real time.
                    </p>
                  </div>
                  {/* Sprint visual */}
                  <div className="flex-1 bg-dark-bg-secondary rounded-xl p-4 border border-dark-border-subtle">
                    <div className="text-xs text-dark-text-tertiary mb-3 font-medium">Sprint 3 · 14 days remaining</div>
                    <div className="space-y-2">
                      {[
                        { label: 'Completed', pct: 62, color: 'bg-success-500' },
                        { label: 'In Progress', pct: 25, color: 'bg-yellow-400' },
                        { label: 'Remaining', pct: 13, color: 'bg-dark-border-medium' },
                      ].map(bar => (
                        <div key={bar.label} className="flex items-center gap-3">
                          <span className="w-20 text-xs text-dark-text-quaternary">{bar.label}</span>
                          <div className="flex-1 h-1.5 bg-dark-bg-tertiary rounded-full overflow-hidden">
                            <div className={`h-full ${bar.color} rounded-full`} style={{ width: `${bar.pct}%` }} />
                          </div>
                          <span className="text-xs text-dark-text-quaternary w-8 text-right">{bar.pct}%</span>
                        </div>
                      ))}
                    </div>
                    <div className="mt-3 pt-3 border-t border-dark-border-subtle grid grid-cols-3 gap-2 text-center">
                      {[{n:'8',l:'Done'},{n:'3',l:'Active'},{n:'2',l:'Blocked'}].map(s=>(
                        <div key={s.l}>
                          <div className="text-lg font-bold text-dark-text-primary">{s.n}</div>
                          <div className="text-[10px] text-dark-text-quaternary">{s.l}</div>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              </div>

              {/* Knowledge Graph */}
              <div className="group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-orange-500/30 transition-all duration-300 overflow-hidden">
                <div className="absolute inset-0 bg-gradient-to-br from-orange-500/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
                <div className="relative">
                  <div className="w-11 h-11 bg-orange-500/10 rounded-xl flex items-center justify-center mb-5">
                    <svg className="w-5 h-5 text-orange-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />
                    </svg>
                  </div>
                  <h3 className="text-xl font-semibold mb-3 tracking-tight">Knowledge graph</h3>
                  <p className="text-sm text-dark-text-tertiary leading-relaxed mb-4">
                    Visualize connections between wiki pages, tasks, and team members. Discover how ideas relate and navigate your project context like a map.
                  </p>
                  {/* Fake graph nodes */}
                  <div className="relative h-28 bg-dark-bg-secondary rounded-lg border border-dark-border-subtle overflow-hidden">
                    <svg className="absolute inset-0 w-full h-full" viewBox="0 0 200 112">
                      <line x1="100" y1="56" x2="40" y2="30" stroke="rgba(99,102,241,0.3)" strokeWidth="1" />
                      <line x1="100" y1="56" x2="160" y2="28" stroke="rgba(99,102,241,0.3)" strokeWidth="1" />
                      <line x1="100" y1="56" x2="50" y2="85" stroke="rgba(99,102,241,0.3)" strokeWidth="1" />
                      <line x1="100" y1="56" x2="155" y2="80" stroke="rgba(99,102,241,0.3)" strokeWidth="1" />
                      <line x1="40" y1="30" x2="160" y2="28" stroke="rgba(99,102,241,0.15)" strokeWidth="0.5" strokeDasharray="3,3" />
                      <circle cx="100" cy="56" r="9" fill="rgba(99,102,241,0.4)" />
                      <circle cx="40" cy="30" r="6" fill="rgba(59,130,246,0.4)" />
                      <circle cx="160" cy="28" r="5" fill="rgba(59,130,246,0.3)" />
                      <circle cx="50" cy="85" r="5" fill="rgba(34,197,94,0.4)" />
                      <circle cx="155" cy="80" r="6" fill="rgba(34,197,94,0.3)" />
                      <text x="100" y="59" textAnchor="middle" fill="rgba(255,255,255,0.6)" fontSize="5">API v2</text>
                      <text x="40" y="46" textAnchor="middle" fill="rgba(255,255,255,0.4)" fontSize="4">Auth</text>
                      <text x="160" y="44" textAnchor="middle" fill="rgba(255,255,255,0.4)" fontSize="4">Docs</text>
                      <text x="50" y="100" textAnchor="middle" fill="rgba(255,255,255,0.4)" fontSize="4">Tests</text>
                      <text x="155" y="96" textAnchor="middle" fill="rgba(255,255,255,0.4)" fontSize="4">Deploy</text>
                    </svg>
                  </div>
                </div>
              </div>
            </div>

            {/* Row 3: GitHub + File Attachments + Figma */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">

              {/* GitHub Integration */}
              <div className="group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-dark-border-medium transition-all duration-300 overflow-hidden">
                <div className="relative">
                  <div className="w-11 h-11 bg-dark-bg-tertiary rounded-xl flex items-center justify-center mb-5">
                    <svg className="w-5 h-5 text-dark-text-secondary" viewBox="0 0 24 24" fill="currentColor">
                      <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
                    </svg>
                  </div>
                  <h3 className="text-xl font-semibold mb-3 tracking-tight">GitHub sync</h3>
                  <p className="text-sm text-dark-text-tertiary leading-relaxed mb-4">
                    Two-way sync with GitHub Issues and PRs. Import issues as tasks, push task changes back, sync comments and reactions. All automatically, on a schedule.
                  </p>
                  <div className="flex flex-wrap gap-1.5">
                    {['Issue sync', 'PR tracking', 'Comment sync', 'Reactions', 'Auto-schedule'].map(tag => (
                      <span key={tag} className="px-2 py-0.5 text-[10px] font-medium bg-dark-bg-tertiary border border-dark-border-subtle rounded text-dark-text-tertiary">{tag}</span>
                    ))}
                  </div>
                </div>
              </div>

              {/* File attachments */}
              <div className="group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-orange-500/30 transition-all duration-300 overflow-hidden">
                <div className="relative">
                  <div className="w-11 h-11 bg-orange-500/10 rounded-xl flex items-center justify-center mb-5">
                    <svg className="w-5 h-5 text-orange-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13" />
                    </svg>
                  </div>
                  <h3 className="text-xl font-semibold mb-3 tracking-tight">File attachments</h3>
                  <p className="text-sm text-dark-text-tertiary leading-relaxed mb-4">
                    Attach images, videos, and PDFs directly to tasks and wiki pages. Stored in your own Cloudinary account — you own your data. Inline previews everywhere.
                  </p>
                  {/* Fake file list */}
                  <div className="space-y-2">
                    {[{name:'wireframe-v2.fig', type:'FIG', color:'bg-purple-500/20 text-purple-300'},{name:'screenshot.png', type:'PNG', color:'bg-blue-500/20 text-blue-300'},{name:'spec.pdf', type:'PDF', color:'bg-red-500/20 text-red-300'}].map(f=>(
                      <div key={f.name} className="flex items-center gap-2 p-2 rounded bg-dark-bg-secondary border border-dark-border-subtle">
                        <span className={`text-[9px] font-bold px-1 py-0.5 rounded ${f.color}`}>{f.type}</span>
                        <span className="text-xs text-dark-text-secondary truncate">{f.name}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>

              {/* Figma embeds */}
              <div className="group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-purple-500/30 transition-all duration-300 overflow-hidden">
                <div className="relative">
                  <div className="w-11 h-11 bg-purple-500/10 rounded-xl flex items-center justify-center mb-5">
                    <svg width="20" height="20" viewBox="0 0 38 57" fill="none">
                      <path d="M19 28.5c0-2.674 1.054-5.24 2.929-7.115C23.804 19.51 26.37 18.457 29.043 18.457s5.239 1.053 7.115 2.928c1.875 1.875 2.929 4.441 2.929 7.115s-1.054 5.24-2.929 7.115C34.282 37.49 31.717 38.543 29.043 38.543s-5.239-1.053-7.114-2.928C20.054 33.74 19 31.174 19 28.5z" fill="#1ABCFE"/>
                      <path d="M-1.087 47.543c0-2.674 1.053-5.24 2.929-7.115C3.717 38.553 6.283 37.5 8.956 37.5H19v10.043c0 2.674-1.053 5.24-2.929 7.115C14.196 56.533 11.63 57.587 8.957 57.587s-5.239-1.054-7.115-2.929C-.033 52.782-1.087 50.217-1.087 47.543z" fill="#0ACF83"/>
                      <path d="M19 .413V18.457h10.043c2.674 0 5.239-1.053 7.115-2.929 1.875-1.875 2.929-4.44 2.929-7.114S38.033 3.174 36.158 1.298C34.282-.577 31.717-1.63 29.043-1.63H19V.413z" fill="#FF7262"/>
                      <path d="M-1.087 8.413c0 2.674 1.053 5.24 2.929 7.115C3.717 17.403 6.283 18.457 8.956 18.457H19V-1.587H8.956c-2.673 0-5.239 1.054-7.114 2.929C-.033 3.217-1.087 5.74-1.087 8.413z" fill="#F24E1E"/>
                      <path d="M-1.087 28.5c0 2.674 1.053 5.24 2.929 7.115C3.717 37.49 6.283 38.543 8.956 38.543H19V18.457H8.956c-2.673 0-5.239 1.053-7.114 2.928C-.033 23.261-1.087 25.826-1.087 28.5z" fill="#A259FF"/>
                    </svg>
                  </div>
                  <h3 className="text-xl font-semibold mb-3 tracking-tight">Figma embeds</h3>
                  <p className="text-sm text-dark-text-tertiary leading-relaxed mb-4">
                    Paste a Figma URL anywhere — wiki pages, task descriptions, comments. Get a live thumbnail preview and interactive iframe, with your designs always in context.
                  </p>
                  {/* Fake embed preview */}
                  <div className="rounded-lg border border-dark-border-subtle bg-dark-bg-secondary overflow-hidden">
                    <div className="flex items-center gap-2 px-3 py-2 border-b border-dark-border-subtle">
                      <div className="w-3 h-3 rounded-full bg-purple-400/60" />
                      <span className="text-[10px] text-dark-text-tertiary flex-1 truncate">App redesign v4</span>
                      <span className="text-[10px] text-primary-400">View embed</span>
                    </div>
                    <div className="h-16 bg-gradient-to-br from-purple-500/10 to-blue-500/10 flex items-center justify-center">
                      <span className="text-xs text-dark-text-quaternary">Figma preview</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ── Knowledge Spine ── */}
      <section id="knowledge-spine" className="py-24 md:py-32 border-t border-dark-border-subtle relative overflow-hidden">
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute top-1/3 left-1/4 w-[500px] h-[500px] bg-violet-500/[0.04] rounded-full blur-[120px]" />
          <div className="absolute bottom-1/4 right-1/4 w-[400px] h-[400px] bg-emerald-500/[0.04] rounded-full blur-[100px]" />
        </div>
        <div className="max-w-6xl mx-auto px-6">
          <div className="text-center mb-16">
            <div className="inline-flex items-center gap-2 px-3 py-1.5 mb-6 text-xs font-medium text-violet-400 bg-violet-500/10 border border-violet-500/20 rounded-full">
              <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
              </svg>
              Knowledge Spine
            </div>
            <h2 className="text-3xl md:text-5xl font-bold tracking-tight">
              Your wiki becomes the
              <br />
              <span className="bg-gradient-to-r from-violet-400 via-purple-300 to-emerald-400 bg-clip-text text-transparent">
                AI's memory
              </span>
            </h2>
            <p className="mt-5 text-base text-dark-text-tertiary max-w-2xl mx-auto leading-relaxed">
              Every wiki page you write is automatically vectorized and indexed. When an AI agent needs context — deployment patterns, API contracts, architecture decisions — it finds exactly the right knowledge in milliseconds, not by keyword matching, but by understanding what you mean.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-12">
            {/* Automatic Embedding */}
            <div className="group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-violet-500/30 transition-all duration-300 overflow-hidden">
              <div className="absolute inset-0 bg-gradient-to-br from-violet-500/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
              <div className="relative">
                <div className="w-11 h-11 bg-violet-500/10 rounded-xl flex items-center justify-center mb-5">
                  <svg className="w-5 h-5 text-violet-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                  </svg>
                </div>
                <h3 className="text-lg font-semibold mb-3 tracking-tight">Auto-Embedding Pipeline</h3>
                <p className="text-sm text-dark-text-tertiary leading-relaxed">
                  Every 2 minutes, updated wiki pages are split into blocks and vectorized using a local embedding model. Zero configuration — it just works.
                </p>
              </div>
            </div>

            {/* Semantic Search */}
            <div className="group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-emerald-500/30 transition-all duration-300 overflow-hidden">
              <div className="absolute inset-0 bg-gradient-to-br from-emerald-500/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
              <div className="relative">
                <div className="w-11 h-11 bg-emerald-500/10 rounded-xl flex items-center justify-center mb-5">
                  <svg className="w-5 h-5 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                  </svg>
                </div>
                <h3 className="text-lg font-semibold mb-3 tracking-tight">Hybrid Search</h3>
                <p className="text-sm text-dark-text-tertiary leading-relaxed">
                  Combines full-text keyword search with vector similarity. "How does auth work" finds your "OAuth2 Login Flow" page — even without keyword overlap.
                </p>
              </div>
            </div>

            {/* Local & Private */}
            <div className="group relative p-8 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-amber-500/30 transition-all duration-300 overflow-hidden">
              <div className="absolute inset-0 bg-gradient-to-br from-amber-500/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
              <div className="relative">
                <div className="w-11 h-11 bg-amber-500/10 rounded-xl flex items-center justify-center mb-5">
                  <svg className="w-5 h-5 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                  </svg>
                </div>
                <h3 className="text-lg font-semibold mb-3 tracking-tight">100% Local</h3>
                <p className="text-sm text-dark-text-tertiary leading-relaxed">
                  Embeddings run on your own server via Ollama. No API keys, no cloud calls, no usage fees. Your knowledge never leaves your infrastructure.
                </p>
              </div>
            </div>
          </div>

          {/* Before/After comparison */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6 max-w-4xl mx-auto">
            <div className="p-6 rounded-2xl bg-dark-bg-primary border border-red-500/20">
              <div className="text-xs font-semibold text-red-400 uppercase tracking-wider mb-4">Without a Knowledge Spine</div>
              <div className="space-y-3">
                {[
                  'LLM reads 50,000 tokens of raw code',
                  'Misses relevant files buried in 200+ files',
                  'Hallucinates function signatures',
                  'Output: plausible but broken code',
                ].map(item => (
                  <div key={item} className="flex items-start gap-2 text-sm text-dark-text-tertiary">
                    <svg className="w-4 h-4 text-red-400 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                    {item}
                  </div>
                ))}
              </div>
            </div>
            <div className="p-6 rounded-2xl bg-dark-bg-primary border border-emerald-500/20">
              <div className="text-xs font-semibold text-emerald-400 uppercase tracking-wider mb-4">With a Knowledge Spine</div>
              <div className="space-y-3">
                {[
                  'LLM queries: "how does deployment work?"',
                  'Vector search returns the 3 exact wiki sections',
                  'Sees real APIs, real patterns, real conventions',
                  'Output: production-quality code that fits',
                ].map(item => (
                  <div key={item} className="flex items-start gap-2 text-sm text-dark-text-tertiary">
                    <svg className="w-4 h-4 text-emerald-400 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                    </svg>
                    {item}
                  </div>
                ))}
              </div>
            </div>
          </div>

          {/* Stats bar */}
          <div className="mt-12 grid grid-cols-2 md:grid-cols-4 gap-4 max-w-3xl mx-auto">
            {[
              { value: '<50ms', label: 'Search latency' },
              { value: '384', label: 'Vector dimensions' },
              { value: '98%', label: 'Blocks embedded' },
              { value: '$0', label: 'Additional cost' },
            ].map(stat => (
              <div key={stat.label} className="text-center p-4">
                <div className="text-2xl font-bold text-dark-text-primary">{stat.value}</div>
                <div className="text-xs text-dark-text-quaternary mt-1">{stat.label}</div>
              </div>
            ))}
          </div>

          <div className="text-center mt-10">
            <a
              href="https://taskai.cc/blog/knowledge-spine-ai-code-quality"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 text-sm text-violet-400 hover:text-violet-300 transition-colors"
            >
              Read the deep dive: The Knowledge Spine
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
              </svg>
            </a>
          </div>
        </div>
      </section>

      {/* ── AI Agents section ── */}
      <section id="ai" className="py-24 md:py-32 border-t border-dark-border-subtle">
        <div className="max-w-6xl mx-auto px-6">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-16 items-center">
            <div>
              <div className="inline-flex items-center gap-2 px-3 py-1.5 mb-6 text-xs font-medium text-primary-400 bg-primary-500/10 border border-primary-500/20 rounded-full">
                <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
                Model Context Protocol
              </div>
              <h2 className="text-3xl md:text-4xl font-bold tracking-tight mb-6">
                Let your AI agent
                <br />
                run your standup
              </h2>
              <p className="text-dark-text-tertiary leading-relaxed mb-8">
                TaskAI ships with a built-in MCP server. Point Claude, Cursor, or any compatible agent at your workspace and it can read project context, create and update tasks, manage sprints and milestones, track roadmap progress — all through natural language.
              </p>
              <div className="space-y-4">
                {[
                  { q: '"What\'s blocked in sprint 3?"', a: '3 tasks blocked on auth service deployment. Assigned to @alex.' },
                  { q: '"Create tasks for the auth refactor"', a: 'Created 5 tasks in the backend project. Sprint 4, high priority.' },
                  { q: '"Is the v2.0 milestone on track?"', a: 'v2.0 is 68% complete with 3 weeks left. Auth refactor is blocking 2 tasks. Suggest rebalancing sprint 4.' },
                ].map((item, i) => (
                  <div key={i} className="p-4 rounded-xl bg-dark-bg-primary border border-dark-border-subtle">
                    <p className="text-sm font-medium text-primary-300 mb-2">{item.q}</p>
                    <p className="text-sm text-dark-text-tertiary">{item.a}</p>
                  </div>
                ))}
              </div>
            </div>

            <div className="space-y-4">
              <div className="p-6 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle">
                <div className="text-xs font-semibold text-dark-text-quaternary uppercase tracking-wider mb-4">MCP tools exposed</div>
                <div className="grid grid-cols-2 gap-2">
                  {[
                    'list_projects', 'create_task', 'update_task', 'list_tasks',
                    'create_wiki_page', 'search_wiki', 'list_milestones', 'add_dependency',
                    'list_sprints', 'get_milestone_progress', 'create_milestone', '+ 40 more',
                  ].map(tool => (
                    <div key={tool} className="flex items-center gap-1.5 text-xs text-dark-text-tertiary font-mono">
                      <span className="text-success-400">→</span>
                      <span className={tool === '+ 40 more' ? 'text-primary-400' : ''}>{tool}</span>
                    </div>
                  ))}
                </div>
              </div>

              <div className="p-6 rounded-2xl bg-dark-bg-primary border border-dark-border-subtle">
                <div className="text-xs font-semibold text-dark-text-quaternary uppercase tracking-wider mb-4">Configure in seconds</div>
                <div className="bg-dark-bg-secondary rounded-lg p-3 font-mono text-xs text-dark-text-secondary border border-dark-border-subtle">
                  <div className="text-dark-text-quaternary mb-1">// claude_desktop_config.json</div>
                  <div><span className="text-blue-400">"mcpServers"</span><span className="text-dark-text-tertiary">: {'{'}</span></div>
                  <div className="ml-4"><span className="text-green-400">"taskai"</span><span className="text-dark-text-tertiary">: {'{'}</span></div>
                  <div className="ml-8"><span className="text-yellow-400">"command"</span><span className="text-dark-text-tertiary">: </span><span className="text-orange-400">"npx"</span><span className="text-dark-text-tertiary">,</span></div>
                  <div className="ml-8"><span className="text-yellow-400">"args"</span><span className="text-dark-text-tertiary">: [</span><span className="text-orange-400">"-y mcp-remote"</span><span className="text-dark-text-tertiary">,</span></div>
                  <div className="ml-12"><span className="text-orange-400">"https://mcp.taskai.cc/mcp"</span><span className="text-dark-text-tertiary">]</span></div>
                  <div className="ml-4 text-dark-text-tertiary">{'}'}</div>
                  <div className="text-dark-text-tertiary">{'}'}</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ── Integrations & everything else ── */}
      <section id="integrations" className="py-24 md:py-32 border-t border-dark-border-subtle">
        <div className="max-w-6xl mx-auto px-6">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold tracking-tight mb-4">
              Everything else you'd expect
            </h2>
            <p className="text-dark-text-tertiary max-w-lg mx-auto">
              We've thought about the details so your team doesn't have to.
            </p>
          </div>

          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            {[
              { icon: '👥', title: 'Teams', desc: 'Invite members, manage roles, shared projects across your org' },
              { icon: '🔐', title: '2FA & Security', desc: 'Two-factor auth, bcrypt passwords, JWT sessions, rate limiting' },
              { icon: '🔑', title: 'API Keys', desc: 'Generate API keys with expiry for automation and integrations' },
              { icon: '💬', title: 'Task comments', desc: 'Threaded comments with GitHub-style emoji reactions' },
              { icon: '🔔', title: 'Notifications', desc: 'In-app notification feed for task updates, mentions, and activity' },
              { icon: '👤', title: 'User profiles', desc: 'Activity feeds, contribution history across shared projects' },
              { icon: '🎨', title: 'Canvas drawings', desc: 'Embed interactive whiteboards directly in wiki pages' },
              { icon: '🌐', title: 'OAuth login', desc: 'Sign in with Google or GitHub. Invite-gated by default.' },
              { icon: '📎', title: 'File storage', desc: 'Per-user Cloudinary storage, drag & drop uploads, inline previews' },
              { icon: '🔍', title: 'Full-text search', desc: 'Search tasks and wiki content across all your projects' },
              { icon: '📊', title: 'Activity feed', desc: 'Real-time project activity log with filtering and search' },
              { icon: '🔁', title: 'Real-time sync', desc: 'All changes sync instantly across all connected clients' },
            ].map(item => (
              <div key={item.title} className="p-5 rounded-xl bg-dark-bg-primary border border-dark-border-subtle hover:border-dark-border-medium transition-all duration-200 group">
                <div className="text-2xl mb-3">{item.icon}</div>
                <h4 className="text-sm font-semibold text-dark-text-primary mb-1.5">{item.title}</h4>
                <p className="text-xs text-dark-text-quaternary leading-relaxed">{item.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Blog ── */}
      {posts.length > 0 && (
        <section className="py-20 md:py-28 border-t border-dark-border-subtle">
          <div className="max-w-6xl mx-auto px-6">
            <div className="flex items-center justify-between mb-12">
              <div>
                <h2 className="text-2xl md:text-3xl font-bold tracking-tight">From the blog</h2>
                <p className="mt-2 text-sm text-dark-text-tertiary">Building AI-native tools, one commit at a time.</p>
              </div>
              <a
                href="/blog/"
                className="text-sm text-primary-400 hover:text-primary-300 transition-colors hidden md:flex items-center gap-1"
              >
                All posts
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7l5 5m0 0l-5 5m5-5H6" />
                </svg>
              </a>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              {posts.map(post => (
                <a
                  key={post.slug}
                  href={`/blog/${post.slug}`}
                  className="group rounded-2xl bg-dark-bg-primary border border-dark-border-subtle hover:border-primary-500/30 transition-all duration-300 overflow-hidden"
                >
                  {post.cover && (
                    <div className="aspect-[16/9] bg-dark-bg-secondary overflow-hidden">
                      <img
                        src={post.cover}
                        alt={post.title}
                        className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500"
                      />
                    </div>
                  )}
                  <div className="p-6">
                    <div className="text-xs text-dark-text-quaternary mb-3">{new Date(post.date).toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' })}</div>
                    <h3 className="text-base font-semibold tracking-tight mb-2 group-hover:text-primary-400 transition-colors leading-snug">
                      {post.title}
                    </h3>
                    <p className="text-sm text-dark-text-tertiary leading-relaxed line-clamp-2">
                      {post.excerpt}
                    </p>
                  </div>
                </a>
              ))}
            </div>

            <div className="mt-8 text-center md:hidden">
              <a href="/blog/" className="text-sm text-primary-400 hover:text-primary-300 transition-colors">
                View all posts →
              </a>
            </div>
          </div>
        </section>
      )}

      {/* ── CTA ── */}
      <section className="py-24 md:py-32 border-t border-dark-border-subtle relative overflow-hidden">
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[400px] bg-primary-500/[0.07] rounded-full blur-[100px]" />
        </div>
        <div className="relative max-w-3xl mx-auto px-6 text-center">
          <h2 className="text-4xl md:text-5xl font-bold tracking-tight mb-6">
            Start shipping with AI today
          </h2>
          <p className="text-lg text-dark-text-tertiary mb-10 max-w-xl mx-auto leading-relaxed">
            Free to get started. Connect your AI agent in under a minute. Your whole team — human and AI — on the same board.
          </p>
          <div className="flex flex-col sm:flex-row items-center justify-center gap-4">
            <Link
              to="/signup"
              className="inline-flex items-center px-8 py-4 text-base font-medium text-white bg-primary-500 hover:bg-primary-600 rounded-md shadow-linear transition-all duration-150 w-full sm:w-auto justify-center"
            >
              Create your workspace
              <svg className="ml-2 w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7l5 5m0 0l-5 5m5-5H6" />
              </svg>
            </Link>
            <a
              href="/docs/"
              className="inline-flex items-center px-6 py-4 text-base font-medium text-dark-text-secondary border border-dark-border-medium hover:border-dark-border-strong hover:bg-dark-bg-secondary rounded-md transition-all duration-150 gap-2 w-full sm:w-auto justify-center"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
              </svg>
              Read the docs
            </a>
          </div>
          <p className="mt-6 text-sm text-dark-text-quaternary">No credit card required. Works with Claude, Cursor, and any MCP client.</p>
        </div>
      </section>

      {/* ── Footer ── */}
      <footer className="py-12 border-t border-dark-border-subtle">
        <div className="max-w-6xl mx-auto px-6 flex flex-col md:flex-row items-center justify-between gap-6">
          <div className="flex items-center gap-2">
            <img src="/logo.svg" alt="TaskAI" className="w-5 h-5 opacity-60" />
            <span className="text-sm text-dark-text-quaternary font-medium">TaskAI</span>
          </div>
          <div className="flex items-center gap-6 text-sm text-dark-text-quaternary">
            <a href="/blog/" className="hover:text-dark-text-tertiary transition-colors">Blog</a>
            <a href="/docs/" className="hover:text-dark-text-tertiary transition-colors">Docs</a>
            <Link to="/login" className="hover:text-dark-text-tertiary transition-colors">Sign in</Link>
            <Link to="/signup" className="hover:text-dark-text-tertiary transition-colors">Sign up</Link>
          </div>
          <p className="text-sm text-dark-text-quaternary">
            Project management for AI-native teams.
          </p>
        </div>
      </footer>
    </div>
  )
}
