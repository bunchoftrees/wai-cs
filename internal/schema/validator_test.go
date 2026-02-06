package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateHeaders_AllRequired(t *testing.T) {
	// Test that all required headers present passes validation
	min := 0.0
	max := 100.0

	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"site_id": {
				Type:      TypeIdentifier,
				Required:  true,
				Weight:    0,
				Direction: DirectionMaximize,
			},
			"population": {
				Type:      TypePopulation,
				Required:  true,
				Weight:    1.5,
				Direction: DirectionMaximize,
			},
			"unemployment": {
				Type:      TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    2.0,
				Direction: DirectionMinimize,
			},
		},
		Weights: map[string]float64{
			"population":   1.5,
			"unemployment": 2.0,
		},
	}

	headers := []string{"site_id", "population", "unemployment"}
	warnings, errors := ValidateHeaders(headers, schema)

	assert.Empty(t, errors, "Should have no errors with all required headers")
	assert.Empty(t, warnings, "Should have no warnings with only required headers")
}

func TestValidateHeaders_MissingRequired(t *testing.T) {
	// Test that missing required column returns error
	min := 0.0
	max := 100.0

	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"site_id": {
				Type:      TypeIdentifier,
				Required:  true,
				Weight:    0,
				Direction: DirectionMaximize,
			},
			"population": {
				Type:      TypePopulation,
				Required:  true,
				Weight:    1.5,
				Direction: DirectionMaximize,
			},
			"unemployment": {
				Type:      TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    2.0,
				Direction: DirectionMinimize,
			},
		},
		Weights: map[string]float64{
			"population":   1.5,
			"unemployment": 2.0,
		},
	}

	// Missing unemployment column
	headers := []string{"site_id", "population"}
	warnings, errors := ValidateHeaders(headers, schema)

	assert.Empty(t, warnings, "Should have no warnings")
	assert.NotEmpty(t, errors, "Should have errors")
	assert.Len(t, errors, 1, "Should have exactly one error")
	assert.Contains(t, errors[0], "unemployment")
	assert.Contains(t, errors[0], "not found")
}

func TestValidateHeaders_MissingSiteIDColumn(t *testing.T) {
	// Test that missing site_id_column returns error
	schema := &ResolvedSchema{
		SiteIDColumn: "location_id",
		Fields: map[string]FieldDef{
			"population": {
				Type:      TypePopulation,
				Required:  true,
				Weight:    1.5,
				Direction: DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"population": 1.5,
		},
	}

	headers := []string{"site_name", "population"}
	warnings, errors := ValidateHeaders(headers, schema)

	assert.Empty(t, warnings, "Should have no warnings")
	assert.NotEmpty(t, errors, "Should have errors")
	assert.Contains(t, errors[0], "location_id")
	assert.Contains(t, errors[0], "not found")
}

func TestValidateHeaders_UnexpectedColumn(t *testing.T) {
	// Test that extra column returns warning, not error
	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"site_id": {
				Type:      TypeIdentifier,
				Required:  true,
				Weight:    0,
				Direction: DirectionMaximize,
			},
			"population": {
				Type:      TypePopulation,
				Required:  true,
				Weight:    1.5,
				Direction: DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"population": 1.5,
		},
	}

	// Extra columns: extra_field1 and extra_field2
	headers := []string{"site_id", "population", "extra_field1", "extra_field2"}
	warnings, errors := ValidateHeaders(headers, schema)

	assert.Empty(t, errors, "Should have no errors for unexpected columns")
	assert.NotEmpty(t, warnings, "Should have warnings for unexpected columns")
	assert.Len(t, warnings, 2, "Should have exactly two warnings")
	assert.Contains(t, warnings[0], "extra_field1")
	assert.Contains(t, warnings[1], "extra_field2")
}

func TestValidateRow_ValidData(t *testing.T) {
	// Test that valid row passes all checks
	min := 0.0
	max := 100.0
	minPop := 0.0
	maxPop := 1000000.0

	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"site_id": {
				Type:      TypeIdentifier,
				Required:  true,
				Weight:    0,
				Direction: DirectionMaximize,
			},
			"population": {
				Type:      TypePopulation,
				Required:  true,
				Min:       &minPop,
				Max:       &maxPop,
				Weight:    1.5,
				Direction: DirectionMaximize,
			},
			"unemployment": {
				Type:      TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    2.0,
				Direction: DirectionMinimize,
			},
			"growth_rate": {
				Type:      TypePercentage,
				Required:  false,
				Min:       &min,
				Max:       &max,
				Weight:    1.0,
				Direction: DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"population":   1.5,
			"unemployment": 2.0,
			"growth_rate":  1.0,
		},
	}

	row := map[string]string{
		"site_id":      "SITE-001",
		"population":   "50000",
		"unemployment": "5.5",
		"growth_rate":  "3.2",
	}

	warnings, errors := ValidateRow(row, schema, 1)

	assert.Empty(t, errors, "Should have no errors with valid data")
	assert.Empty(t, warnings, "Should have no warnings")
}

func TestValidateRow_InvalidPercentage(t *testing.T) {
	// Test that percentage > 100 returns error
	min := 0.0
	max := 100.0

	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"unemployment": {
				Type:      TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    2.0,
				Direction: DirectionMinimize,
			},
		},
		Weights: map[string]float64{
			"unemployment": 2.0,
		},
	}

	row := map[string]string{
		"unemployment": "105.5",
	}

	warnings, errors := ValidateRow(row, schema, 5)

	assert.Empty(t, warnings, "Should have no warnings")
	assert.NotEmpty(t, errors, "Should have errors")
	assert.Contains(t, errors[0], "between 0 and 100")
	assert.Contains(t, errors[0], "row 5")
}

