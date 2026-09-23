# browser-mcp

An MCP server (streamable HTTP) that lets AI tools like Claude Code or Codex control a headless Firefox tab in a limited way, meant for testing web apps.

- One session is one browser context with a single tab. New tabs and popups are closed and reported.
- A session is restricted to a list of domains, each domain also allows its subdomains. If the tab navigates to anything else, the session is closed.
- Optionally the server restricts which domains sessions may use at all (`ALLOWED_DOMAINS`). The allowed domains are logged on startup.
- Sessions close after 5 minutes without a tool call. `keep_alive` resets the timer.
- Tool calls within one session run one at a time.
- Clicks, typing, key presses and wheel scrolling go through Playwright's input pipeline, so the page sees `isTrusted: true` events.
- Dialogs (alert/confirm/prompt) are accepted automatically. They are reported as events, along with page errors, `console.error` output and closed popups.

## Tools

All tools, their arguments (as TypeScript types), the result format and the aria dump syntax are documented in [TOOLS.md](TOOLS.md).

## Running

```sh
docker build -t browser-mcp .
docker run --rm -p 8080:8080 --shm-size=1g browser-mcp
claude mcp add --transport http browser http://localhost:8080/mcp
```

To test an app running on the host from inside the container, use `host.docker.internal` (add `--add-host=host.docker.internal:host-gateway` on Linux) or `--network=host`.

Locally without Docker:

```sh
go run .

# Might be needed to install firefox
go run github.com/mxschmitt/playwright-go/cmd/playwright install --with-deps firefox
```

## Configuration

Every option can be set with a flag or an env variable. A flag wins over the env variable, and the env variable wins over the default. Run `browser-mcp --help` for the flag list.

| flag | env | default | notes |
| --- | --- | --- | --- |
| `--port` | `PORT` | `8080` | |
| `--public-url` | `PUBLIC_URL` | `http://localhost:$PORT` | base url used in screenshot links |
| `--max-sessions` | `MAX_SESSIONS` | `50` | |
| `--allowed-domains` | `ALLOWED_DOMAINS` | none | comma separated list of domains (subdomains included) sessions may use, e.g. `google.com,duckduckgo.com`. A session's `allowed_domains` must fall within this list. Empty means no global limit. The flag can also be repeated. |
| `--ignore-https-errors` | `IGNORE_HTTPS_ERRORS` | `false` | set to `true` for self-signed dev certificates |
| `--headful` | `HEADFUL` | `false` | show the firefox window so you can watch what happens, needs a display (not for docker) |

There is no authentication, anyone who can reach the port can use it.
