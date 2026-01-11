package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

func main() {
	reportDir := "report_output"
	reports := []string{"report.html", "tuning_report.html"}

	allowedFiles := make(map[string]bool)

	// 1. Parse reports to find allowed images
	for _, reportName := range reports {
		path := filepath.Join(reportDir, reportName)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Printf("Report not found, skipping: %s\n", path)
			continue
		}

		f, err := os.Open(path)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		doc, err := html.Parse(f)
		if err != nil {
			panic(err)
		}

		var crawler func(*html.Node)
		crawler = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "img" {
				for _, a := range n.Attr {
					if a.Key == "src" {
						// The src might be relative or just a filename.
						// In these reports, they are filenames.
						fname := filepath.Base(a.Val)
						allowedFiles[fname] = true
						fmt.Printf("Keeping: %s (referenced in %s)\n", fname, reportName)
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				crawler(c)
			}
		}
		crawler(doc)
	}

	// 2. Walk directory and delete unreferenced images
	entries, err := os.ReadDir(reportDir)
	if err != nil {
		panic(err)
	}

	deletedCount := 0
	keptCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))

		// Only target image files
		if ext != ".jpg" && ext != ".png" {
			continue
		}

		if !allowedFiles[name] {
			fmt.Printf("Deleting: %s\n", name)
			fullPath := filepath.Join(reportDir, name)
			if err := os.Remove(fullPath); err != nil {
				fmt.Printf("Error deleting %s: %v\n", name, err)
			}
			deletedCount++
		} else {
			keptCount++
		}
	}

	fmt.Printf("\nCleanup complete. Kept %d files, deleted %d files.\n", keptCount, deletedCount)
}
