package schema

import (
	"fmt"
	"strconv"
	"strings"
)

// ValidateHeaders checks that required columns are present and flags unexpected columns
func ValidateHeaders(headers []string, schema *ResolvedSchema) (warnings []string, errors []string) {
	headerSet := make(map[string]bool)
	for _, h := range headers {
		headerSet[h] = true
	}

	// Check for required fields
	for fieldName, fieldDef := range schema.Fields {
		if fieldDef.Required {
			if !headerSet[fieldName] {
				errors = append(errors, fmt.Sprintf("required field '%s' not found in headers", fieldName))
			}
		}
	}

	// Check for site_id_column
	if !headerSet[schema.SiteIDColumn] {
		errors = append(errors, fmt.Sprintf("site_id_column '%s' not found in headers", schema.SiteIDColumn))
	}

	// Flag unexpected columns (headers that don't match any defined field and aren't the site_id_column)
	for _, header := range headers {
		if header == schema.SiteIDColumn {
			continue
		}
		if _, exists := schema.Fields[header]; !exists {
			warnings = append(warnings, fmt.Sprintf("unexpected column '%s' found in CSV; will be included in record data but not validated", header))
		}
	}

	return warnings, errors
}

// ValidateRow validates each field value in a row against type and range constraints
func ValidateRow(row map[string]string, schema *ResolvedSchema, rowNum int) (warnings []string, errors []string) {
	// Validate each defined field
	for fieldName, fieldDef := range schema.Fields {
		value, exists := row[fieldName]

		// Check if required field is present
		if fieldDef.Required && !exists {
			errors = append(errors, fmt.Sprintf("row %d: required field '%s' is missing", rowNum, fieldName))
			continue
		}

		// Skip validation if field is not required and not present
		if !exists {
			continue
		}

		// Skip validation if field is empty and not required
		if value == "" && !fieldDef.Required {
			continue
		}

		// Validate the value based on type
		if err := validateFieldValue(fieldName, value, fieldDef, rowNum); err != nil {
			errors = append(errors, err.Error())
		}
	}

	return warnings, errors
}

// validateFieldValue validates a single field value against its constraints
func validateFieldValue(fieldName, value string, fieldDef FieldDef, rowNum int) error {
	switch fieldDef.Type {
	case TypePercentage:
		return validatePercentage(fieldName, value, rowNum)
	case TypeIndex:
		return validateIndex(fieldName, value, fieldDef, rowNum)
	case TypeInteger:
		return validateInteger(fieldName, value, fieldDef, rowNum)
	case TypeNumeric:
		return validateNumeric(fieldName, value, fieldDef, rowNum)
	case TypePopulation:
		return validatePopulation(fieldName, value, rowNum)
	case TypeText:
		return validateText(fieldName, value, rowNum)
	case TypeIdentifier:
		return validateIdentifier(fieldName, value, rowNum)
	default:
		return fmt.Errorf("row %d: unknown field type '%s' for field '%s'", rowNum, fieldDef.Type, fieldName)
	}
}

// validatePercentage validates percentage fields (0-100)
func validatePercentage(fieldName, value string, rowNum int) error {
	floatVal, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("row %d: field '%s' must be a valid number, got '%s'", rowNum, fieldName, value)
	}

	if floatVal < 0 || floatVal > 100 {
		return fmt.Errorf("row %d: field '%s' must be between 0 and 100, got %v", rowNum, fieldName, floatVal)
	}

	return nil
}

// validateIndex validates index fields with optional min/max bounds
func validateIndex(fieldName, value string, fieldDef FieldDef, rowNum int) error {
	floatVal, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("row %d: field '%s' must be a valid number, got '%s'", rowNum, fieldName, value)
	}

	if fieldDef.Min != nil && floatVal < *fieldDef.Min {
		return fmt.Errorf("row %d: field '%s' must be >= %v, got %v", rowNum, fieldName, *fieldDef.Min, floatVal)
	}

	if fieldDef.Max != nil && floatVal > *fieldDef.Max {
		return fmt.Errorf("row %d: field '%s' must be <= %v, got %v", rowNum, fieldName, *fieldDef.Max, floatVal)
	}

	return nil
}

// validateInteger validates integer fields with optional min/max bounds
func validateInteger(fieldName, value string, fieldDef FieldDef, rowNum int) error {
	// Check if value is a whole number
	floatVal, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("row %d: field '%s' must be a valid number, got '%s'", rowNum, fieldName, value)
	}

	if floatVal != float64(int64(floatVal)) {
		return fmt.Errorf("row %d: field '%s' must be a whole number, got %v", rowNum, fieldName, floatVal)
	}

	if fieldDef.Min != nil && floatVal < *fieldDef.Min {
		return fmt.Errorf("row %d: field '%s' must be >= %v, got %v", rowNum, fieldName, *fieldDef.Min, floatVal)
	}

	if fieldDef.Max != nil && floatVal > *fieldDef.Max {
		return fmt.Errorf("row %d: field '%s' must be <= %v, got %v", rowNum, fieldName, *fieldDef.Max, floatVal)
	}

	return nil
}

// validateNumeric validates numeric fields (any number) with optional min/max bounds
func validateNumeric(fieldName, value string, fieldDef FieldDef, rowNum int) error {
	floatVal, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("row %d: field '%s' must be a valid number, got '%s'", rowNum, fieldName, value)
	}

	if fieldDef.Min != nil && floatVal < *fieldDef.Min {
		return fmt.Errorf("row %d: field '%s' must be >= %v, got %v", rowNum, fieldName, *fieldDef.Min, floatVal)
	}

	if fieldDef.Max != nil && floatVal > *fieldDef.Max {
		return fmt.Errorf("row %d: field '%s' must be <= %v, got %v", rowNum, fieldName, *fieldDef.Max, floatVal)
	}

	return nil
}

// validatePopulation validates population fields (non-negative integers)
func validatePopulation(fieldName, value string, rowNum int) error {
	floatVal, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("row %d: field '%s' must be a valid number, got '%s'", rowNum, fieldName, value)
	}

	if floatVal != float64(int64(floatVal)) {
		return fmt.Errorf("row %d: field '%s' must be a whole number, got %v", rowNum, fieldName, floatVal)
	}

	if floatVal < 0 {
		return fmt.Errorf("row %d: field '%s' must be non-negative, got %v", rowNum, fieldName, floatVal)
	}

	return nil
}

// validateText validates text fields (any string is valid)
func validateText(fieldName, value string, rowNum int) error {
	// Text fields accept any string, so we just check that it's not empty if required
	// But the required check is already done in ValidateRow
	return nil
}

// validateIdentifier validates identifier fields (non-empty strings)
func validateIdentifier(fieldName, value string, rowNum int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("row %d: field '%s' (identifier) cannot be empty", rowNum, fieldName)
	}

	return nil
}
