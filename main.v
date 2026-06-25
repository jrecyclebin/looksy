module main

import os
import strconv
import strings
import time

// loadConfig sources config.env from the OS-appropriate config directory.
// Linux: ~/.config/looksy/config.env
// macOS: ~/Library/Application Support/looksy/config.env
// Windows: %AppData%\looksy\config.env
// Override with LOOKSY_CONFIG env var. Lines starting with # are comments,
// blank lines are skipped.
fn load_config() {
	mut path := os.getenv('LOOKSY_CONFIG')
	if path == '' {
		config_base := os.config_dir() or { return } // can't determine config dir, that's fine
		path = os.join_path(config_base, 'looksy', 'config.env')
	}
	data := os.read_file(path) or { return } // no config file is fine
	for line in data.split_into_lines() {
		l := line.trim_space()
		if l == '' || l.starts_with('#') {
			continue
		}
		eq := l.index('=') or { continue }
		key := l[..eq].trim_space()
		value := l[eq + 1..].trim_space()
		// Don't override env vars already set (CLI env takes precedence over config)
		if os.getenv(key) == '' {
			os.setenv(key, value, true)
		}
	}
}

fn main() {
	load_config()

	// Determine command: "go", "models", or "help" (default: go)
	mut command := 'go'
	mut args := os.args[1..].clone()
	for i, a in args {
		if a in ['go', 'models', 'help'] {
			command = a
			args.delete(i)
			break
		}
	}

	// Parse leading flags (-l, -m, -s); the first non-flag token begins the query.
	mut flag_llm := ''
	mut flag_model := ''
	mut flag_search := ''
	mut i := 0
	for i < args.len {
		a := args[i]
		if !a.starts_with('-') || a == '-' {
			break
		}
		if a == '--' {
			i++
			break
		}
		mut fname := a
		mut fval := ''
		mut have_val := false
		if eq := a.index('=') {
			fname = a[..eq]
			fval = a[eq + 1..]
			have_val = true
		}
		if fname !in ['-l', '-m', '-s'] {
			eprintln('flag provided but not defined: ${fname}')
			print_help()
			exit(2)
		}
		if !have_val {
			if i + 1 >= args.len {
				eprintln('flag needs an argument: ${fname}')
				exit(2)
			}
			fval = args[i + 1]
			i += 2
		} else {
			i++
		}
		match fname {
			'-l' { flag_llm = fval }
			'-m' { flag_model = fval }
			'-s' { flag_search = fval }
			else {}
		}
	}
	rest := args[i..].clone()

	// Resolve effective values: flag > env > config > built-in default
	llm := coalesce(flag_llm, os.getenv('LOOKSY_LLM'), 'pi')
	model := coalesce(flag_model, os.getenv('LOOKSY_MODEL'), '')
	mut search := coalesce(flag_search, os.getenv('LOOKSY_SEARCH'), 'rg')

	// Fall back to grep if the chosen search tool isn't on PATH
	if _ := os.find_abs_path_of_executable(search) {
		// found, keep as-is
	} else {
		if search != 'grep' {
			eprintln('warning: "${search}" not found on PATH, falling back to grep')
		}
		search = 'grep'
	}

	match command {
		'models' {
			list_models(llm) or {
				eprintln('Error: ${err}')
				exit(1)
			}
		}
		'help' {
			print_help()
		}
		else { // "go"
			cmd_go(llm, model, search, rest)
		}
	}
}

fn print_help() {
	eprintln('Usage: looksy <command> [flags] [args]')
	eprintln('')
	eprintln('Commands:')
	eprintln('  go       Look at a codebase and find relevant references (default)')
	eprintln('  models   List available models for the LLM tool')
	eprintln('  help     Show this help message')
	eprintln('')
	eprintln('Flags:')
	eprintln('  -l string')
	eprintln('        LLM tool to use (pi, claude, gemini, ollama, opencode)')
	eprintln('  -m string')
	eprintln('        model name or alias to pass to the LLM tool')
	eprintln('  -s string')
	eprintln('        search tool to hint in prompt (rg, grep)')
	eprintln('')
	eprintln('Config: <os config dir>/looksy/config.env (or set LOOKSY_CONFIG)')
}

fn cmd_go(llm string, model string, search string, query_args []string) {
	if query_args.len == 0 {
		print_help()
		exit(1)
	}
	query := query_args.join(' ')

	prompt := build_prompt(search)
	output := call_llm(llm, model, prompt, query) or {
		eprintln('Error: ${err}')
		exit(1)
	}

	print(process_output(output))
}

fn coalesce(vals ...string) string {
	for v in vals {
		if v != '' {
			return v
		}
	}
	return ''
}

// --- Prompt ---

fn build_prompt(search_tool string) string {
	search_section := if search_tool == 'grep' { search_section_grep } else { search_section_rg }
	cwd := os.getwd()
	return prompt_preamble.replace('CWD', cwd) + search_section + prompt_postamble
}

