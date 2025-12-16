package main

import (
	"os"
	"strings"
	"testing"
)

func TestExtractConstant(t *testing.T) {
	// Create a temporary file with Go constants
	tmpContent := `package foo

const (
	MyRegex = ` + "`^https://example.com/.*$`" + `
	OtherConst = "some string"
)
`
	tmpfile, err := os.CreateTemp("", "const_*.go")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up

	if _, err := tmpfile.Write([]byte(tmpContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Test extraction
	val, err := extractConstant(tmpfile.Name(), "MyRegex")
	if err != nil {
		t.Fatalf("extractConstant failed: %v", err)
	}

	expected := "^https://example.com/.*$"
	if val != expected {
		t.Errorf("Expected %q, got %q", expected, val)
	}
}

func TestGenerateJSBlock(t *testing.T) {
	regexMap := map[string]string{
		"Wallhaven": "^wallhaven.*$",
		"Pexels":    "^pexels.*$",
	}

	block := generateJSBlock(regexMap)

	if !strings.Contains(block, "// REGEX_START") {
		t.Error("Missing start marker")
	}
	if !strings.Contains(block, "// REGEX_END") {
		t.Error("Missing end marker")
	}
	if !strings.Contains(block, "/^wallhaven.*$/") {
		t.Error("Missing Wallhaven regex")
	}
	if !strings.Contains(block, "/^pexels.*$/") {
		t.Error("Missing Pexels regex")
	}
}

func TestUpdateJSContent(t *testing.T) {
	original := `
// Some header
// REGEX_START
old content
// REGEX_END
// Some footer
`
	newBlock := `
// REGEX_START
new content
// REGEX_END`

	updated, err := updateJSContent(original, newBlock)
	if err != nil {
		t.Fatalf("updateJSContent failed: %v", err)
	}

	if !strings.Contains(updated, "new content") {
		t.Error("Updated content does not contain new content")
	}
	if strings.Contains(updated, "old content") {
		t.Error("Updated content still contains old content")
	}
	if !strings.Contains(updated, "// Some header") || !strings.Contains(updated, "// Some footer") {
		t.Error("Updated content destroyed surrounding markers")
	}
}