func TestValidateRow_InvalidPercentageNegative(t *testing.T) {
	// Test that negative percentage returns error
	min := 0.0
	max := 100.0

	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"unemployment": {
				Type:      TypePercentage,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    2.0,
				Direction: DirectionMinimize,
			},
		},
		Weights: map[string]float64{
			"unemployment": 2.0,
		},
	}

	row := map[string]string{
		"unemployment": "-5.0",
	}

	_, errors := ValidateRow(row, schema, 2)

	assert.NotEmpty(t, errors, "Should have errors")
	assert.Contains(t, errors[0], "between 0 and 100")
}

func TestValidateRow_EmptyOptional(t *testing.T) {
	// Test that empty optional field passes validation
	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"site_id": {
				Type:      TypeIdentifier,
				Required:  true,
				Weight:    0,
				Direction: DirectionMaximize,
			},
			"optional_notes": {
				Type:      TypeText,
				Required:  false,
				Weight:    0,
				Direction: DirectionMaximize,
			},
		},
		Weights: map[string]float64{},
	}

	row := map[string]string{
		"site_id":         "SITE-002",
		"optional_notes":  "",
	}

	warnings, errors := ValidateRow(row, schema, 1)

	assert.Empty(t, errors, "Should have no errors with empty optional field")
	assert.Empty(t, warnings, "Should have no warnings")
}

func TestValidateRow_MissingRequired(t *testing.T) {
	// Test that missing required field returns error
	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"site_id": {
				Type:      TypeIdentifier,
				Required:  true,
				Weight:    0,
				Direction: DirectionMaximize,
			},
			"population": {
				Type:      TypePopulation,
				Required:  true,
				Weight:    1.5,
				Direction: DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"population": 1.5,
		},
	}

	row := map[string]string{
		"site_id": "SITE-003",
	}

	_, errors := ValidateRow(row, schema, 3)

	assert.NotEmpty(t, errors, "Should have errors")
	assert.Contains(t, errors[0], "population")
	assert.Contains(t, errors[0], "missing")
	assert.Contains(t, errors[0], "row 3")
}

func TestValidateRow_InvalidNumericFormat(t *testing.T) {
	// Test that invalid numeric format returns error
	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"population": {
				Type:      TypePopulation,
				Required:  true,
				Weight:    1.5,
				Direction: DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"population": 1.5,
		},
	}

	row := map[string]string{
		"population": "not_a_number",
	}

	_, errors := ValidateRow(row, schema, 2)

	assert.NotEmpty(t, errors, "Should have errors")
	assert.Contains(t, errors[0], "valid number")
}

func TestValidateRow_IntegerFieldWithDecimal(t *testing.T) {
	// Test that integer field with decimal value returns error
	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"num_employees": {
				Type:      TypeInteger,
				Required:  true,
				Weight:    1.0,
				Direction: DirectionMaximize,
			},
		},
		Weights: map[string]float64{
			"num_employees": 1.0,
		},
	}

	row := map[string]string{
		"num_employees": "1500.5",
	}

	_, errors := ValidateRow(row, schema, 4)

	assert.NotEmpty(t, errors, "Should have errors")
	assert.Contains(t, errors[0], "whole number")
}

func TestValidateRow_IndexWithBounds(t *testing.T) {
	// Test that index field respects min/max bounds
	min := 50.0
	max := 150.0

	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"cost_index": {
				Type:      TypeIndex,
				Required:  true,
				Min:       &min,
				Max:       &max,
				Weight:    2.0,
				Direction: DirectionMinimize,
			},
		},
		Weights: map[string]float64{
			"cost_index": 2.0,
		},
	}

	// Below minimum
	rowBelowMin := map[string]string{
		"cost_index": "40.0",
	}
	_, errors := ValidateRow(rowBelowMin, schema, 1)
	assert.NotEmpty(t, errors, "Should error when below minimum")
	assert.Contains(t, errors[0], "must be >= 50")

	// Above maximum
	rowAboveMax := map[string]string{
		"cost_index": "160.0",
	}
	_, errors = ValidateRow(rowAboveMax, schema, 2)
	assert.NotEmpty(t, errors, "Should error when above maximum")
	assert.Contains(t, errors[0], "must be <= 150")

	// Within bounds
	rowValid := map[string]string{
		"cost_index": "100.0",
	}
	_, errors = ValidateRow(rowValid, schema, 3)
	assert.Empty(t, errors, "Should pass when within bounds")
}

func TestValidateRow_IdentifierCannotBeEmpty(t *testing.T) {
	// Test that identifier field cannot be empty
	schema := &ResolvedSchema{
		SiteIDColumn: "site_id",
		Fields: map[string]FieldDef{
			"site_id": {
				Type:      TypeIdentifier,
				Required:  true,
				Weight:    0,
				Direction: DirectionMaximize,
			},
		},
		Weights: map[string]float64{},
	}

	row := map[string]string{
		"site_id": "   ",
	}

	_, errors := ValidateRow(row, schema, 1)

	assert.NotEmpty(t, errors, "Should have errors")
	assert.Contains(t, errors[0], "cannot be empty")
}
