module main

import os

fn test_find_refs_match() {
	cases := {
		'handler.go:782-920 — desc':                            true
		'web/frbr.js:250-310 — desc':                           true
		'some/path/file.ts:1-5':                                true
		'main.go:77':                                           true
		'not a reference':                                      false
		'  handler.go:782-920':                                 true // leading space is fine — refs found anywhere
		'## 2. **`main.go:77`** - Update model flag help text': true
	}
	for line, want in cases {
		got := find_refs(line).len > 0
		assert got == want, 'find_refs(${line}).len > 0 = ${got}, want ${want}'
	}
}

fn test_extract_comment() {
	cases := {
		' — Update model flag help text': ' Update model flag help text'
		' - some description':            ' some description'
		'** - Update the thing':          ' Update the thing'
		'`**':                            ''
		'':                               ''
	}
	for after, want in cases {
		got := extract_comment(after)
		assert got == want, 'extract_comment(${after}) = ${got}, want ${want}'
	}
}

fn test_process_output_with_fake_refs() {
	// Create a temp file so expand_reference can actually read it
	tmp := os.join_path(os.temp_dir(), 'looksy-test-${os.getpid()}.go')
	mut content := ''
	for i in 1 .. 21 {
		content += 'line ${i}\n'
	}
	os.write_file(tmp, content) or { assert false, 'could not write temp file' }
	defer {
		os.rm(tmp) or {}
	}

	input := 'Here is what I found:\n\n' + '${tmp}:3-7 — test range\n' +
		'nonexistent.go:1-5 — missing file\n'

	output := process_output(input)

	// The full LLM response should be printed as-is
	assert output.contains('Here is what I found:'), 'output missing LLM response text'
	// The separator should appear before expanded refs
	assert output.contains('\n---\n'), 'output missing separator before expanded refs'
	assert output.contains('line 3') && output.contains('line 7'), 'output missing expanded file range'
	assert output.contains('FILE NOT FOUND'), 'output missing FILE NOT FOUND for nonexistent file'
}

fn test_build_prompt() {
	rg := build_prompt('rg')
	grep := build_prompt('grep')

	assert rg.contains('ripgrep'), 'rg prompt should mention ripgrep'
	assert grep.contains('grep'), 'grep prompt should mention grep'
	assert grep.contains('grep -rn'), 'grep prompt should have grep examples'
	assert rg.contains('rg '), 'rg prompt should have rg examples'
	// Both should have the common postamble with reference examples
	assert rg.contains('handler.go:782-920') && grep.contains('handler.go:782-920'), 'both prompts should contain example file references'
}

struct LlmCase {
	tool   string
	name   string
	has_sp bool
}

fn test_llm_command() {
	cases := [
		LlmCase{'pi', 'pi', true},
		LlmCase{'claude', 'claude', true},
		LlmCase{'gemini', 'gemini', false},
		LlmCase{'opencode', 'opencode', false},
	]
	for c in cases {
		name, args := llm_command(c.tool, '', 'sys', 'query')
		assert name == c.name, 'llm_command(${c.tool}) name = ${name}, want ${c.name}'
		joined := args.join(' ')
		if c.has_sp {
			assert joined.contains('sys'), 'llm_command(${c.tool}) should pass system prompt, got: ${joined}'
		}
	}
}

fn test_claude_defaults_haiku() {
	_, args := llm_command('claude', '', 'sys', 'query')
	joined := args.join(' ')
	assert joined.contains('--model haiku'), 'claude without model should default to haiku, got: ${joined}'
}

fn test_claude_explicit_model() {
	_, args := llm_command('claude', 'opus', 'sys', 'query')
	joined := args.join(' ')
	assert joined.contains('--model opus'), 'claude with explicit model should use it, got: ${joined}'
	assert !joined.contains('haiku'), 'claude with explicit model should not contain haiku, got: ${joined}'
}

struct ModelCase {
	tool     string
	model    string
	want_arg string
}

