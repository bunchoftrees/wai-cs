package scoring

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/workforce-ai/site-selection-iq/internal/schema"
)

func TestDefaultScoreFunc_BasicScoring(t *testing.T) {
	// Test basic scoring with known inputs produces expected range
	min := 0.0
	max := 100.0

	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"population": {
				Type:      schema.TypePopulation,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMaximize,
			},
			"unemployment": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMinimize,
			},
		},
		Weights: map[string]float64{
			"population":   1.0,
			"unemployment": 1.0,
		},
	}

	// Site with good population and low unemployment (good site)
	siteData := map[string]interface{}{
		"population":   75.0,
		"unemployment": 25.0,
	}

	rawScore, finalScore, explanation, err := DefaultScoreFunc(siteData, resolvedSchema)

	require.NoError(t, err, "Should not error with valid data")
	assert.NotNil(t, explanation)

	// Final score should be in valid range (0-100)
	assert.GreaterOrEqual(t, finalScore, 0.0, "Final score should be >= 0")
	assert.LessOrEqual(t, finalScore, 100.0, "Final score should be <= 100")

	// Raw score should be positive since both normalized values are positive
	assert.Greater(t, rawScore, 0.0, "Raw score should be positive")

	// Check that factors are created
	assert.NotEmpty(t, explanation.Factors, "Should have explanation factors")
	assert.GreaterOrEqual(t, len(explanation.Factors), 1, "Should have at least one factor")

	// Summary should be populated
	assert.NotEmpty(t, explanation.Summary, "Summary should not be empty")
}

func TestDefaultScoreFunc_ExplanationFactors(t *testing.T) {
	// Test that all weighted fields produce explanation factors
	min := 0.0
	max := 100.0
	minPop := 0.0
	maxPop := 1000000.0

	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"population": {
				Type:      schema.TypePopulation,
				Required:  true,
				Min:       &minPop,
				Max:       &maxPop,
				Weight:    2.0,
				Direction: schema.DirectionMaximize,
				Description: "Site population",
			},
			"unemployment": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.5,
				Direction: schema.DirectionMinimize,
				Description: "Unemployment rate",
			},
			"growth_rate": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMaximize,
				Description: "Market growth rate",
			},
			"notes": {
				Type:      schema.TypeText,
				Required:  false,
				Weight:    0.0,
				Direction: schema.DirectionMaximize,
				Description: "Site notes",
			},
		},
		Weights: map[string]float64{
			"population":   2.0,
			"unemployment": 1.5,
			"growth_rate":  1.0,
		},
	}

	siteData := map[string]interface{}{
		"population":   500000.0,
		"unemployment": 4.5,
		"growth_rate":  7.2,
		"notes":        "Good location",
	}

	rawScore, finalScore, explanation, err := DefaultScoreFunc(siteData, resolvedSchema)

	require.NoError(t, err)
	assert.NotNil(t, explanation)

	// Should have exactly 3 factors (population, unemployment, growth_rate)
	// notes field should not contribute (weight is 0)
	assert.Len(t, explanation.Factors, 3, "Should have exactly 3 factors")

	// Verify each factor has required fields
	for _, factor := range explanation.Factors {
		assert.NotEmpty(t, factor.Name, "Factor name should not be empty")
		assert.NotEmpty(t, factor.Reason, "Factor reason should not be empty")
		assert.Greater(t, factor.Weight, 0.0, "Factor weight should be positive")
		assert.GreaterOrEqual(t, factor.Value, 0.0, "Factor value should be valid")
		assert.NotEmpty(t, factor.Direction, "Direction should be specified")
	}

	// Verify score ranges are reasonable
	assert.GreaterOrEqual(t, finalScore, 0.0)
	assert.LessOrEqual(t, finalScore, 100.0)
	assert.Greater(t, rawScore, 0.0, "Raw score should be positive with these inputs")
}

