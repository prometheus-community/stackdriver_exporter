package utils

import (
	"regexp"
	"strings"

	"github.com/fatih/camelcase"
)

var (
	safeNameRE = regexp.MustCompile(`[^a-zA-Z0-9_]*$`)
)

func NormalizeMetricName(metricName string) string {
	var normalizedMetricName []string

	words := camelcase.Split(metricName)
	for _, word := range words {
		safeWord := strings.Trim(safeNameRE.ReplaceAllLiteralString(word, "_"), "_")
		lowerWord := strings.TrimSpace(strings.ToLower(safeWord))
		if lowerWord != "" {
			normalizedMetricName = append(normalizedMetricName, lowerWord)
		}
	}

	return strings.Join(normalizedMetricName, "_")
}

func ProjectResource(projectID string) string {
	return "projects/" + projectID
}
