# Acorn

CLI coding assistant that connects to an [Anima](https://github.com/Klace/Anima-AI) agent. Like Claude Code, but with persistent memory across projects via Anima's knowledge graph.

## Features

- **REPL + one-shot mode** — `acorn` for interactive, `acorn "fix the tests"` for quick tasks
- **Local tool execution** — file reads/writes, shell commands, and search run on your machine
- **Persistent memory** — knowledge learned in one project carries over to future work
- **Multi-user** — each developer gets their own identity and session history
- **Permission system** — auto-approves reads, prompts for writes and shell commands
- **Rich terminal UI** — streaming markdown, syntax highlighting, diffs, tool panels

## Install

```bash
pip install -e .
```

## Setup

First run prompts for server connection:

```
$ acorn
Welcome to Acorn! Let's connect to your Anima agent.
Anima host [localhost]: 192.168.1.78
Anima web port [18810]: 18810
Your username: yam
Team key (ANIMA_ACORN_KEY from server .env): acorn_sk_...
```

## Usage

```bash
# Interactive REPL
acorn

# One-shot
acorn "what does this project do?"

# Override connection
acorn --host 10.0.0.5 --port 18810 --user dan "fix the auth bug"
```

### Slash Commands

| Command | Action |
|---------|--------|
| `/help` | Show commands |
| `/quit` | Exit |
| `/clear` | Clear session history |
| `/stop` | Abort current generation |
| `/status` | Connection info |
| `/context` | Show project context |
| `/tree` | Show directory tree |
| `/init` | Create ACORN.md template |
| `/approve-all` | Auto-approve all tools |

## Server Requirements

Requires Anima with Acorn support (PR #4). Add to your agent's `.env`:

```env
ANIMA_ACORN_KEY=your-team-secret-here
```
