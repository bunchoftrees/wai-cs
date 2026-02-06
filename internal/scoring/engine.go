package scoring

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/workforce-ai/site-selection-iq/internal/models"
	"github.com/workforce-ai/site-selection-iq/internal/schema"
)

// ScoreFunc defines the signature for a scoring function.
// It takes site data and a resolved schema, and returns:
// - rawScore: the unscaled numeric score
// - finalScore: the normalized score (0-100)
// - explanation: detailed breakdown of scoring factors
// - error: any error that occurred during scoring
type ScoreFunc func(
	siteData map[string]interface{},
	resolvedSchema *schema.ResolvedSchema,
) (rawScore float64, finalScore float64, explanation models.Explanation, err error)

// DefaultScoreFunc implements weighted scoring across numeric fields.
// For each numeric field in the schema that has a weight:
// 1. Extract the value from siteData
// 2. Normalize the value to 0-1 range using min/max bounds
// 3. Multiply normalized value by the field's weight
// 4. Sum all weighted contributions for raw score
// 5. Normalize raw score to 0-100 range for final score
// Each factor produces detailed explanation including contribution and reasoning.
func DefaultScoreFunc(
	siteData map[string]interface{},
	resolvedSchema *schema.ResolvedSchema,
) (rawScore float64, finalScore float64, explanation models.Explanation, err error) {
	if resolvedSchema == nil {
		return 0, 0, models.Explanation{}, fmt.Errorf("resolved schema cannot be nil")
	}

	if len(siteData) == 0 {
		return 0, 0, models.Explanation{}, fmt.Errorf("site data cannot be empty")
	}

	explanation.Factors = []models.ExplanationFactor{}
	var totalWeightedScore float64
	var totalWeight float64
	maxPossibleScore := 0.0

	// Iterate through all fields in the resolved schema
	for fieldName, fieldDef := range resolvedSchema.Fields {
		// Only process numeric fields that have weights
		if !isNumericFieldType(fieldDef.Type) || fieldDef.Weight == 0 {
			continue
		}

		weight := resolvedSchema.Weights[fieldName]
		if weight == 0 {
			continue
		}

		// Extract the value from site data
		rawValue, exists := siteData[fieldName]
		if !exists {
			// Skip missing fields - treat as not contributing to score
			continue
		}

		// Convert to float64
		numValue, err := toFloat64(rawValue)
		if err != nil {
			// Skip fields that can't be converted to numeric
			continue
		}

		// Normalize value to 0-1 range
		normalizedValue := normalizeValue(numValue, fieldDef.Min, fieldDef.Max, fieldDef.Direction)

		// Calculate contribution (normalized value * weight)
		contribution := normalizedValue * weight
		totalWeightedScore += contribution
		totalWeight += weight

		// Determine if this is a positive or negative contribution
		direction := string(fieldDef.Direction)
		if fieldDef.Direction == schema.DirectionMinimize {
			direction = "minimize"
		} else {
			direction = "maximize"
		}

		// Generate reason string for this factor
		reason := generateReasonString(fieldName, numValue, normalizedValue, fieldDef.Direction)

		// Create explanation factor
		factor := models.ExplanationFactor{
			Name:         fieldName,
			Value:        numValue,
			Weight:       weight,
			Contribution: contribution,
			Direction:    direction,
			Reason:       reason,
		}

		explanation.Factors = append(explanation.Factors, factor)
		maxPossibleScore += weight // Each weight can contribute max of 1 * weight
	}

	// Calculate raw score
	rawScore = totalWeightedScore

	// Normalize raw score to 0-100 range
	if maxPossibleScore > 0 {
		finalScore = (rawScore / maxPossibleScore) * 100
	} else {
		finalScore = 0
	}

	// Ensure final score is bounded to 0-100
	finalScore = math.Max(0, math.Min(100, finalScore))

	// Sort factors by contribution (descending) for summary generation
	sort.Slice(explanation.Factors, func(i, j int) bool {
		return math.Abs(explanation.Factors[i].Contribution) > math.Abs(explanation.Factors[j].Contribution)
	})

	// Generate summary from top contributing factors
	explanation.Summary = generateSummary(explanation.Factors, finalScore)

	return rawScore, finalScore, explanation, nil
}

// isNumericFieldType checks if a field type is numeric
func isNumericFieldType(fieldType schema.FieldType) bool {
	switch fieldType {
	case schema.TypeNumeric, schema.TypeInteger, schema.TypePercentage,
		schema.TypeIndex, schema.TypePopulation:
		return true
	default:
		return false
	}
}

