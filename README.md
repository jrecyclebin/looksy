# Looksy

A simple CLI tool to quickly research a task and give you PERFECT context.

How does it do this? It uses one of your existing tools (Claude, Pi, Opencode,
etc) to investigate a coding task and asks for a list of code references —
files and line number blocks.

Looksy then reads those EXACT lines of code and expands them fully in its
response. This means that the agent doing the coding can have the pieces it
needs without a bunch of irrelevant code in the way.

  $ looksy "Add Codex support."

  > **What needs to happen to add Codex support:**
  > [...Steps the LLM suggests...]
  >
  > main.go:75-77 — `-l` flag definition with current help text
  > [...more references from the LLM...]
  >
  > ---
  >
  > === main.go:75-77 — `-l` flag definition with current help text
  > fs := flag.NewFlagSet("looksy", flag.ExitOnError)
  > fs.StringVar(&flagLLM, "l", "", "LLM tool to use (pi, claude, gemini, ollama, opencode)")
  > fs.StringVar(&flagModel, "m", "", "model name or alias to pass to the LLM tool")
  >
  > === main.go:207-240 — `llmCommand` with per-tool flag construction
  > [...expanded file contents...]
  >
  > [...3 other expanded references...]

The LLM's full response is printed as-is, then Looksy appends a `---`
separator and expands every file reference it found into the actual source code.

Because it uses your existing tools, there's no need to do auth or config for
Looksy at all — it'll use what it can to do its job. (For example, if all you
have is Claude Code installed, it'll default to using Haiku.)

You can also use flags to select the LLM and model you want to use:

  $ looksy -l ollama -m "qwen3.5:2b" "Add Codex support."

If you need to list the available models for a given tool, you can do that too:

  $ looksy -l ollama models

## Using in Prompts

You can easily add Looksy to any prompt where you are kicking off a round of
work:

> Add two new features: 1) an audit log for tracking calls to the API  and
> 2) support for automatically refreshing expired tokens when you get a 401
> response from the control panel UI.
>
> Before doing this work, build a map of the codebase using this command:
> `looksy -l pi "[Put the above prompt here]"`

A prompt like this can then be bottled up into a reusable command in Claude.
For example, save this at `~/.claude/commands/looksy.md`:

> You have been tasked with the following prompt:
>
> <user-prompt>
> $ARGUMENTS
> </user-prompt>
>
> Before running that prompt, give it to Looksy to build a map of file
> references. Use the following command: `looksy -l claude "[The above prompt]"`

## Quick Start

Download a release - it's just a zip file with `looksy` inside.

You'll need Pi, Claude, Gemini, Ollama, or Opencode set up already. Looksy
just calls out to whichever one you choose and uses your existing setup.

> [!NOTE]
> You can install using [mise](https://mise.jdx.dev). Just use `mise use
> github:jrecyclebin/looksy`. Then run `looksy` to bring up the app.

For odd architectures - like Windows or Linux on ARM - you need to build from
source.

## Configuration

Looksy reads `config.env` from its config directory on startup:

- **Linux:** `~/.config/looksy/config.env`
- **macOS:** `~/Library/Application Support/looksy/config.env`
- **Windows:** `%AppData%\looksy\config.env`

Override the config path with the `LOOKSY_CONFIG` environment variable.
Lines starting with `#` are comments; blank lines are skipped.

```env
# Which LLM tool to use (pi, claude, gemini, ollama, opencode)
LOOKSY_LLM=pi

# Model name or alias to pass to the LLM tool
LOOKSY_MODEL=

# Search tool to hint in prompt (rg, grep)
LOOKSY_SEARCH=rg
```

Config values don't override existing environment variables — so you can set
`LOOKSY_LLM=claude` in your shell and the config file won't clobber it.

Priority chain: **flag > env var > config.env > built-in default**.

## Building from Source

Install [mise](https://mise.jdx.dev/installing-mise.html).

Then: `mise build`.