fn test_llm_command_with_model() {
	cases := [
		ModelCase{'pi', 'sonnet', '--model sonnet'},
		ModelCase{'claude', 'opus', '--model opus'},
		ModelCase{'gemini', 'gemini-2.5-pro', '--model gemini-2.5-pro'},
		ModelCase{'ollama', 'codellama', 'codellama'},
		ModelCase{'opencode', 'anthropic/sonnet', '--model anthropic/sonnet'},
	]
	for c in cases {
		_, args := llm_command(c.tool, c.model, 'sys', 'query')
		joined := args.join(' ')
		assert joined.contains(c.want_arg), 'llm_command(${c.tool}, ${c.model}) should contain ${c.want_arg}, got: ${joined}'
	}
}

fn test_ollama_defaults_model() {
	_, args := llm_command('ollama', '', 'sys', 'query')
	joined := args.join(' ')
	assert joined.contains('llama3'), 'ollama without model should default to llama3, got: ${joined}'
}

fn test_ollama_prepends_system() {
	_, args := llm_command('ollama', 'mistral', 'YOU ARE LOOKSY', 'find auth')
	joined := args.join(' ')
	assert joined.contains('YOU ARE LOOKSY'), 'ollama should prepend system prompt to query, got: ${joined}'
	assert joined.contains('run mistral'), 'ollama should use \'run <model>\', got: ${joined}'
}

fn test_llm_command_no_model() {
	_, args := llm_command('pi', '', 'sys', 'query')
	joined := args.join(' ')
	assert !joined.contains('--model'), 'empty model should not produce --model flag, got: ${joined}'
}

fn test_coalesce() {
	assert coalesce() == '', 'coalesce() should return empty'
	assert coalesce('', '', 'fallback') == 'fallback', 'coalesce should skip empties'
	assert coalesce('first', 'second') == 'first', 'coalesce should return first non-empty'
}

fn test_load_config() {
	tmp := os.join_path(os.temp_dir(), 'looksy-config-${os.getpid()}.env')
	os.write_file(tmp, '# comment line\nLOOKSY_LLM=claude\nLOOKSY_MODEL=opus\n\nLOOKSY_SEARCH=grep\n') or {
		assert false, 'could not write temp config'
	}
	defer {
		os.rm(tmp) or {}
		os.unsetenv('LOOKSY_CONFIG')
		os.unsetenv('LOOKSY_LLM')
		os.unsetenv('LOOKSY_MODEL')
		os.unsetenv('LOOKSY_SEARCH')
	}

	os.setenv('LOOKSY_CONFIG', tmp, true)
	// Clear any existing values
	os.unsetenv('LOOKSY_LLM')
	os.unsetenv('LOOKSY_MODEL')
	os.unsetenv('LOOKSY_SEARCH')

	load_config()

	assert os.getenv('LOOKSY_LLM') == 'claude', 'LOOKSY_LLM = ${os.getenv('LOOKSY_LLM')}, want claude'
	assert os.getenv('LOOKSY_MODEL') == 'opus', 'LOOKSY_MODEL = ${os.getenv('LOOKSY_MODEL')}, want opus'
	assert os.getenv('LOOKSY_SEARCH') == 'grep', 'LOOKSY_SEARCH = ${os.getenv('LOOKSY_SEARCH')}, want grep'
}

fn test_blockquoted_multiline_query() {
	query := 'add a retry loop\nto the HTTP client'
	_, args := llm_command('pi', '', 'sys', query)
	joined := args.join(' ')

	// Every line of the query should be blockquoted
	assert joined.contains('> add a retry loop'), 'first line should be blockquoted, got: ${joined}'
	assert joined.contains('\n> to the HTTP client'), 'second line should be blockquoted, got: ${joined}'
}

fn test_load_config_doesnt_override_existing_env() {
	tmp := os.join_path(os.temp_dir(), 'looksy-config-override-${os.getpid()}.env')
	os.write_file(tmp, 'LOOKSY_LLM=gemini\n') or { assert false, 'could not write temp config' }
	defer {
		os.rm(tmp) or {}
		os.unsetenv('LOOKSY_CONFIG')
		os.unsetenv('LOOKSY_LLM')
	}

	os.setenv('LOOKSY_CONFIG', tmp, true)
	// Set env BEFORE loading config — config should not override
	os.setenv('LOOKSY_LLM', 'claude', true)

	load_config()

	assert os.getenv('LOOKSY_LLM') == 'claude', 'existing env should take precedence, got ${os.getenv('LOOKSY_LLM')}'
}