// toFloat64 converts a value to float64
func toFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case string:
		// Try to parse as JSON number
		var num float64
		if err := json.Unmarshal([]byte(v), &num); err != nil {
			return 0, fmt.Errorf("cannot convert string to float64: %w", err)
		}
		return num, nil
	default:
		return 0, fmt.Errorf("unsupported type for numeric conversion: %T", value)
	}
}

// normalizeValue normalizes a numeric value to 0-1 range based on min/max bounds
// Takes direction into account: for minimize, higher actual values become lower normalized values
func normalizeValue(value float64, min, max *float64, direction schema.Direction) float64 {
	// Handle cases where min/max are not defined
	if min == nil || max == nil {
		// Without bounds, use a default normalization strategy
		// Clamp value to prevent extreme outliers
		// Assume a reasonable range; values are normalized relative to a reference
		return math.Max(0, math.Min(1, value/100.0))
	}

	minVal := *min
	maxVal := *max

	// Prevent division by zero
	if maxVal == minVal {
		return 0.5
	}

	// Normalize to 0-1 range
	normalized := (value - minVal) / (maxVal - minVal)
	normalized = math.Max(0, math.Min(1, normalized)) // Clamp to 0-1

	// For minimize direction, invert the normalized value
	// So lower actual values = higher normalized scores
	if direction == schema.DirectionMinimize {
		normalized = 1.0 - normalized
	}

	return normalized
}

// generateReasonString creates a human-readable explanation for a field's contribution
func generateReasonString(
	fieldName string,
	value float64,
	normalizedValue float64,
	direction schema.Direction,
) string {
	// Format the field name for readability
	readableName := strings.ReplaceAll(fieldName, "_", " ")
	readableName = capitalizeWords(readableName)

	// Determine quality description based on normalized value
	quality := "neutral"
	if normalizedValue >= 0.75 {
		quality = "excellent"
	} else if normalizedValue >= 0.5 {
		quality = "good"
	} else if normalizedValue >= 0.25 {
		quality = "fair"
	} else {
		quality = "poor"
	}

	// Build the reason string
	if direction == schema.DirectionMaximize {
		return fmt.Sprintf("%s value is %.2f, which is %s for this metric (higher is better)",
			readableName, value, quality)
	} else {
		return fmt.Sprintf("%s value is %.2f, which is %s for this metric (lower is better)",
			readableName, value, quality)
	}
}

// generateSummary creates a summary from the top contributing factors
func generateSummary(factors []models.ExplanationFactor, finalScore float64) string {
	if len(factors) == 0 {
		return "No scoring factors contributed to this site's score."
	}

	// Get top 3 contributing factors
	topCount := 3
	if len(factors) < topCount {
		topCount = len(factors)
	}

	var topFactors []string
	for i := 0; i < topCount; i++ {
		factor := factors[i]
		if factor.Contribution > 0 {
			topFactors = append(topFactors, factor.Name)
		}
	}

	if len(topFactors) == 0 {
		return fmt.Sprintf("Final score is %.1f based on weighted factor analysis.", finalScore)
	}

	// Build summary statement
	summary := fmt.Sprintf("Final score is %.1f.", finalScore)

	if len(topFactors) == 1 {
		summary += fmt.Sprintf(" The primary contributing factor is %s.",
			strings.ReplaceAll(topFactors[0], "_", " "))
	} else if len(topFactors) == 2 {
		summary += fmt.Sprintf(" Top contributing factors are %s and %s.",
			strings.ReplaceAll(topFactors[0], "_", " "),
			strings.ReplaceAll(topFactors[1], "_", " "))
	} else if len(topFactors) >= 3 {
		lastFactor := topFactors[len(topFactors)-1]
		otherFactors := strings.Join(
			func() []string {
				var cleaned []string
				for _, f := range topFactors[:len(topFactors)-1] {
					cleaned = append(cleaned, strings.ReplaceAll(f, "_", " "))
				}
				return cleaned
			}(),
			", ",
		)
		summary += fmt.Sprintf(" Top contributing factors are %s, and %s.",
			otherFactors, strings.ReplaceAll(lastFactor, "_", " "))
	}

	return summary
}

// capitalizeWords capitalizes the first letter of each word in a string
func capitalizeWords(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	for i := 1; i < len(runes); i++ {
		if runes[i-1] == ' ' && i < len(runes) {
			runes[i] = unicode.ToUpper(runes[i])
		}
	}
	return string(runes)
}
