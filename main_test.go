package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestExtractBlock(t *testing.T) {
	input := "before <summary>hello world</summary> after"
	got := extractBlock(input, "summary")
	want := "hello world"
	if got != want {
		t.Errorf("extractBlock(summary) = %q, want %q", got, want)
	}
}

func TestExtractBlockMissing(t *testing.T) {
	got := extractBlock("no tags here", "summary")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractBlockReferences(t *testing.T) {
	input := "<references>\nfoo.go:1-10 — desc\nbar.go:5-20 — other\n</references>"
	got := extractBlock(input, "references")
	if !strings.Contains(got, "foo.go:1-10") {
		t.Errorf("missing foo.go reference, got %q", got)
	}
}

func TestRefLineRe(t *testing.T) {
	cases := []struct {
		line   string
		match  bool
	}{
		{"handler.go:782-920 — desc", true},
		{"web/frbr.js:250-310 — desc", true},
		{"some/path/file.ts:1-5", true},
		{"not a reference", false},
		{"  handler.go:782-920", false}, // leading space
	}
	for _, c := range cases {
		got := refLineRe.MatchString(c.line)
		if got != c.match {
			t.Errorf("refLineRe.MatchString(%q) = %v, want %v", c.line, got, c.match)
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

	input := "<summary>Found some stuff</summary>\n\n" +
		"<references>\n" +
		tmpFile.Name() + ":3-7 — test range\n" +
		"nonexistent.go:1-5 — missing file\n" +
		"</references>"

	var buf bytes.Buffer
	processOutput(&buf, input)
	output := buf.String()

	if !strings.Contains(output, "Found some stuff") {
		t.Error("output missing summary")
	}
	if !strings.Contains(output, "<references>") {
		t.Error("output missing <references> tag")
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
	// Both should have the common postamble
	if !strings.Contains(rg, "<references>") || !strings.Contains(grep, "<references>") {
		t.Error("both prompts should contain the example references section")
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

func TestLLMCommandWithModel(t *testing.T) {
	cases := []struct {
		tool    string
		model   string
		wantArg string
	}{
		{"pi", "sonnet", "--model sonnet"},
		{"claude", "opus", "--model opus"},
		{"gemini", "gemini-2.5-pro", "--model gemini-2.5-pro"},
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
