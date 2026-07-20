package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	for _, f := range []string{"docs/collections/rijksmuseum.json", "docs/collections/npm.json"} {
		b, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", f, err)
			continue
		}
		var x interface{}
		err = json.Unmarshal(b, &x)
		if err != nil {
			if syntaxErr, ok := err.(*json.SyntaxError); ok {
				fmt.Printf("Error in %s: %v at byte %d\n", f, err, syntaxErr.Offset)
			} else {
				fmt.Printf("Error in %s: %v\n", f, err)
			}
		} else {
			fmt.Printf("OK %s\n", f)
		}
	}
}
