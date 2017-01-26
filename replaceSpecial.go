package main

import (
	"regexp"
	"strings"
)

func ReplaceSpecial(in string) string {

	r, _ := regexp.Compile("[^a-zA-Z\\d\\s]")

	out := strings.TrimSpace(r.ReplaceAllString(in, ""))

	r2, _ := regexp.Compile("\\s+")

	out = strings.TrimSpace(r2.ReplaceAllString(out, " "))

	return out
}
