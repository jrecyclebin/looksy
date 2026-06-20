package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestRefRe(t *testing.T) {
	cases := []struct {
		line  string
		match bool
	}{
		{"handler.go:782-920 — desc", true},
		{"web/frbr.js:250-310 — desc", true},
		{"some/path/file.ts:1-5", true},
		{"main.go:77", true},
		{"not a reference", false},
		{"  handler.go:782-920", true}, // leading space is fine now — refs found anywhere
		{"## 2. **`main.go:77`** - Update model flag help text", true},
	}
	for _, c := range cases {
		got := refRe.MatchString(c.line)
		if got != c.match {
			t.Errorf("refRe.MatchString(%q) = %v, want %v", c.line, got, c.match)
		}
	}
}

func TestExtractComment(t *testing.T) {
	cases := []struct {
		after string
		want  string
	}{
		{" — Update model flag help text", " Update model flag help text"},
		{" - some description", " some description"},
		{"** - Update the thing", " Update the thing"},
		{"`**", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := extractComment(c.after)
		if got != c.want {
			t.Errorf("extractComment(%q) = %q, want %q", c.after, got, c.want)
		}
	}
}

func TestProcessOutputWithFakeRefs(t *testing.T) {
	// Create a temp file so expandReference can actually read it
	tmpFile, err := os.CreateTemp("", "looksy-test-*.go")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	// Write 20 lines
	var content strings.Builder
	for i := 1; i <= 20; i++ {
		content.WriteString(fmt.Sprintf("line %d\n", i))
	}
	tmpFile.WriteString(content.String())
	tmpFile.Close()

	input := "Here is what I found:\n\n" +
		tmpFile.Name() + ":3-7 — test range\n" +
		"nonexistent.go:1-5 — missing file\n"

	var buf bytes.Buffer
	processOutput(&buf, input)
	output := buf.String()

	// The full LLM response should be printed as-is
	if !strings.Contains(output, "Here is what I found:") {
		t.Error("output missing LLM response text")
	}
	// The separator should appear before expanded refs
	if !strings.Contains(output, "\n---\n") {
		t.Error("output missing separator before expanded refs")
	}
	if !strings.Contains(output, "line 3") || !strings.Contains(output, "line 7") {
		t.Error("output missing expanded file range")
	}
	if !strings.Contains(output, "FILE NOT FOUND") {
		t.Error("output missing FILE NOT FOUND for nonexistent file")
	}
}

func TestBuildPrompt(t *testing.T) {
	rg := buildPrompt("rg")
	grep := buildPrompt("grep")

	if !strings.Contains(rg, "ripgrep") {
		t.Error("rg prompt should mention ripgrep")
	}
	if !strings.Contains(grep, "grep") {
		t.Error("grep prompt should mention grep")
	}
	if !strings.Contains(grep, "grep -rn") {
		t.Error("grep prompt should have grep examples")
	}
	if !strings.Contains(rg, "rg ") {
		t.Error("rg prompt should have rg examples")
	}
	// Both should have the common postamble with reference examples
	if !strings.Contains(rg, "handler.go:782-920") || !strings.Contains(grep, "handler.go:782-920") {
		t.Error("both prompts should contain example file references")
	}
}

func TestLLMCommand(t *testing.T) {
	cases := []struct {
		tool  string
		name  string
		hasSP bool
	}{
		{"pi", "pi", true},
		{"claude", "claude", true},
		{"gemini", "gemini", false},
		{"opencode", "opencode", false},
	}
	for _, c := range cases {
		name, args := llmCommand(c.tool, "", "sys", "query")
		if name != c.name {
			t.Errorf("llmCommand(%q) name = %q, want %q", c.tool, name, c.name)
		}
		joined := strings.Join(args, " ")
		if c.hasSP {
			if !strings.Contains(joined, "sys") {
				t.Errorf("llmCommand(%q) should pass system prompt, got args: %s", c.tool, joined)
			}
		}
	}
}

func TestClaudeDefaultsHaiku(t *testing.T) {
	_, args := llmCommand("claude", "", "sys", "query")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--model haiku") {
		t.Errorf("claude without model should default to haiku, got: %s", joined)
	}
}

func TestClaudeExplicitModel(t *testing.T) {
	_, args := llmCommand("claude", "opus", "sys", "query")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--model opus") {
		t.Errorf("claude with explicit model should use it, got: %s", joined)
	}
	if strings.Contains(joined, "haiku") {
		t.Errorf("claude with explicit model should not contain haiku, got: %s", joined)
	}
}

