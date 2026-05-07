import solidPlugin from "@opentui/solid/bun-plugin"

const platformMap: Record<string, string> = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
}

const archMap: Record<string, string> = {
  arm64: "arm64",
  x64: "x64",
}

const platform = platformMap[process.platform]
const arch = archMap[process.arch]

if (!platform || !arch) {
  throw new Error(`unsupported compile target: ${process.platform}/${process.arch}`)
}

const result = await Bun.build({
  entrypoints: ["./index.tsx"],
  target: "bun",
  outdir: ".",
  plugins: [solidPlugin],
  compile: {
    target: `bun-${platform}-${arch}`,
    outfile: "arkloop-tui",
  },
})

if (!result.success) {
  for (const log of result.logs) {
    const level = log.level.toUpperCase()
    const message = log.message.trim()
    process.stderr.write(`[${level}] ${message}\n`)
  }
  process.exit(1)
}
