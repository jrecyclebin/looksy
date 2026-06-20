package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	flagLLM    string
	flagModel  string
	flagSearch string
)

// loadConfig sources config.env from the OS-appropriate config directory.
// Linux: ~/.config/looksy/config.env
// macOS: ~/Library/Application Support/looksy/config.env
// Windows: %AppData%\looksy\config.env
// Override with LOOKSY_CONFIG env var. Lines starting with # are comments,
// blank lines are skipped.
func loadConfig() {
	path := os.Getenv("LOOKSY_CONFIG")
	if path == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return // can't determine config dir, that's fine
		}
		path = filepath.Join(configDir, "looksy", "config.env")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // no config file is fine
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// Don't override env vars already set (CLI env takes precedence over config)
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

func main() {
	loadConfig()

	// Determine command: "go", "models", or "help" (default: go)
	command := "go"
	args := os.Args[1:]

	for i, arg := range args {
		if arg == "go" || arg == "models" || arg == "help" {
			command = arg
			args = append(args[:i], args[i+1:]...)
			break
		}
	}

	// Parse remaining args as flags
	fs := flag.NewFlagSet("looksy", flag.ExitOnError)
	fs.StringVar(&flagLLM, "l", "", "LLM tool to use (pi, claude, gemini, ollama, opencode)")
	fs.StringVar(&flagModel, "m", "", "model name or alias to pass to the LLM tool")
	fs.StringVar(&flagSearch, "s", "", "search tool to hint in prompt (rg, grep)")
	fs.Parse(args)

	// Resolve effective values: flag > env > config > built-in default
	llm := coalesce(flagLLM, os.Getenv("LOOKSY_LLM"), "pi")
	search := coalesce(flagSearch, os.Getenv("LOOKSY_SEARCH"), "rg")
	model := coalesce(flagModel, os.Getenv("LOOKSY_MODEL"), "")

	switch command {
	case "models":
		if err := listModels(llm); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "help":
		printHelp()
	default: // "go"
		cmdGo(llm, model, search, fs.Args())
	}
}

func printHelp() {
	fmt.Fprintln(os.Stderr, "Usage: looksy <command> [flags] [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  go       Look at a codebase and find relevant references (default)")
	fmt.Fprintln(os.Stderr, "  models   List available models for the LLM tool")
	fmt.Fprintln(os.Stderr, "  help     Show this help message")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  -l string")
	fmt.Fprintln(os.Stderr, "        LLM tool to use (pi, claude, gemini, ollama, opencode)")
	fmt.Fprintln(os.Stderr, "  -m string")
	fmt.Fprintln(os.Stderr, "        model name or alias to pass to the LLM tool")
	fmt.Fprintln(os.Stderr, "  -s string")
	fmt.Fprintln(os.Stderr, "        search tool to hint in prompt (rg, grep)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Config: <os config dir>/looksy/config.env (or set LOOKSY_CONFIG)")
}

func cmdGo(llm, model, search string, queryArgs []string) {
	if len(queryArgs) == 0 {
    printHelp()
		os.Exit(1)
	}
	query := strings.Join(queryArgs, " ")

	prompt := buildPrompt(search)
	output, err := callLLM(llm, model, prompt, query)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	processOutput(os.Stdout, output)
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// --- Prompt ---

func buildPrompt(searchTool string) string {
	searchSection := searchSectionRG
	if searchTool == "grep" {
		searchSection = searchSectionGrep
	}
	return promptPreamble + searchSection + promptPostamble
}

const promptPreamble = "You are Looksy, an agent used to look at the current codebase and find all\n" +
	"the relevant code references needed for the task. You do the footwork, to\n" +
	"walk through all the code, to map it well enough, to provide a guide that can be\n" +
	"used to make the edits needed or for reading up on the basis for any new work.\n\n"

const searchSectionRG = "Use ripgrep (rg) for file searches. Some quick examples:\n\n" +
	"```bash\n" +
	"rg \"foo|bar\" # OR: doesn't require a backslash\n" +
	"rg -i \"foo.*bar\" # case-insensitive search for both\n" +
	"rg -F \"Main\" -g \"*.go\" -g \"!*_test.go\" # search only Go files (but not tests) for exact string \"Main\"\n" +
	"rg \"func \\w+\\s*\\(\" -g \"*.go\" # find all function definitions in Go files\n" +
	"rg '\"[^\"]*\"' -g \"*.js\" # find all string literals in JavaScript files\n" +
	"```\n\n" +
	"Other flags:\n" +
	"- `-c` for file list with total count of matches\n" +
	"- `-C 2` for two lines of surrounding context\n\n"

const searchSectionGrep = "Use grep for file searches. Some quick examples:\n\n" +
	"```bash\n" +
	"grep -rn \"foo\\|bar\" . # OR: requires backslash escaping\n" +
	"grep -ri \"foo.*bar\" . # case-insensitive search for both\n" +
	"grep -rnF \"Main\" --include=\"*.go\" --exclude=\"*_test.go\" . # search only Go files (not tests) for exact string \"Main\"\n" +
	"grep -rn \"func [a-zA-Z_]\\+\\s*(\" --include=\"*.go\" . # find all function definitions in Go files\n" +
	"grep -rn '\"[^\"]*\"' --include=\"*.js\" . # find all string literals in JavaScript files\n" +
	"```\n\n" +
	"Other flags:\n" +
	"- `-c` for count of matches per file\n" +
	"- `-C 2` for two lines of surrounding context\n\n"

const promptPostamble = "Respond to the user's prompt with two XML blocks: 1) a <summary> block,\n" +
	"containing an overview of the discoveries and any notes; and 2) a <references> block,\n" +
	"containing the specific code references (one per line) using the format\n" +
	"`path/to/file.ext:line-range`. Always use this exact format for the code\n" +
	"references in this block.\n\n" +
	"As an example:\n\n" +
	"```\n" +
	"<references>\n" +
	"handler.go:782-920 — `handleTaskExec` (full execution pipeline: multipart/JSON parsing, env vars, file uploads, shell exec, MCP-format response)\n" +
	"handler.go:1978-2020 — `verifyTaskToken` (validates Bearer token against referenced auth service)\n" +
	"web/frbr.js:250-310 — Tasks routing in ServiceProxy (`listTools`/`callTool` detect `tasks://` URL, `#callTask` for multipart support)\n" +
	"</references>\n" +
	"```\n\n" +
	"Try to keep references focused to less than 200 lines. Unless extensive context\n" +
	"truly needed."

// --- LLM invocation ---

func callLLM(tool, model, systemPrompt, query string) (string, error) {
	name, args := llmCommand(tool, model, systemPrompt, query)
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running %s: %w", tool, err)
	}
	return string(out), nil
}