const prompt_preamble = 'You are Looksy, an agent used to look at the files in
the current working directory (CWD) and find all the relevant code references.
Keep your searches and file reads limited to this working directory.
You are researching and planning for a specific prompt. You do the footwork, to
walk through this directory, to map it well enough, to provide a guide that can be
used to make the edits needed or for reading up on the basis for any new work.

'

const search_section_rg = 'Use ripgrep (rg) for file searches. Some quick examples:

```bash
rg "foo|bar" # OR: doesn\'t require a backslash
rg -i "foo.*bar" # case-insensitive search for both
rg -F "Main" -g "*.go" -g "!*_test.go" # search only Go files (but not tests) for exact string "Main"
rg "func \\w+\\s*\\(" -g "*.go" # find all function definitions in Go files
rg \'"[^"]*"\' -g "*.js" # find all string literals in JavaScript files
```

Other flags:
- `-c` for file list with total count of matches
- `-C 2` for two lines of surrounding context

'

const search_section_grep = 'Use grep for file searches. Some quick examples:

```bash
grep -rn "foo\\|bar" . # OR: requires backslash escaping
grep -ri "foo.*bar" . # case-insensitive search for both
grep -rnF "Main" --include="*.go" --exclude="*_test.go" . # search only Go files (not tests) for exact string "Main"
grep -rn "func [a-zA-Z_]\\+\\s*(" --include="*.go" . # find all function definitions in Go files
grep -rn \'"[^"]*"\' --include="*.js" . # find all string literals in JavaScript files
```

Other flags:
- `-c` for count of matches per file
- `-C 2` for two lines of surrounding context

'

const prompt_postamble = "Respond to the user's prompt with your findings, and include specific code
references using the format `path/to/file.ext:line-range` — one per line,
each optionally followed by a dash and a description. Surround them with a
code block and keep them neatly arranged.

For example:

```
handler.go:782-920 — `handleTaskExec` (full execution pipeline)
handler.go:1978-2020 — `verifyTaskToken` (validates Bearer token)
web/frbr.js:250-310 — Tasks routing in ServiceProxy
```

Try to keep references focused to less than 200 lines. Unless you feel
extensive context is truly needed.

Make no edits to the code - don't touch any files - you are just in planning
mode for this - just grab the file references and make a list for me.
"

const actual_prompt = 'Here is the prompt you are researching:

PROMPT

When you are done, reply with your findings and list of file references.
'

// --- LLM invocation ---

fn call_llm(tool string, model string, system_prompt string, query string) !string {
	name, args := llm_command(tool, model, system_prompt, query)
	exe := os.find_abs_path_of_executable(name) or {
		return error('running ${tool}: ${name} not found on PATH')
	}
	mut p := os.new_process(exe)
	p.set_args(args)
	p.set_redirect_stdio()
	p.run()
	// Capture stdout into our buffer while forwarding the tool's stderr live.
	// Polling with is_pending keeps a large response from filling the pipe and
	// deadlocking before the child exits.
	mut out := strings.new_builder(4096)
	for p.is_alive() {
		if p.is_pending(.stdout) {
			out.write_string(p.stdout_read())
		}
		if p.is_pending(.stderr) {
			eprint(p.stderr_read())
		}
		time.sleep(2 * time.millisecond)
	}
	out.write_string(p.stdout_slurp())
	eprint(p.stderr_slurp())
	p.wait()
	code := p.code
	p.close()
	if code != 0 {
		return error('running ${tool}: exited with status ${code}')
	}
	return out.str()
}

fn llm_command(tool string, model string, system_prompt string, query string) (string, []string) {
	blockquoted := '> ' + query.replace('\n', '\n> ')
	full_prompt := actual_prompt.replace('PROMPT', blockquoted)
	match tool {
		'claude' {
			m := if model == '' { 'haiku' } else { model }
			return 'claude', ['--model', m, '--system-prompt', system_prompt, '-p', full_prompt]
		}
		'gemini' {
			mut args := []string{}
			if model != '' {
				args << '--model'
				args << model
			}
			args << '-p'
			args << system_prompt + '\n\n' + full_prompt
			return 'gemini', args
		}
		'ollama' {
			m := if model == '' { 'llama3' } else { model }
			return 'ollama', ['run', m, system_prompt + '\n\n' + full_prompt]
		}
		'opencode' {
			mut args := ['run']
			if model != '' {
				args << '--model'
				args << model
			}
			args << system_prompt + '\n\n' + full_prompt
			return 'opencode', args
		}
		else {
			mut args := []string{}
			if model != '' {
				args << '--model'
				args << model
			}
			args << '--exclude-tools'
			args << 'edit,write'
			args << '--system-prompt'
			args << system_prompt
			args << '-p'
			args << full_prompt
			return 'pi', args
		}
	}
}

fn list_models(tool string) ! {
	match tool {
		'claude' {
			eprintln("claude does not support --models. Use 'claude --model <alias>' with aliases like sonnet, opus, haiku.")
		}
		'gemini' {
			eprintln("gemini does not support --models. Use 'gemini -m <model>' with a model name.")
		}
		'ollama' {
			if run_command('ollama', ['list']) != 0 {
				return error('ollama list failed')
			}
		}
		'opencode' {
			if run_command('opencode', ['models']) != 0 {
				return error('opencode models failed')
			}
		}
		else {
			if run_command('pi', ['--list-models']) != 0 {
				return error('pi --list-models failed')
			}
		}
	}
}

