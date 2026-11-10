package telemetry

import (
	"regexp"
	"strings"
)

var (
	sqlWords        = regexp.MustCompile(`[A-Za-z_][A-Za-z_0-9]*`)
	sqlPlaceholders = regexp.MustCompile(`\$[0-9]+`)
	sqlWhitespace   = regexp.MustCompile(`\s+`)
)

func SQLMetadata(query string) (operation, collection string) {
	words := sqlWords.FindAllString(query, -1)
	if len(words) == 0 {
		return "UNKNOWN", ""
	}
	operation = strings.ToUpper(words[0])
	marker := ""
	switch operation {
	case "SELECT":
		marker = "FROM"
	case "INSERT":
		marker = "INTO"
	case "UPDATE":
		marker = "UPDATE"
	case "BEGIN", "COMMIT", "ROLLBACK":
		return operation, ""
	default:
		return "UNKNOWN", ""
	}
	for i, word := range words {
		if strings.ToUpper(word) != marker || i+1 >= len(words) {
			continue
		}
		candidate := strings.ToLower(words[i+1])
		if candidate == "public" && i+2 < len(words) {
			candidate = strings.ToLower(words[i+2])
		}
		if candidate == "trips" || candidate == "trip_status_history" {
			return operation, candidate
		}
		return operation, ""
	}
	return operation, ""
}

func SafeSQL(query string) string {
	stripped := sqlPlaceholders.ReplaceAllString(query, "")
	if strings.ContainsAny(stripped, "'0123456789;\\") || strings.Contains(stripped, "--") || strings.Contains(stripped, "/*") || strings.Contains(stripped, "$") {
		return ""
	}
	allowed := map[string]bool{}
	for _, word := range strings.Fields("SELECT INSERT INTO UPDATE SET FROM WHERE AND RETURNING FOR COUNT BEGIN COMMIT ROLLBACK VALUES IS NULL TRUE FALSE PUBLIC TRIPS TRIP_STATUS_HISTORY ID USER_ID DRIVER_ID START_LATITUDE START_LONGITUDE END_LATITUDE END_LONGITUDE PRICE STATUS STARTED_AT FINISHED_AT CREATED_AT UPDATED_AT TRIP_ID FROM_STATUS TO_STATUS CHANGED_AT") {
		allowed[word] = true
	}
	for _, word := range sqlWords.FindAllString(stripped, -1) {
		if !allowed[strings.ToUpper(word)] {
			return ""
		}
	}
	return strings.TrimSpace(sqlWhitespace.ReplaceAllString(query, " "))
}
