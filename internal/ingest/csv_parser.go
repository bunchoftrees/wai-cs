package ingest

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"

	"github.com/workforce-ai/site-selection-iq/internal/schema"
)

// Parse reads and validates a CSV file, returning site records, validation warnings, and any fatal errors.
// Warnings are non-fatal (e.g., unexpected columns, skipped rows). Errors are fatal (e.g., missing required columns).
func Parse(reader io.Reader, schemaConfig *schema.ResolvedSchema) (
	records []json.RawMessage,
	warnings []string,
	err error,
) {
	records = make([]json.RawMessage, 0)
	warnings = make([]string, 0)

	// Create CSV reader
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1 // Allow variable number of fields

	// Read header row
	headers, err := csvReader.Read()
	if err != nil {
		if err == io.EOF {
			return records, warnings, fmt.Errorf("CSV file is empty")
		}
		return records, warnings, fmt.Errorf("failed to read CSV headers: %v", err)
	}

	// Validate headers
	headerWarnings, headerErrors := schema.ValidateHeaders(headers, schemaConfig)
	warnings = append(warnings, headerWarnings...)

	// If there are critical header errors, return early
	if len(headerErrors) > 0 {
		return records, warnings, fmt.Errorf("header validation failed: %v", headerErrors)
	}

	lineNum := 2 // Start at line 2 since line 1 is headers

	// Process data rows
	for {
		csvRow, err := csvReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return records, warnings, fmt.Errorf("line %d: failed to read CSV row: %v", lineNum, err)
		}

		// Convert CSV row to map
		rowMap := make(map[string]string)
		for i, header := range headers {
			if i < len(csvRow) {
				rowMap[header] = csvRow[i]
			} else {
				rowMap[header] = ""
			}
		}

		// Validate row
		rowWarnings, rowErrors := schema.ValidateRow(rowMap, schemaConfig, lineNum)
		warnings = append(warnings, rowWarnings...)

		// Skip rows with validation errors (capture as warnings for reporting)
		if len(rowErrors) > 0 {
			for _, re := range rowErrors {
				warnings = append(warnings, fmt.Sprintf("row %d skipped: %s", lineNum, re))
			}
			lineNum++
			continue
		}

		// Extract site_id
		siteID, exists := rowMap[schemaConfig.SiteIDColumn]
		if !exists || siteID == "" {
			lineNum++
			continue
		}

		// Build JSONB data field with all columns
		dataMap := make(map[string]interface{})
		for header, value := range rowMap {
			dataMap[header] = value
		}

		dataJSON, err := json.Marshal(dataMap)
		if err != nil {
			lineNum++
			continue
		}

		records = append(records, dataJSON)
		lineNum++
	}

	return records, warnings, nil
}
