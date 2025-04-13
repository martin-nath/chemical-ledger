package utils

import "strings"

func ToCamelCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.ToLower(w)
		if i == 0 {
			continue
		}
		words[i] = strings.ToUpper(words[i][0:1]) + w[1:]
	}
	return strings.Join(words, "")
}
