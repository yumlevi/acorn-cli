# acorn — Go port

Go / Bubble Tea rewrite of acorn-cli. Distribution is a single static
binary per OS/arch — no Python, no Node, no runtime dependencies, launches
from any directory once dropped in `$PATH`.

## Why

The Python implementation uses Textual, which glitches on older Windows
terminals and requires a working Python install + pip/pipx. The Go port
targets:

- **Zero-install** — download one binary, `chmod +x`, done.
- **Robust Windows** — Bubble Tea uses the Windows virtual-terminal API
  directly and falls back gracefully on cmd.exe.
- **Fast startup** — compiled Go boots in tens of ms vs Python's ~1s.

## Build

Requires Go 1.22+.

```sh
cd go
go mod tidy
make build          # ./acorn
make install        # ~/.local/bin/acorn
make release        # cross-compile linux/darwin/windows × amd64/arm64 into dist/
```

## Install (prebuilt binary)

```sh
# Linux/mac
curl -sSL https://github.com/yumlevi/acorn-cli/releases/latest/download/acorn-$(uname -s | tr A-Z a-z)-$(uname -m) \
  -o ~/.local/bin/acorn && chmod +x ~/.local/bin/acorn

# Windows PowerShell
iwr https://github.com/yumlevi/acorn-cli/releases/latest/download/acorn-windows-amd64.exe -OutFile acorn.exe
```

## Configure

Global config at `~/.acorn/config.toml`, per-project override at
`./.acorn/config.toml` (project wins).

```toml
[connection]
host = "spore.hyrule.vip"   # or an https://… URL
port = 18801
user = "yam"
key = "<your-acorn-team-key>"

[display]
theme = "dark"              # dark, oak, forest, oled, light
show_thinking = true
show_tools = true
show_usage = true
```

If this file doesn't exist the Go port errors out and tells you to create
it. The Python port has an interactive setup wizard — run `python -m
acorn` once to go through it, or write the TOML by hand.

## Run

```sh
acorn                       # normal mode — REPL in your cwd
acorn -c                    # resume the last session (saved in ~/.acorn/last_session)
acorn --session cli:…-…-…   # resume a specific session
acorn --plan                # start in plan mode
acorn --host spore.tld --port 443 --user foo
```

## Keybindings

| Key          | Action |
|--------------|--------|
| Enter        | Send message |
| Alt+Enter    | Newline in input (multi-line drafting) |
| Shift+Tab    | Toggle plan / execute mode |
| PgUp / PgDn  | Scroll chat history |
| Esc          | Close modal |
| Ctrl+C       | Quit |

## Slash commands

| Command | What it does |
|---------|--------------|
| `/help` | Show this list |
| `/new` | Fresh session in current cwd |
| `/clear` | Clear chat (server-side too) |
| `/resume <sessionId>` | Resume a specific session |
| `/sessions` | List saved sessions for this project |
| `/quit` | Exit |
| `/stop` | Stop the current generation |
| `/plan` | Toggle plan/execute mode (same as Shift+Tab) |
| `/status` | Connection + session info |
| `/theme <name>` | Switch theme (dark/oak/forest/oled/light) |
| `/mode <auto\|ask\|locked\|yolo\|rules>` | Tool approval mode |
| `/approve-all` | `/mode auto` shortcut |
| `/approve-all-dangerous` | `/mode yolo` shortcut |
| `/bg [list]` | Background process list (stub — see notes) |
| `/update [check]` | Check GitHub releases for newer versions |

## Feature parity vs Python acorn

**Fully ported**:

- Authentication (HTTP POST `/api/acorn/auth` → token → WS connect)
- Auto-reconnect with exponential backoff + outbox flush
- Heartbeat (15s WS ping) + disconnect detection
- Chat send + streaming deltas + thinking-delta + tool-status indicators
- Chat history replay on session join
- Plan mode with PLAN_PREFIX constant + PLAN_READY marker detection
- Plan approval modal (Execute / Revise / Cancel) + plan save to
  `.acorn/plans/plan-<ts>.md`
