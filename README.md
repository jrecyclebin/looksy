# Looksy

A simple CLI tool to quickly research a task and give you PERFECT context.

How does it do this? It uses one of your existing tools (Claude, Pi, Opencode,
etc) to investigate a coding tasks and asks for a list of code references -
files and line number blocks.

Looksy then reads those EXACT lines of code expands them fully in its response.
This means that the agent doing the coding can have the pieces it needs without
having a bunch of irrelevant code in the way.

  $ looksy "Add Codex support."

  > **What needs to happen to add Codex support:**
  > [...Steps the LLM suggests...]
  >
  > <references>
  > === main.go:75-77 — `-l` flag definition with current help text listing `(pi, claude, gemini, opencode)` — needs `codex` added
	> fs := flag.NewFlagSet("looksy", flag.ExitOnError)
	> fs.StringVar(&flagLLM, "l", "", "LLM tool to use (pi, claude, gemini, opencode)")
	> fs.StringVar(&flagModel, "m", "", "model name or alias to pass to the LLM tool")
  >
  > === main.go:187-195 — `callLLM` function — the `codex exec` command may need special output handling (e.g., `--output-last-message`)
	> "references in this block.\n\n" +
	> "As an example:\n\n" +
	> "```\n" +
	> "<references>\n" +
	> "handler.go:782-920 — `handleTaskExec` (full execution pipeline: multipart/JSON parsing, env vars, file uploads, shell exec, MCP-format response)\n" +
	> "handler.go:1978-2020 — `verifyTaskToken` (validates Bearer token against referenced auth service)\n" +
	> "web/frbr.js:250-310 — Tasks routing in ServiceProxy (`listTools`/`callTool` detect `tasks://` URL, `#callTask` for multipart support)\n" +
	> "</references>\n" +
	> "```\n\n" +
  > [...3 other references...]
  > </references>

Because it uses your existing tools, there's no need to do auth or config for
Looksy at all - it'll use what it can to do its job. (For example, if all you
have is Claude Code installed, it'll default to using Haiku.)

You can also use flags to select the LLM and model you want to use:

  $ looksy -l ollama -m "qwen3.5:2b" "Add Codex support."

If you need to list the available models for a given tool, you can do that too:

  $ looksy -l ollama models

## Quick Start

Download a release - it's just a zip file with `looksy` inside.

You'll need Claude, Codex, Gemini, Pi or Opencode all set up already. Looksy
just calls out to it and uses your existing setup.

> [!NOTE]
> You can install using [mise](https://mise.jdx.dev). Just use `mise use
> github:jrecyclebin/looksy`. Then run `looksy` to bring up the app.

For odd architectures - like Windows or Linux on ARM - you need to build from
source.

## Configuration

Jabsco reads `config.env` from its config directory on startup...

## Building from Source

Install [mise](https://mise.jdx.dev/installing-mise.html).

Then: `mise build`.
