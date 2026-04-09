package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
)

// plist XML structure
type plistDict struct {
	XMLName xml.Name `xml:"plist"`
	Version string   `xml:"version,attr"`
	Dict    dict     `xml:"dict"`
}

type dict struct {
	Items []xmlItem
}

type xmlItem struct {
	Key   string
	Value string
	Type  string // "string", "true", "false"
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: plist_modify <plist_path> <key>=<type>:<value> ...\n")
		fmt.Fprintf(os.Stderr, "Types: string, bool\n")
		fmt.Fprintf(os.Stderr, "Example: plist_modify Info.plist LSUIElement=bool:true CFBundleVersion=string:101\n")
		os.Exit(1)
	}

	plistPath := os.Args[1]

	// Parse the modifications
	mods := make(map[string]xmlItem)
	for _, arg := range os.Args[2:] {
		// Strip any stray carriage returns from CRLF line ending issues
		arg = strings.TrimRight(arg, "\r")
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid argument: %q\n", arg)
			os.Exit(1)
		}
		key := strings.TrimSpace(parts[0])
		typeVal := strings.SplitN(parts[1], ":", 2)
		if len(typeVal) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid type:value: %s\n", parts[1])
			os.Exit(1)
		}
		mods[key] = xmlItem{Key: key, Type: typeVal[0], Value: strings.TrimRight(typeVal[1], "\r")}
	}

	// Read the plist as raw text (preserving format as much as possible)
	data, err := os.ReadFile(plistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading plist: %v\n", err)
		os.Exit(1)
	}

	// Normalize line endings — strip \r so string searches work regardless of encoding
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	// For each modification, either replace existing key or insert before </dict>
	for key, mod := range mods {
		valueXML := formatValue(mod.Type, mod.Value)

		// Check if key already exists
		keyTag := fmt.Sprintf("<key>%s</key>", key)
		if idx := strings.Index(content, keyTag); idx != -1 {
			// Find the value element after the key
			afterKey := content[idx+len(keyTag):]
			// Skip whitespace/newlines
			trimmed := strings.TrimLeft(afterKey, " \t\n\r")
			// Find the end of the value element
			valueEnd := findValueEnd(trimmed)
			if valueEnd > 0 {
				// Calculate positions in original content
				valueStart := idx + len(keyTag) + (len(afterKey) - len(trimmed))
				content = content[:valueStart] + "\n\t" + valueXML + content[valueStart+valueEnd:]
			}
		} else {
			// Insert before </dict>
			dictEnd := strings.LastIndex(content, "</dict>")
			if dictEnd == -1 {
				preview := content
				if len(preview) > 500 {
					preview = preview[:500]
				}
				fmt.Fprintf(os.Stderr, "Could not find </dict> in plist. File length: %d bytes. Preview:\n%s\n", len(content), preview)
				os.Exit(1)
			}
			insert := fmt.Sprintf("\t%s\n\t%s\n", keyTag, valueXML)
			content = content[:dictEnd] + insert + content[dictEnd:]
		}
	}

	// Write the modified plist back
	if err := os.WriteFile(plistPath, []byte(content), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing plist: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully modified %s with %d changes\n", plistPath, len(mods))
}

func formatValue(typ, value string) string {
	switch typ {
	case "string":
		return fmt.Sprintf("<string>%s</string>", value)
	case "bool":
		if value == "true" {
			return "<true/>"
		}
		return "<false/>"
	default:
		return fmt.Sprintf("<string>%s</string>", value)
	}
}

// findValueEnd finds the end position of a plist value element
func findValueEnd(s string) int {
	// Handle self-closing tags like <true/> or <false/>
	if strings.HasPrefix(s, "<true/>") {
		return len("<true/>")
	}
	if strings.HasPrefix(s, "<false/>") {
		return len("<false/>")
	}

	// Handle <string>...</string>, <integer>...</integer>, etc.
	if strings.HasPrefix(s, "<") {
		closeIdx := strings.Index(s, ">")
		if closeIdx == -1 {
			return -1
		}
		tagName := s[1:closeIdx]
		closeTag := fmt.Sprintf("</%s>", tagName)
		endIdx := strings.Index(s, closeTag)
		if endIdx == -1 {
			return -1
		}
		return endIdx + len(closeTag)
	}

	return -1
}
