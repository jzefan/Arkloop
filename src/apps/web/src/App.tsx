import arkloopMark from './assets/arkloop.svg'

function App() {
  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      <main className="mx-auto max-w-3xl px-6 py-16">
        <div className="flex items-center gap-4">
          <img src={arkloopMark} alt="Arkloop" className="h-10 w-10" />
          <h1 className="text-4xl font-semibold tracking-tight">Arkloop Web</h1>
        </div>
        <p className="mt-4 text-slate-300">
          Vite + React + TypeScript + Tailwind 已就绪。
        </p>
      </main>
    </div>
  )
}

export default App
