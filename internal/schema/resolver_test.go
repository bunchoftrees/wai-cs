package schema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_GlobalOnly(t *testing.T) {
	// Test that resolving with only global config works correctly
	globalConfig := `{
		"site_id_column": "site_id",
		"fields": {
			"population": {
				"type": "population",
				"required": true,
				"weight": 1.5,
				"direction": "maximize",
				"description": "Site population"
			},
			"unemployment": {
				"type": "percentage",
				"required": true,
				"weight": 2.0,
				"direction": "minimize",
				"description": "Unemployment rate"
			}
		}
	}`

	resolved, err := Resolve(json.RawMessage(globalConfig), nil)

	require.NoError(t, err, "Should not error with valid global config")
	assert.NotNil(t, resolved)
	assert.Equal(t, "site_id", resolved.SiteIDColumn)
	assert.Len(t, resolved.Fields, 2, "Should have 2 fields")
	assert.Len(t, resolved.Weights, 2, "Should have 2 weights")

	// Verify fields are present and weights are set correctly
	assert.Equal(t, 1.5, resolved.Weights["population"])
	assert.Equal(t, 2.0, resolved.Weights["unemployment"])

	// Verify field definitions
	popField := resolved.Fields["population"]
	assert.Equal(t, TypePopulation, popField.Type)
	assert.True(t, popField.Required)
	assert.Equal(t, DirectionMaximize, popField.Direction)
}

func TestResolve_WithTenantOverrides(t *testing.T) {
	// Test that tenant weight overrides and extra fields merge correctly
	globalConfig := `{
		"site_id_column": "site_id",
		"fields": {
			"population": {
				"type": "population",
				"required": true,
				"weight": 1.5,
				"direction": "maximize",
				"description": "Site population"
			},
			"unemployment": {
				"type": "percentage",
				"required": true,
				"weight": 2.0,
				"direction": "minimize",
				"description": "Unemployment rate"
			}
		}
	}`

	tenantConfig := `{
		"fields": {
			"growth_rate": {
				"type": "percentage",
				"required": false,
				"weight": 3.0,
				"direction": "maximize",
				"description": "Market growth rate"
			}
		},
		"weights": {
			"population": 2.5,
			"unemployment": 1.5
		}
	}`

	resolved, err := Resolve(json.RawMessage(globalConfig), json.RawMessage(tenantConfig))

	require.NoError(t, err, "Should not error with valid configs")
	assert.NotNil(t, resolved)
	assert.Equal(t, "site_id", resolved.SiteIDColumn)

	// Should have 3 fields: 2 from global + 1 from tenant override
	assert.Len(t, resolved.Fields, 3, "Should have 3 fields total")
	assert.Len(t, resolved.Weights, 3, "Should have 3 weights total")

	// Verify tenant weight overrides are applied
	assert.Equal(t, 2.5, resolved.Weights["population"], "Population weight should be overridden to 2.5")
	assert.Equal(t, 1.5, resolved.Weights["unemployment"], "Unemployment weight should be overridden to 1.5")

	// Verify new tenant field is added
	assert.Equal(t, 3.0, resolved.Weights["growth_rate"], "Growth rate should use field weight")
	growthField := resolved.Fields["growth_rate"]
	assert.Equal(t, TypePercentage, growthField.Type)
	assert.False(t, growthField.Required)
}

func TestResolve_TenantWeightsOverrideGlobal(t *testing.T) {
	// Test that specific weight override takes precedence over field weight
	globalConfig := `{
		"site_id_column": "location_id",
		"fields": {
			"revenue": {
				"type": "numeric",
				"required": true,
				"weight": 1.0,
				"direction": "maximize",
				"description": "Annual revenue"
			},
			"cost_index": {
				"type": "index",
				"required": true,
				"min": 50,
				"max": 150,
				"weight": 1.0,
				"direction": "minimize",
				"description": "Cost index"
			}
		}
	}`

	tenantConfig := `{
		"site_id_column": "custom_site_id",
		"weights": {
			"revenue": 5.0,
			"cost_index": 0.5
		}
	}`

	resolved, err := Resolve(json.RawMessage(globalConfig), json.RawMessage(tenantConfig))

	require.NoError(t, err, "Should not error with valid configs")
	assert.NotNil(t, resolved)

	// Site ID column should be overridden
	assert.Equal(t, "custom_site_id", resolved.SiteIDColumn, "Site ID column should use tenant override")

	// Weights should be overridden by tenant weights
	assert.Equal(t, 5.0, resolved.Weights["revenue"], "Revenue weight should be 5.0 from tenant override")
	assert.Equal(t, 0.5, resolved.Weights["cost_index"], "Cost index weight should be 0.5 from tenant override")

	// Fields should still have original definitions
	revenueField := resolved.Fields["revenue"]
	assert.Equal(t, TypeNumeric, revenueField.Type)
	assert.Equal(t, 1.0, revenueField.Weight, "Field weight should remain unchanged")
}

func TestResolve_GlobalConfigMissingRequired(t *testing.T) {
	// Test that missing required fields in global config returns error
	missingFieldsConfig := `{
		"fields": {
			"population": {
				"type": "population",
				"required": true,
				"weight": 1.5,
				"direction": "maximize",
				"description": "Site population"
			}
		}
	}`

	resolved, err := Resolve(json.RawMessage(missingFieldsConfig), nil)

	assert.Error(t, err, "Should error when site_id_column is missing")
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "site_id_column")
}

func TestResolve_GlobalConfigNoFields(t *testing.T) {
	// Test that missing fields in global config returns error
	noFieldsConfig := `{
		"site_id_column": "site_id",
		"fields": {}
	}`

	resolved, err := Resolve(json.RawMessage(noFieldsConfig), nil)

	assert.Error(t, err, "Should error when no fields are defined")
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "at least one field")
}

func TestResolve_TenantWeightNonexistentField(t *testing.T) {
	// Test that overriding weight for non-existent field returns error
	globalConfig := `{
		"site_id_column": "site_id",
		"fields": {
			"population": {
				"type": "population",
				"required": true,
				"weight": 1.5,
				"direction": "maximize",
				"description": "Site population"
			}
		}
	}`

	tenantConfig := `{
		"weights": {
			"nonexistent_field": 2.0
		}
	}`

	resolved, err := Resolve(json.RawMessage(globalConfig), json.RawMessage(tenantConfig))

	assert.Error(t, err, "Should error when trying to override weight for non-existent field")
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "nonexistent_field")
}

func TestResolve_InvalidGlobalJSON(t *testing.T) {
	// Test that invalid JSON in global config returns error
	invalidJSON := `{invalid json}`

	resolved, err := Resolve(json.RawMessage(invalidJSON), nil)

	assert.Error(t, err, "Should error on invalid JSON")
	assert.Nil(t, resolved)
}

func TestResolve_NullTenantConfig(t *testing.T) {
	// Test that null tenant config is treated as no override
	globalConfig := `{
		"site_id_column": "site_id",
		"fields": {
			"population": {
				"type": "population",
				"required": true,
				"weight": 1.5,
				"direction": "maximize",
				"description": "Site population"
			}
		}
	}`

	resolved, err := Resolve(json.RawMessage(globalConfig), json.RawMessage("null"))

	require.NoError(t, err, "Should not error with null tenant config")
	assert.NotNil(t, resolved)
	assert.Equal(t, "site_id", resolved.SiteIDColumn)
	assert.Equal(t, 1.5, resolved.Weights["population"])
}
