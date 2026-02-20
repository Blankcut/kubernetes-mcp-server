package utils

import (
	"strings"
)

// TruncateContent ensures that a string doesn't exceed a maximum size
// while trying to maintain meaningful content
func TruncateContent(content string, maxSize int) string {
	if len(content) <= maxSize {
		return content
	}

	// Simple truncation strategy - keep the beginning and add a note
	truncateMsg := "\n\n[Note: Content was truncated due to length limitations.]"
	reservedSize := len(truncateMsg)
	truncated := content[:maxSize-reservedSize] + truncateMsg

	return truncated
}

// TruncateContextSmartly truncates content with a smarter approach:
// - Keeps the beginning and end parts
// - Removes content from the middle if necessary
func TruncateContextSmartly(content string, maxSize int) string {
	if len(content) <= maxSize {
		return content
	}

	// Note about truncation
	truncateMsg := "\n\n[...Content truncated...]\n\n"
	reservedSize := len(truncateMsg)

	// Keep 60% from the beginning and 40% from the end
	beginSize := int(float64(maxSize-reservedSize) * 0.6)
	endSize := int(float64(maxSize-reservedSize) * 0.4)

	// Make sure we maintain complete sentences
	beginPart := content[:beginSize]
	lastPeriod := strings.LastIndex(beginPart, ".")
	if lastPeriod > 0 {
		beginPart = beginPart[:lastPeriod+1]
	}

	endPart := content[len(content)-endSize:]
	firstPeriod := strings.Index(endPart, ".")
	if firstPeriod > 0 {
		endPart = endPart[firstPeriod+1:]
	}

	return beginPart + truncateMsg + endPart
}