// run_command runs a child process with inherited stdio (its output streams
// straight to our terminal) and returns its exit code.
fn run_command(name string, args []string) int {
	exe := os.find_abs_path_of_executable(name) or {
		eprintln('${name} not found on PATH')
		return 127
	}
	mut p := os.new_process(exe)
	p.set_args(args)
	p.run()
	p.wait()
	code := p.code
	p.close()
	return code
}

// --- Output processing (the spy logic) ---

struct RefMatch {
	text  string // the matched reference, e.g. "handler.go:782-920"
	after string // remainder of the line following the match
}

fn process_output(input string) string {
	mut sb := strings.new_builder(input.len + 256)
	// Print the LLM's response as-is
	sb.write_string(input)

	// Scan the entire response for file references and expand them
	mut refs := []string{}
	for line in input.split_into_lines() {
		for m in find_refs(line) {
			comment := extract_comment(m.after)
			refs << m.text + comment
		}
	}

	if refs.len > 0 {
		sb.write_string('\n---\nFILE REFERENCES\nAll file content below is current - is the exact file data from each chunk. Provided here to limit the need for file reads.\n---\n')
		for ref in refs {
			sb.write_string(expand_reference(ref))
		}
	}
	return sb.str()
}

// find_refs locates file references anywhere in a line, e.g.:
//
//	main.go:77-90
//	src/handler.go:42
//	**`web/frbr.js:250-310`** - some description
//
// matching the shape [\w./@-]+:\d+(-\d+)? — a path, a colon, a line number,
// and an optional -end range.
fn find_refs(line string) []RefMatch {
	mut out := []RefMatch{}
	mut i := 0
	for i < line.len {
		e := match_ref_at(line, i)
		if e > i {
			out << RefMatch{
				text:  line[i..e]
				after: line[e..]
			}
			i = e
		} else {
			i++
		}
	}
	return out
}

// match_ref_at returns the end index of a reference starting at `start`, or -1
// if no reference begins there.
fn match_ref_at(s string, start int) int {
	if start >= s.len || !is_ref_char(s[start]) {
		return -1
	}
	mut j := start
	for j < s.len && is_ref_char(s[j]) {
		j++
	}
	if j >= s.len || s[j] != `:` {
		return -1
	}
	j++ // consume ':'
	d := j
	for j < s.len && is_digit(s[j]) {
		j++
	}
	if j == d {
		return -1 // no line number after the colon
	}
	// optional -[0-9]+ range end
	if j < s.len && s[j] == `-` {
		k := j + 1
		mut m := k
		for m < s.len && is_digit(s[m]) {
			m++
		}
		if m > k {
			j = m
		}
	}
	return j
}

fn is_ref_char(c u8) bool {
	return (c >= `a` && c <= `z`) || (c >= `A` && c <= `Z`) || (c >= `0` && c <= `9`)
		|| c == `_` || c == `.` || c == `/` || c == `@` || c == `-`
}

fn is_digit(c u8) bool {
	return c >= `0` && c <= `9`
}

// extract_comment looks for a dash separator ( - or —) in the text after a
// file reference and returns everything following it, trimmed, with a leading
// space. Returns an empty string if no dash separator is found.
fn extract_comment(after string) string {
	// Try em-dash first, then space-hyphen-space
	for sep in [' — ', ' - '] {
		idx := after.index(sep) or { continue }
		return ' ' + after[idx + sep.len..].trim_space()
	}
	return ''
}

fn expand_reference(ref_with_comment string) string {
	// Split ref from comment (comment was appended after a space)
	mut ref := ref_with_comment
	mut comment := ''
	if sp := ref_with_comment.index(' ') {
		ref = ref_with_comment[..sp]
		comment = ref_with_comment[sp..] // includes leading space
	}

	parts := ref.split_nth(':', 2)
	if parts.len != 2 {
		return ''
	}
	path := parts[0]
	range_str := parts[1]

	mut start := 0
	mut end := 0
	if range_str.contains('-') {
		rp := range_str.split_nth('-', 2)
		start = strconv.atoi(rp[0]) or { return '' }
		end = strconv.atoi(rp[1]) or { return '' }
	} else {
		start = strconv.atoi(range_str) or { return '' }
		end = start
	}

	if !os.exists(path) {
		return '=== ${ref}${comment}  (FILE NOT FOUND) ===\n\n'
	}

	return '=== ${ref}${comment}\n' + read_file_range(path, start, end) + '\n'
}

fn read_file_range(path string, start int, end int) string {
	lines := os.read_lines(path) or { return '  (error reading file: ${err})\n' }
	mut sb := strings.new_builder(256)
	for idx, line in lines {
		lineno := idx + 1
		if lineno < start {
			continue
		}
		if lineno > end {
			break
		}
		sb.writeln(line)
	}
	return sb.str()
}