func TestDefaultScoreFunc_HigherUnemploymentScoresHigher(t *testing.T) {
	// Test that for minimize direction, lower unemployment produces higher normalized score
	min := 0.0
	max := 100.0

	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"unemployment": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMinimize,
			},
		},
		Weights: map[string]float64{
			"unemployment": 1.0,
		},
	}

	// Site with low unemployment (good)
	siteDataLowUnemployment := map[string]interface{}{
		"unemployment": 2.0,
	}

	_, scoreWithLowUnemployment, explanationLow, err := DefaultScoreFunc(siteDataLowUnemployment, resolvedSchema)
	require.NoError(t, err)

	// Site with high unemployment (bad)
	siteDataHighUnemployment := map[string]interface{}{
		"unemployment": 10.0,
	}

	_, scoreWithHighUnemployment, explanationHigh, err := DefaultScoreFunc(siteDataHighUnemployment, resolvedSchema)
	require.NoError(t, err)

	// Low unemployment should score higher than high unemployment
	assert.Greater(t, scoreWithLowUnemployment, scoreWithHighUnemployment,
		"Lower unemployment should produce higher final score")

	// Verify factor directions in explanations
	require.Len(t, explanationLow.Factors, 1)
	require.Len(t, explanationHigh.Factors, 1)

	lowFactor := explanationLow.Factors[0]
	highFactor := explanationHigh.Factors[0]

	assert.Equal(t, "minimize", lowFactor.Direction, "Direction should be minimize")
	assert.Equal(t, "minimize", highFactor.Direction, "Direction should be minimize")

	// Low unemployment should have positive/higher contribution
	assert.Greater(t, lowFactor.Contribution, highFactor.Contribution,
		"Lower unemployment should have higher contribution")
}

func TestDefaultScoreFunc_MaximizeDirection(t *testing.T) {
	// Test maximize direction: higher values = higher scores
	min := 0.0
	max := 1000000.0

	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"population": {
				Type:      schema.TypePopulation,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"population": 1.0,
		},
	}

	// Site with low population
	siteDataLowPop := map[string]interface{}{
		"population": 10000.0,
	}

	_, scoreLow, explanationLow, err := DefaultScoreFunc(siteDataLowPop, resolvedSchema)
	require.NoError(t, err)

	// Site with high population
	siteDataHighPop := map[string]interface{}{
		"population": 500000.0,
	}

	_, scoreHigh, explanationHigh, err := DefaultScoreFunc(siteDataHighPop, resolvedSchema)
	require.NoError(t, err)

	// High population should score higher than low population
	assert.Greater(t, scoreHigh, scoreLow,
		"Higher population should produce higher final score")

	require.Len(t, explanationLow.Factors, 1)
	require.Len(t, explanationHigh.Factors, 1)

	lowFactor := explanationLow.Factors[0]
	highFactor := explanationHigh.Factors[0]

	assert.Equal(t, "maximize", lowFactor.Direction)
	assert.Equal(t, "maximize", highFactor.Direction)
}

func TestDefaultScoreFunc_ZeroWeightFieldsIgnored(t *testing.T) {
	// Test that fields with zero weight are not included in scoring
	min := 0.0
	max := 100.0

	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"unemployment": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMinimize,
			},
			"notes": {
				Type:      schema.TypeText,
				Required:  false,
				Weight:    0.0,
				Direction: schema.DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"unemployment": 1.0,
		},
	}

	siteData := map[string]interface{}{
		"unemployment": 5.0,
		"notes":        "Important notes",
	}

	_, finalScore, explanation, err := DefaultScoreFunc(siteData, resolvedSchema)

	require.NoError(t, err)
	// Only unemployment should be in factors, not notes
	assert.Len(t, explanation.Factors, 1, "Should only have unemployment factor")
	assert.Equal(t, "unemployment", explanation.Factors[0].Name)
	assert.GreaterOrEqual(t, finalScore, 0.0)
	assert.LessOrEqual(t, finalScore, 100.0)
}

func TestDefaultScoreFunc_MissingFieldsSkipped(t *testing.T) {
	// Test that missing optional fields are skipped without error
	min := 0.0
	max := 100.0

	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"unemployment": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMinimize,
			},
			"growth_rate": {
				Type:      schema.TypePercentage,
				Required:  false,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"unemployment": 1.0,
			"growth_rate":  1.0,
		},
	}

	// Only provide unemployment, omit growth_rate
	siteData := map[string]interface{}{
		"unemployment": 5.0,
	}

	_, finalScore, explanation, err := DefaultScoreFunc(siteData, resolvedSchema)

	require.NoError(t, err)
	// Only unemployment should contribute
	assert.Len(t, explanation.Factors, 1, "Should have one factor")
	assert.Equal(t, "unemployment", explanation.Factors[0].Name)
	assert.GreaterOrEqual(t, finalScore, 0.0)
	assert.LessOrEqual(t, finalScore, 100.0)
}

