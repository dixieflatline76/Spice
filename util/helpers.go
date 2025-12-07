package util

import (
	"fmt"
)

// StringerString is a string that implements fmt.Stringer
type StringerString string

func (s StringerString) String() string {
	return string(s)
}

// StringsToStringers converts a slice of strings to a slice of fmt.Stringer
func StringsToStringers(strs []string) []fmt.Stringer {
	stringers := make([]fmt.Stringer, len(strs))
	for i, s := range strs {
		stringers[i] = StringerString(s)
	}
	return stringers
}
