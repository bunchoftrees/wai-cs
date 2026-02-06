package schema

import (
	"context"
	"encoding/json"
	"fmt"
)

// FieldType represents the data type of a field
type FieldType string

const (
	TypePercentage   FieldType = "percentage"
	TypeIndex        FieldType = "index"
	TypeInteger      FieldType = "integer"
	TypeNumeric      FieldType = "numeric"
	TypePopulation   FieldType = "population"
	TypeText         FieldType = "text"
	TypeIdentifier   FieldType = "identifier"
)

// Direction represents whether a field value should be maximized or minimized
type Direction string

const (
	DirectionMaximize Direction = "maximize"
	DirectionMinimize Direction = "minimize"
)

// FieldDef defines the schema for a single field
type FieldDef struct {
	Type        FieldType  `json:"type"`
	Required    bool       `json:"required"`
	Min         *float64   `json:"min,omitempty"`
	Max         *float64   `json:"max,omitempty"`
	Weight      float64    `json:"weight"`
	Direction   Direction  `json:"direction"`
	Description string     `json:"description"`
}

// ResolvedSchema represents the final merged schema with all fields and weights
type ResolvedSchema struct {
	Fields        map[string]FieldDef `json:"fields"`
	SiteIDColumn  string              `json:"site_id_column"`
	Weights       map[string]float64  `json:"weights"`
}

// Resolver handles schema resolution logic
type Resolver struct{}

// NewResolver creates a new schema resolver
func NewResolver() *Resolver {
	return &Resolver{}
}

// Resolve resolves a schema using the resolver
func (r *Resolver) Resolve(ctx context.Context, config json.RawMessage, tenantConfig json.RawMessage) (*ResolvedSchema, error) {
	return Resolve(config, tenantConfig)
}

// GlobalSchemaConfig represents the global schema configuration
type GlobalSchemaConfig struct {
	Fields       map[string]FieldDef `json:"fields"`
	SiteIDColumn string              `json:"site_id_column"`
}

// TenantSchemaOverride represents tenant-specific schema overrides
type TenantSchemaOverride struct {
	Fields       map[string]FieldDef `json:"fields,omitempty"`
	SiteIDColumn *string             `json:"site_id_column,omitempty"`
	Weights      map[string]float64  `json:"weights,omitempty"`
}

// Resolve merges global defaults with tenant overrides to create a final resolved schema
func Resolve(globalConfig, tenantConfig json.RawMessage) (*ResolvedSchema, error) {
	// Parse global configuration
	var global GlobalSchemaConfig
	if err := json.Unmarshal(globalConfig, &global); err != nil {
		return nil, fmt.Errorf("failed to parse global schema config: %w", err)
	}

	// Validate global config has required fields
	if global.SiteIDColumn == "" {
		return nil, fmt.Errorf("global schema config must specify site_id_column")
	}
	if len(global.Fields) == 0 {
		return nil, fmt.Errorf("global schema config must specify at least one field")
	}

	// Initialize resolved schema with global defaults
	resolved := &ResolvedSchema{
		Fields:       make(map[string]FieldDef),
		SiteIDColumn: global.SiteIDColumn,
		Weights:      make(map[string]float64),
	}

	// Copy global fields
	for name, fieldDef := range global.Fields {
		resolved.Fields[name] = fieldDef
		resolved.Weights[name] = fieldDef.Weight
	}

	// Parse and apply tenant overrides if provided
	if len(tenantConfig) > 0 && string(tenantConfig) != "null" {
		var tenant TenantSchemaOverride
		if err := json.Unmarshal(tenantConfig, &tenant); err != nil {
			return nil, fmt.Errorf("failed to parse tenant schema override: %w", err)
		}

		// Override site_id_column if specified
		if tenant.SiteIDColumn != nil {
			resolved.SiteIDColumn = *tenant.SiteIDColumn
		}

		// Add or override fields from tenant config
		for name, fieldDef := range tenant.Fields {
			resolved.Fields[name] = fieldDef
			// Set weight from field definition
			resolved.Weights[name] = fieldDef.Weight
		}

		// Override weights from tenant-specific weight overrides
		for name, weight := range tenant.Weights {
			if _, exists := resolved.Fields[name]; !exists {
				return nil, fmt.Errorf("cannot override weight for non-existent field: %s", name)
			}
			resolved.Weights[name] = weight
		}
	}

	return resolved, nil
}
