package utils

import (
	"fmt"
	"strings"
	"time"
)

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

func UnixTimestamp(dateStr string) (int64, error) {
	// Parse input date
	date, err := time.Parse("02-01-2006", dateStr)
	if err != nil {
		return 0, err
	}

	// Get current time
	now := time.Now()

	// Combine date with current time (local)
	combined := time.Date(date.Year(), date.Month(), date.Day(),
		now.Hour(), now.Minute(), now.Second(), 0, time.Local)

	// fmt.Println(now.Hour())
	// fmt.Println(now.Minute())
	// fmt.Println(now.Second())


	fmt.Println(combined.Unix())
	// Convert to Unix timestamp (UTC)
	return combined.Unix(), nil
}