func TestLLMCommandWithModel(t *testing.T) {
	cases := []struct {
		tool    string
		model   string
		wantArg string
	}{
		{"pi", "sonnet", "--model sonnet"},
		{"claude", "opus", "--model opus"},
		{"gemini", "gemini-2.5-pro", "--model gemini-2.5-pro"},
		{"ollama", "codellama", "codellama"},
		{"opencode", "anthropic/sonnet", "--model anthropic/sonnet"},
	}
	for _, c := range cases {
		_, args := llmCommand(c.tool, c.model, "sys", "query")
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, c.wantArg) {
			t.Errorf("llmCommand(%q, %q) should contain %q, got: %s", c.tool, c.model, c.wantArg, joined)
		}
	}
}

func TestOllamaDefaultsModel(t *testing.T) {
	_, args := llmCommand("ollama", "", "sys", "query")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "llama3") {
		t.Errorf("ollama without model should default to llama3, got: %s", joined)
	}
}

func TestOllamaPrependsSystem(t *testing.T) {
	_, args := llmCommand("ollama", "mistral", "YOU ARE LOOKSY", "find auth")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "YOU ARE LOOKSY") {
		t.Errorf("ollama should prepend system prompt to query, got: %s", joined)
	}
	if !strings.Contains(joined, "run mistral") {
		t.Errorf("ollama should use 'run <model>', got: %s", joined)
	}
}

func TestLLMCommandNoModel(t *testing.T) {
	_, args := llmCommand("pi", "", "sys", "query")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--model") {
		t.Errorf("empty model should not produce --model flag, got: %s", joined)
	}
}

func TestCoalesce(t *testing.T) {
	if coalesce() != "" {
		t.Error("coalesce() should return empty")
	}
	if coalesce("", "", "fallback") != "fallback" {
		t.Error("coalesce should skip empties")
	}
	if coalesce("first", "second") != "first" {
		t.Error("coalesce should return first non-empty")
	}
}

func TestLoadConfig(t *testing.T) {
	// Write a temp config file
	tmpFile, err := os.CreateTemp("", "looksy-config-*.env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("# comment line\n")
	tmpFile.WriteString("LOOKSY_LLM=claude\n")
	tmpFile.WriteString("LOOKSY_MODEL=opus\n")
	tmpFile.WriteString("\n") // blank line
	tmpFile.WriteString("LOOKSY_SEARCH=grep\n")
	tmpFile.Close()

	// Point to our temp config
	os.Setenv("LOOKSY_CONFIG", tmpFile.Name())
	defer os.Unsetenv("LOOKSY_CONFIG")

	// Clear any existing values
	os.Unsetenv("LOOKSY_LLM")
	os.Unsetenv("LOOKSY_MODEL")
	os.Unsetenv("LOOKSY_SEARCH")

	loadConfig()

	if os.Getenv("LOOKSY_LLM") != "claude" {
		t.Errorf("LOOKSY_LLM = %q, want %q", os.Getenv("LOOKSY_LLM"), "claude")
	}
	if os.Getenv("LOOKSY_MODEL") != "opus" {
		t.Errorf("LOOKSY_MODEL = %q, want %q", os.Getenv("LOOKSY_MODEL"), "opus")
	}
	if os.Getenv("LOOKSY_SEARCH") != "grep" {
		t.Errorf("LOOKSY_SEARCH = %q, want %q", os.Getenv("LOOKSY_SEARCH"), "grep")
	}
}

func TestBlockquotedMultilineQuery(t *testing.T) {
	query := "add a retry loop\nto the HTTP client"
	_, args := llmCommand("pi", "", "sys", query)
	joined := strings.Join(args, " ")

	// Every line of the query should be blockquoted
	if !strings.Contains(joined, "> add a retry loop") {
		t.Errorf("first line should be blockquoted, got: %s", joined)
	}
	if !strings.Contains(joined, "\n> to the HTTP client") {
		t.Errorf("second line should be blockquoted with > prefix, got: %s", joined)
	}
}

func TestLoadConfigDoesntOverrideExistingEnv(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "looksy-config-*.env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("LOOKSY_LLM=gemini\n")
	tmpFile.Close()

	os.Setenv("LOOKSY_CONFIG", tmpFile.Name())
	defer os.Unsetenv("LOOKSY_CONFIG")

	// Set env BEFORE loading config — config should not override
	os.Setenv("LOOKSY_LLM", "claude")
	defer os.Unsetenv("LOOKSY_LLM")

	loadConfig()

	if os.Getenv("LOOKSY_LLM") != "claude" {
		t.Errorf("existing env should take precedence, got %q", os.Getenv("LOOKSY_LLM"))
	}
}