- Plan save silent-exception fix (prints to stderr instead of swallowing)
- QUESTIONS: prose parser with single-select `[a / b]`, multi-select `{a / b}`,
  and open-ended formats (parity with `acorn/questions.py`)
- Composition fix for plan-mode + QUESTIONS: in the same response (the
  Python 277fc8c bug — questions run first, plan modal surfaces after answers)
- Structured `ask_user` tool (new SPORE capability) with its own picker modal
- Local tool execution:
  - `read_file` / `write_file` / `edit_file` (cwd-sandboxed)
  - `glob` / `grep` (walkdir with noise-dir skipping, 500/200 result caps)
  - `exec` (inactivity timeout, dangerous-pattern block, sensitive-path block,
    output truncation, log file in `.acorn/logs/`)
- Permission system: four modes (auto/ask/locked/yolo), dangerous-pattern
  heuristics, session allow rules ("exec:git*", "write_file:src/*"),
  structured approval modal with "Allow similar" option for non-dangerous
- Session persistence — per-session JSONL at `~/.acorn/sessions/<safeid>.jsonl`
  compatible with the Python format (same character substitutions, same
  `_meta` header); `/sessions` lists, `/resume` loads
- Diagnostic log at `~/.acorn/logs/<ts>_<safeid>.log` mirroring session_log.py
- Context gathering on first message (OS, git branch, top-level project
  markers, available CLIs)
- Themes: dark, oak, forest, oled, light (runtime switchable via `/theme`)
- Companion observer protocol — outbound: `state:questions`,
  `plan:show-approval`, `plan:decided`, `plan:set-mode`, `interactive:resolved`,
  `perm:current-mode`. Inbound: `plan:decision`, `perm:query`, `perm:set-mode`
- Delegation policies: /mode pipes through the `tools.Executor.Delegation`
  field; `delegate_task` inputs are gated before the server sees them
- Slash command set matching Python's `constants.py:SLASH_COMMANDS`

**Stubbed / not yet ported** (clearly flagged at runtime):

- `/bg run` — background process manager (Python has a full ProcessManager
  with persistence; Go port refuses and suggests tmux/screen)
- `/update` install — only `/update check` works; installing means
  re-downloading the binary manually
- `/test` test harness — internal to acorn
- Code-viewer side panel — paths logged inline instead
- Subagent activity panel

## Layout

```
go/
├── Makefile                     build + cross-compile targets
├── README.md                    this file
├── install.sh                   prebuilt-binary installer
├── go.mod / go.sum              module + locked deps
├── cmd/acorn/main.go            entry point
└── internal/
    ├── config/config.go         [connection]+[display] TOML loader, last_session
    ├── conn/ws.go               auth + WS + reconnect + outbox
    ├── proto/messages.go        server message shapes
    ├── sessionlog/
    │   ├── writer.go            ~/.acorn/sessions/*.jsonl writer + listings
    │   └── debuglog.go          ~/.acorn/logs/*.log verbose logger
    ├── tools/
    │   ├── executor.go          dispatch, delegation policing, hooks
    │   ├── fileops.go           read/write/edit_file with cwd sandbox
    │   ├── search.go            glob + grep
    │   └── shell.go             exec with timeout/safety/log
    └── app/
        ├── model.go             Bubble Tea Model + init wiring
        ├── update.go            Update() — keystrokes, frames, slash
        ├── view.go              chat rendering, header, footer
        ├── themes.go            dark/oak/forest/oled/light palettes
        ├── context.go           first-message context gathering
        ├── session.go           sessionID compute, git helpers
        ├── permissions.go       TUIPerms (impl Permissions interface)
        ├── permmodal.go         per-tool approval modal
        ├── questions.go         QUESTIONS: parser + prose picker + ask_user
        ├── plan.go              plan approval modal + savePlan
        └── updater.go           /update check against GitHub releases
```

## Protocol compatibility

The Go port speaks the same protocol as the Python CLI and connects to the
same SPORE server endpoints. You can switch between the two CLIs on the
same machine — they share the `~/.acorn/config.toml`, `~/.acorn/sessions/`,
`~/.acorn/logs/`, and `~/.acorn/last_session` files. `/sessions` in either
CLI lists the same history.
