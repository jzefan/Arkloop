import { render } from "@opentui/solid"
import { parseFlags, resolveConfig } from "./src/lib/config"
import { ApiClient } from "./src/api/client"
import { App } from "./src/components/App"
import { setConnected } from "./src/store/app"

const flags = parseFlags(process.argv.slice(2))
const config = resolveConfig(flags)
const client = new ApiClient(config)

// Verify connection
try {
  await client.getMe()
} catch (err) {
  process.stderr.write(`Failed to connect to Desktop API at ${config.host}\n`)
  process.stderr.write(err instanceof Error ? err.message : String(err))
  process.stderr.write("\n")
  setConnected(false)
  process.exit(1)
}

render(() => <App client={client} />)
