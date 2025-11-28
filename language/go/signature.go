package golang

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// ExtractSignatures extracts function signatures from all queries in a suite.
// It uses the query analyzer to extract parameters and return fields,
// then maps them to Go types using the schema when available.
func ExtractSignatures(suite *scaf.Suite, analyzer scaf.QueryAnalyzer, schema *analysis.TypeSchema) ([]*FuncSignature, error) {
	if suite == nil {
		return nil, nil
	}

	signatures := make([]*FuncSignature, 0, len(suite.Queries))

	for _, query := range suite.Queries {
		sig, err := ExtractSignature(query, analyzer, schema)
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", query.Name, err)
		}

		signatures = append(signatures, sig)
	}

	return signatures, nil
}

// ExtractSignature extracts a function signature from a single query.
func ExtractSignature(query *scaf.Query, analyzer scaf.QueryAnalyzer, schema *analysis.TypeSchema) (*FuncSignature, error) {
	sig := &FuncSignature{
		Name:         toExportedName(query.Name),
		Query:        query.Body,
		QueryName:    query.Name,
		Params:       []FuncParam{},
		Returns:      []FuncReturn{},
		ReturnsSlice: true, // Default to slice (safe)
	}

	// If no analyzer, we can only provide basic signature
	if analyzer == nil {
		return sig, nil
	}

	// Try to use schema-aware analyzer for cardinality inference
	var metadata *scaf.QueryMetadata
	var err error

	if schemaAnalyzer, ok := analyzer.(analysis.SchemaAwareAnalyzer); ok && schema != nil {
		metadata, err = schemaAnalyzer.AnalyzeQueryWithSchema(query.Body, schema)
	} else {
		metadata, err = analyzer.AnalyzeQuery(query.Body)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to analyze query: %w", err)
	}

	if metadata == nil {
		return sig, nil
	}

	// Set cardinality from metadata
	sig.ReturnsSlice = !metadata.ReturnsOne

	// Convert parameters
	for _, param := range metadata.Parameters {
		funcParam := FuncParam{
			Name:     param.Name,
			Type:     inferParamType(param, schema),
			Required: true, // Parameters are required by default
		}
		sig.Params = append(sig.Params, funcParam)
	}

	// Convert returns
	for _, ret := range metadata.Returns {
		// Skip wildcards - they need special handling
		if ret.IsWildcard {
			continue
		}

		// Determine column name: if there's an alias, column = Alias.
		// If no alias, column = Expression.
		columnName := ret.Alias
		if columnName == "" {
			columnName = ret.Expression
		}
		if columnName == "" {
			columnName = ret.Name // fallback
		}

		funcReturn := FuncReturn{
			Name:       ret.Name,
			ColumnName: columnName,
			Type:       inferReturnType(ret, schema),
			IsSlice:    sig.ReturnsSlice,
		}
		sig.Returns = append(sig.Returns, funcReturn)
	}

	// Generate result struct name if there are multiple returns
	if len(sig.Returns) > 1 {
		sig.ResultStruct = toResultStructName(sig.Name)
	}

	return sig, nil
}

// toExportedName converts a query name to an exported Go function name.
// Examples:
//
//	"getUserById" -> "GetUserById"
//	"get_user_by_id" -> "GetUserByID"
//	"GetUser" -> "GetUser"
//	"id" -> "ID"
func toExportedName(name string) string {
	if name == "" {
		return ""
	}

	// Handle snake_case
	if strings.Contains(name, "_") {
		return snakeToPascal(name)
	}

	// Check if it's a known acronym (case insensitive)
	upper := strings.ToUpper(name)
	if isAcronym(upper) {
		return upper
	}

	// Already PascalCase or camelCase - just ensure first letter is uppercase
	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}

// toExportedFieldName converts a database field name to an exported Go struct field name.
// This is an alias for toExportedName as they use the same conversion logic.
// Examples:
//
//	"name" -> "Name"
//	"user_id" -> "UserID"
//	"email" -> "Email"
func toExportedFieldName(name string) string {
	return toExportedName(name)
}

// snakeToPascal converts snake_case to PascalCase.
func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Handle common acronyms
		upper := strings.ToUpper(part)
		if isAcronym(upper) {
			result = append(result, upper)
		} else {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			result = append(result, string(runes))
		}
	}

	return strings.Join(result, "")
}

// isAcronym returns true if the string is a common acronym.
func isAcronym(s string) bool {
	acronyms := map[string]bool{
		"ID":   true,
		"URL":  true,
		"API":  true,
		"HTTP": true,
		"JSON": true,
		"XML":  true,
		"SQL":  true,
		"UUID": true,
		"DB":   true,
	}

	return acronyms[s]
}

// inferParamType infers the Go type for a query parameter.
// Uses the analyzer's type (from schema-aware analysis) or falls back to schema lookup.
func inferParamType(param scaf.ParameterInfo, schema *analysis.TypeSchema) string {
	// If the analyzer already inferred the type from schema, use it directly
	// (The Cypher analyzer returns Go type strings like "string", "int", "[]string")
	if param.Type != "" {
		return param.Type
	}

	// Fallback: try to find the type from the schema by looking up fields with matching names
	// This is a best-effort approach when the analyzer couldn't determine the model context
	if schema != nil {
		if fieldType := lookupFieldType(param.Name, schema); fieldType != nil {
			return fieldType.String()
		}
	}

	return "any"
}

// inferReturnType infers the Go type for a return field.
// Uses the analyzer's type (from schema-aware analysis) or falls back to schema lookup.
func inferReturnType(ret scaf.ReturnInfo, schema *analysis.TypeSchema) string {
	// If the analyzer already inferred the type from schema, use it directly
	// (The Cypher analyzer returns Go type strings like "string", "int", "*User")
	if ret.Type != "" {
		return ret.Type
	}

	// Fallback: try to find the type from the schema using the parsed name
	// (The analyzer already extracts the field name from expressions like "u.name")
	if schema != nil {
		if fieldType := lookupFieldType(ret.Name, schema); fieldType != nil {
			return fieldType.String()
		}
	}

	return "any"
}

// lookupFieldType searches the schema for a field with the given name.
// Returns the field's type if found, nil otherwise.
func lookupFieldType(fieldName string, schema *analysis.TypeSchema) *analysis.Type {
	if schema == nil || fieldName == "" {
		return nil
	}

	// Search all models for a matching field
	for _, model := range schema.Models {
		for _, field := range model.Fields {
			if field.Name == fieldName {
				return field.Type
			}
		}
	}

	return nil
}

// LookupFieldTypeInModel looks up a field type in a specific model.
func LookupFieldTypeInModel(modelName, fieldName string, schema *analysis.TypeSchema) *analysis.Type {
	if schema == nil {
		return nil
	}

	model, ok := schema.Models[modelName]
	if !ok {
		return nil
	}

	for _, field := range model.Fields {
		if field.Name == fieldName {
			return field.Type
		}
	}

	return nil
}

// TypeToGoString converts an analysis.Type to a Go type string.
func TypeToGoString(t *analysis.Type) string {
	if t == nil {
		return "any"
	}

	return t.String()
}

// toResultStructName generates the private result struct name from a function name.
// Examples:
//
//	"GetUser" -> "getUserResult"
//	"FindAllPosts" -> "findAllPostsResult"
func toResultStructName(funcName string) string {
	if funcName == "" {
		return ""
	}

	// Lowercase first letter to make it private, then add Result suffix
	runes := []rune(funcName)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes) + "Result"
}