func llmCommand(tool, model, systemPrompt, query string) (string, []string) {
	switch tool {
	case "claude":
		args := []string{"--system-prompt", systemPrompt, "-p", query}
		if model != "" {
			args = append([]string{"--model", model}, args...)
		}
		return "claude", args
	case "gemini":
		args := []string{"-p", systemPrompt + "\n\n" + query}
		if model != "" {
			args = append([]string{"--model", model}, args...)
		}
		return "gemini", args
	case "ollama":
		if model == "" {
			model = "llama3"
		}
		return "ollama", []string{"run", model, systemPrompt + "\n\n" + query}
	case "opencode":
		args := []string{"run"}
		if model != "" {
			args = append(args, "--model", model)
		}
		args = append(args, systemPrompt+"\n\n"+query)
		return "opencode", args
	default:
		args := []string{"--system-prompt", systemPrompt, "-p", query}
		if model != "" {
			args = append([]string{"--model", model}, args...)
		}
		return "pi", args
	}
}

func listModels(tool string) error {
	var name string
	var args []string
	switch tool {
	case "claude":
		fmt.Fprintln(os.Stderr, "claude does not support --models. Use 'claude --model <alias>' with aliases like sonnet, opus, haiku.")
		return nil
	case "gemini":
		fmt.Fprintln(os.Stderr, "gemini does not support --models. Use 'gemini -m <model>' with a model name.")
		return nil
	case "ollama":
		name = "ollama"
		args = []string{"list"}
	case "opencode":
		name = "opencode"
		args = []string{"models"}
	default:
		name = "pi"
		args = []string{"--list-models"}
	}
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// --- Output processing (the spy logic) ---

var refLineRe = regexp.MustCompile(`^[\w./@-]+:\d+-\d+`)

func processOutput(w io.Writer, input string) {
	summary := extractBlock(input, "summary")
	if summary != "" {
		fmt.Fprintln(w, summary)
	}

	fmt.Fprintln(w, "<references>")

	refsBlock := extractBlock(input, "references")
	if refsBlock != "" {
		scanner := bufio.NewScanner(strings.NewReader(refsBlock))
		for scanner.Scan() {
			line := scanner.Text()
			if !refLineRe.MatchString(line) {
				continue
			}
			expandReference(w, line)
		}
	}

	fmt.Fprintln(w, "</references>")
}

func extractBlock(input, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(input, open)
	if start == -1 {
		return ""
	}
	start += len(open)
	end := strings.Index(input, close)
	if end == -1 || end < start {
		return ""
	}
	return strings.TrimSpace(input[start:end])
}

func expandReference(w io.Writer, line string) {
	ref := strings.SplitN(line, " ", 2)[0]
	comment := ""
	if parts := strings.SplitN(line, " ", 2); len(parts) > 1 {
		comment = " " + parts[1]
	}

	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return
	}
	path := parts[0]
	rangeStr := parts[1]

	rangeParts := strings.SplitN(rangeStr, "-", 2)
	if len(rangeParts) != 2 {
		return
	}
	start, err := strconv.Atoi(rangeParts[0])
	if err != nil {
		return
	}
	end, err := strconv.Atoi(rangeParts[1])
	if err != nil {
		return
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Fprintf(w, "=== %s%s  (FILE NOT FOUND) ===\n\n", ref, comment)
		return
	}

	fmt.Fprintf(w, "=== %s%s\n", ref, comment)
	printFileRange(w, path, start, end)
	fmt.Fprintln(w)
}

func printFileRange(w io.Writer, path string, start, end int) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(w, "  (error reading file: %v)\n", err)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineno := 0
	for scanner.Scan() {
		lineno++
		if lineno < start {
			continue
		}
		if lineno > end {
			break
		}
		fmt.Fprintln(w, scanner.Text())
	}
}