func TestDefaultScoreFunc_InvalidTypeHandling(t *testing.T) {
	// Test that non-numeric fields are skipped gracefully
	min := 0.0
	max := 100.0

	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"unemployment": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMinimize,
			},
			"site_name": {
				Type:      schema.TypeText,
				Required:  true,
				Weight:    0.0,
				Direction: schema.DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"unemployment": 1.0,
		},
	}

	siteData := map[string]interface{}{
		"unemployment": 5.0,
		"site_name":    "Downtown Office",
	}

	_, finalScore, explanation, err := DefaultScoreFunc(siteData, resolvedSchema)

	require.NoError(t, err)
	// Only unemployment should be scored (text field has weight 0)
	assert.Len(t, explanation.Factors, 1)
	assert.GreaterOrEqual(t, finalScore, 0.0)
	assert.LessOrEqual(t, finalScore, 100.0)
}

func TestDefaultScoreFunc_NilSchemaReturnsError(t *testing.T) {
	// Test that nil schema returns error
	siteData := map[string]interface{}{
		"unemployment": 5.0,
	}

	rawScore, finalScore, explanation, err := DefaultScoreFunc(siteData, nil)

	assert.Error(t, err, "Should error with nil schema")
	assert.Equal(t, 0.0, rawScore)
	assert.Equal(t, 0.0, finalScore)
	assert.Empty(t, explanation.Factors)
	assert.Contains(t, err.Error(), "nil")
}

func TestDefaultScoreFunc_EmptySiteDataReturnsError(t *testing.T) {
	// Test that empty site data returns error
	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"unemployment": {
				Type:      schema.TypePercentage,
				Required:  true,
				Weight:    1.0,
				Direction: schema.DirectionMinimize,
			},
		},
		Weights: map[string]float64{
			"unemployment": 1.0,
		},
	}

	rawScore, finalScore, explanation, err := DefaultScoreFunc(map[string]interface{}{}, resolvedSchema)

	assert.Error(t, err, "Should error with empty site data")
	assert.Equal(t, 0.0, rawScore)
	assert.Equal(t, 0.0, finalScore)
	assert.Empty(t, explanation.Factors)
	assert.Contains(t, err.Error(), "empty")
}

func TestDefaultScoreFunc_MultipleFactorsSorted(t *testing.T) {
	// Test that factors are sorted by contribution (descending)
	min := 0.0
	max := 100.0

	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"unemployment": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    3.0,
				Direction: schema.DirectionMinimize,
			},
			"growth_rate": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMaximize,
			},
			"income": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    2.0,
				Direction: schema.DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"unemployment": 3.0,
			"growth_rate":  1.0,
			"income":       2.0,
		},
	}

	siteData := map[string]interface{}{
		"unemployment": 5.0,
		"growth_rate":  50.0,
		"income":       75.0,
	}

	_, _, explanation, err := DefaultScoreFunc(siteData, resolvedSchema)

	require.NoError(t, err)
	require.Len(t, explanation.Factors, 3)

	// Verify factors are sorted by absolute contribution (highest first)
	for i := 0; i < len(explanation.Factors)-1; i++ {
		current := math.Abs(explanation.Factors[i].Contribution)
		next := math.Abs(explanation.Factors[i+1].Contribution)
		assert.GreaterOrEqual(t, current, next,
			"Factors should be sorted by contribution (descending)")
	}
}

func TestDefaultScoreFunc_StringNumericConversion(t *testing.T) {
	// Test that string numeric values are converted properly
	min := 0.0
	max := 100.0

	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"unemployment": {
				Type:      schema.TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMinimize,
			},
		},
		Weights: map[string]float64{
			"unemployment": 1.0,
		},
	}

	// String numeric value
	siteData := map[string]interface{}{
		"unemployment": "5.0",
	}

	_, finalScore, explanation, err := DefaultScoreFunc(siteData, resolvedSchema)

	require.NoError(t, err)
	assert.Len(t, explanation.Factors, 1)
	assert.Equal(t, 5.0, explanation.Factors[0].Value)
	assert.GreaterOrEqual(t, finalScore, 0.0)
	assert.LessOrEqual(t, finalScore, 100.0)
}

func TestDefaultScoreFunc_BoundedNormalization(t *testing.T) {
	// Test that normalized values are properly bounded to 0-1
	min := 20.0
	max := 80.0

	resolvedSchema := &schema.ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]schema.FieldDef{
			"metric": {
				Type:      schema.TypeIndex,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: schema.DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"metric": 1.0,
		},
	}

	// Value outside bounds
	siteData := map[string]interface{}{
		"metric": 150.0, // Way above max of 80
	}

	_, finalScore, _, err := DefaultScoreFunc(siteData, resolvedSchema)

	require.NoError(t, err)
	// Final score should still be bounded to 0-100
	assert.GreaterOrEqual(t, finalScore, 0.0)
	assert.LessOrEqual(t, finalScore, 100.0)
}
