package cypher

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
	cyphergrammar "github.com/rlch/scaf/dialects/cypher/grammar"
)

func init() {
	// Register the Cypher analyzer with the analyzer registry.
	// This allows the LSP and other tools to use the analyzer without
	// direct imports, enabling dialect-agnostic completion/hover.
	scaf.RegisterAnalyzer("cypher", func() scaf.QueryAnalyzer {
		return NewAnalyzer()
	})
}

// Analyzer implements scaf.QueryAnalyzer for Cypher queries.
type Analyzer struct{}

// NewAnalyzer creates a new Cypher query analyzer.
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// variableBinding tracks a variable's binding to a model (label).
type variableBinding struct {
	variable string   // e.g., "u"
	labels   []string // e.g., ["User"]
}

// queryContext holds context during query analysis.
type queryContext struct {
	bindings map[string]*variableBinding // variable name -> binding
	schema   *analysis.TypeSchema
}

func newQueryContext(schema *analysis.TypeSchema) *queryContext {
	return &queryContext{
		bindings: make(map[string]*variableBinding),
		schema:   schema,
	}
}

// AnalyzeQuery parses a Cypher query and extracts metadata.
func (a *Analyzer) AnalyzeQuery(query string) (*scaf.QueryMetadata, error) {
	return a.analyzeQueryInternal(query, nil)
}

// AnalyzeQueryWithSchema parses a Cypher query and extracts metadata with type inference.
// If schema is provided, it infers types for parameters and returns.
func (a *Analyzer) AnalyzeQueryWithSchema(query string, schema *analysis.TypeSchema) (*scaf.QueryMetadata, error) {
	return a.analyzeQueryInternal(query, schema)
}

// analyzeQueryInternal is the shared implementation for query analysis.
func (a *Analyzer) analyzeQueryInternal(query string, schema *analysis.TypeSchema) (*scaf.QueryMetadata, error) {
	tree, err := parseCypherQuery(query)
	if err != nil {
		return nil, err
	}

	ctx := newQueryContext(schema)
	result := &scaf.QueryMetadata{
		Parameters: []scaf.ParameterInfo{},
		Returns:    []scaf.ReturnInfo{},
	}

	// First pass: extract variable bindings from MATCH clauses
	extractBindings(tree, ctx)

	// Extract parameters with type inference
	extractParameters(tree, result, ctx)

	// Extract return items with type inference
	extractReturns(tree, result, ctx)

	// Check for unique field filters if schema is provided
	if schema != nil {
		result.ReturnsOne = checkUniqueFilter(tree, schema)
	}

	return result, nil
}

// parseCypherQuery parses a Cypher query string and returns the parse tree.
//
//nolint:unparam,ireturn // error is always nil; antlr.ParseTree is required interface return
func parseCypherQuery(query string) (antlr.ParseTree, error) {
	input := antlr.NewInputStream(query)
	lexer := cyphergrammar.NewCypherLexer(input)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := cyphergrammar.NewCypherParser(stream)

	// Set error listeners to track parsing errors
	errorListener := &parseErrorListener{}

	parser.RemoveErrorListeners()
	parser.AddErrorListener(errorListener)

	// Parse the script
	tree := parser.Script()

	if len(errorListener.errors) > 0 {
		// Return partial results even on parse errors - we still want completion
		// for partially valid queries
		return tree, nil
	}

	return tree, nil
}

// parseErrorListener tracks parse errors.
type parseErrorListener struct {
	errors []string
}

func (pel *parseErrorListener) SyntaxError(_ antlr.Recognizer, _ any, _, _ int, msg string, _ antlr.RecognitionException) {
	pel.errors = append(pel.errors, msg)
}

func (pel *parseErrorListener) ReportAmbiguity(_ antlr.Parser, _ *antlr.DFA, _, _ int, _ bool, _ *antlr.BitSet, _ *antlr.ATNConfigSet) {
}

func (pel *parseErrorListener) ReportAttemptingFullContext(_ antlr.Parser, _ *antlr.DFA, _, _ int, _ *antlr.BitSet, _ *antlr.ATNConfigSet) {
}

func (pel *parseErrorListener) ReportContextSensitivity(_ antlr.Parser, _ *antlr.DFA, _, _, _ int, _ *antlr.ATNConfigSet) {
}

// extractBindings walks the tree to find variable bindings from node patterns.
// E.g., MATCH (u:User) binds variable "u" to label "User".
func extractBindings(tree antlr.ParseTree, ctx *queryContext) {
	var walk func(node antlr.Tree)

	walk = func(node antlr.Tree) {
		if nodeCtx, ok := node.(*cyphergrammar.NodePatternContext); ok {
			extractNodeBinding(nodeCtx, ctx)
		}

		// Recursively walk children
		if ruleCtx, ok := node.(antlr.RuleContext); ok {
			for i := 0; i < ruleCtx.GetChildCount(); i++ {
				if child := ruleCtx.GetChild(i); child != nil {
					walk(child)
				}
			}
		}
	}

	walk(tree)
}

// extractNodeBinding extracts variable binding from a node pattern.
func extractNodeBinding(nodeCtx *cyphergrammar.NodePatternContext, ctx *queryContext) {
	// Get variable name
	var varName string
	if symbolCtx := nodeCtx.Symbol(); symbolCtx != nil {
		varName = symbolCtx.GetText()
	}

	if varName == "" {
		return
	}

	// Get labels
	var labels []string
	if labelsCtx := nodeCtx.NodeLabels(); labelsCtx != nil {
		for _, nameCtx := range labelsCtx.AllName() {
			if nameCtx != nil {
				labels = append(labels, nameCtx.GetText())
			}
		}
	}

	ctx.bindings[varName] = &variableBinding{
		variable: varName,
		labels:   labels,
	}
}

// extractParameters walks the tree to find all $parameters.
func extractParameters(tree antlr.ParseTree, result *scaf.QueryMetadata, ctx *queryContext) {
	indexByKey := make(map[string]int) // map name -> index in Parameters slice

	var walk func(node antlr.Tree, parentPropName string, parentLabels []string)

	walk = func(node antlr.Tree, parentPropName string, parentLabels []string) {
		// Check if this is a map pair (property: $param)
		if mapPairCtx, ok := node.(*cyphergrammar.MapPairContext); ok {
			propName := ""
			if nameCtx := mapPairCtx.Name(); nameCtx != nil {
				propName = nameCtx.GetText()
			}

			// Get labels from parent node pattern
			labels := getParentNodeLabels(mapPairCtx, ctx)

			// Walk children with property context
			if ruleCtx, ok := node.(antlr.RuleContext); ok {
				for i := 0; i < ruleCtx.GetChildCount(); i++ {
					if child := ruleCtx.GetChild(i); child != nil {
						walk(child, propName, labels)
					}
				}
			}

			return
		}

		if paramCtx, ok := node.(*cyphergrammar.ParameterContext); ok {
			// Extract parameter name from Symbol() or NumLit()
			var paramName string

			startToken := paramCtx.GetStart()
			position := startToken.GetStart()
			line := startToken.GetLine()
			column := startToken.GetColumn() + 1 // ANTLR is 0-based, we want 1-based

			if symbol := paramCtx.Symbol(); symbol != nil {
				paramName = symbol.GetText()
			} else if numLit := paramCtx.NumLit(); numLit != nil {
				paramName = numLit.GetText()
			}

			if paramName != "" {
				// Length includes the $ prefix
				length := len(paramName) + 1

				// Infer type from schema if we know the property and model
				paramType := inferParameterType(parentPropName, parentLabels, ctx)

				if idx, exists := indexByKey[paramName]; exists {
					result.Parameters[idx].Count++
					// Update type if we found one and didn't have one before
					if paramType != "" && result.Parameters[idx].Type == "" {
						result.Parameters[idx].Type = paramType
					}
				} else {
					indexByKey[paramName] = len(result.Parameters)
					result.Parameters = append(result.Parameters, scaf.ParameterInfo{
						Name:     paramName,
						Type:     paramType,
						Position: position,
						Line:     line,
						Column:   column,
						Length:   length,
						Count:    1,
					})
				}
			}
		}

		// Recursively walk children
		if ruleCtx, ok := node.(antlr.RuleContext); ok {
			for i := 0; i < ruleCtx.GetChildCount(); i++ {
				if child := ruleCtx.GetChild(i); child != nil {
					walk(child, parentPropName, parentLabels)
				}
			}
		}
	}

	walk(tree, "", nil)
}

// getParentNodeLabels finds the labels from a parent node pattern.
func getParentNodeLabels(node antlr.Tree, ctx *queryContext) []string {
	// Walk up the tree to find NodePatternContext
	current := node
	for current != nil {
		if nodeCtx, ok := current.(*cyphergrammar.NodePatternContext); ok {
			var labels []string
			if labelsCtx := nodeCtx.NodeLabels(); labelsCtx != nil {
				for _, nameCtx := range labelsCtx.AllName() {
					if nameCtx != nil {
						labels = append(labels, nameCtx.GetText())
					}
				}
			}
			return labels
		}

		// Try to get parent
		if ruleCtx, ok := current.(antlr.RuleContext); ok {
			current = ruleCtx.GetParent()
		} else {
			break
		}
	}

	return nil
}

// inferParameterType looks up the type of a property in the schema.
func inferParameterType(propName string, labels []string, ctx *queryContext) string {
	if ctx.schema == nil || propName == "" {
		return ""
	}

	// Try each label
	for _, label := range labels {
		if model, ok := ctx.schema.Models[label]; ok {
			for _, field := range model.Fields {
				if field.Name == propName && field.Type != nil {
					return field.Type.String()
				}
			}
		}
	}

	return ""
}

// extractReturns walks the tree to find RETURN clause items.
func extractReturns(tree antlr.ParseTree, result *scaf.QueryMetadata, ctx *queryContext) {
	var walk func(node antlr.Tree)

	walk = func(node antlr.Tree) {
		if returnCtx, ok := node.(*cyphergrammar.ReturnStContext); ok {
			extractReturnInfo(returnCtx, result, ctx)
		}

		// Recursively walk children
		if ruleCtx, ok := node.(antlr.RuleContext); ok {
			for i := 0; i < ruleCtx.GetChildCount(); i++ {
				if child := ruleCtx.GetChild(i); child != nil {
					walk(child)
				}
			}
		}
	}

	walk(tree)
}

// extractReturnInfo processes a ReturnStContext to extract return items.
func extractReturnInfo(returnCtx *cyphergrammar.ReturnStContext, result *scaf.QueryMetadata, ctx *queryContext) {
	projBody := returnCtx.ProjectionBody()
	if projBody == nil {
		return
	}

	projItems := projBody.ProjectionItems()
	if projItems == nil {
		return
	}

	// Check for RETURN * (wildcard)
	if projItems.MULT() != nil {
		result.Returns = append(result.Returns, scaf.ReturnInfo{
			Name:       "*",
			Expression: "*",
			IsWildcard: true,
		})

		return
	}

	// Iterate through each projection item
	for _, itemCtx := range projItems.AllProjectionItem() {
		if projItemCtx, ok := itemCtx.(*cyphergrammar.ProjectionItemContext); ok {
			extractProjectionItem(projItemCtx, result, ctx)
		}
	}
}

// extractProjectionItem processes a single ProjectionItemContext.
func extractProjectionItem(itemCtx *cyphergrammar.ProjectionItemContext, result *scaf.QueryMetadata, ctx *queryContext) {
	if itemCtx == nil {
		return
	}

	exprCtx := itemCtx.Expression()
	if exprCtx == nil {
		return
	}

	expression := exprCtx.GetText()

	// Get position info from the expression's start token
	var line, column, length int
	if ruleCtx, ok := exprCtx.(antlr.ParserRuleContext); ok {
		startToken := ruleCtx.GetStart()
		if startToken != nil {
			line = startToken.GetLine()
			column = startToken.GetColumn() + 1 // ANTLR is 0-based, we want 1-based
			length = len(expression)
		}
	}

	// Check for alias (AS keyword)
	var alias string

	if itemCtx.AS() != nil {
		if symbolCtx := itemCtx.Symbol(); symbolCtx != nil {
			alias = symbolCtx.GetText()
		}
	}

	// Determine the name to use for completion
	name := alias
	if name == "" {
		name = inferNameFromExpression(expression)
	}

	// Check for aggregate function
	isAggregate := isAggregateExpression(exprCtx)

	// Check for wildcard patterns like n.*
	isWildcard := expression == "*" || strings.HasSuffix(expression, ".*")

	// Infer type from schema
	returnType := inferReturnType(expression, ctx)

	result.Returns = append(result.Returns, scaf.ReturnInfo{
		Name:        name,
		Type:        returnType,
		Expression:  expression,
		Alias:       alias,
		IsAggregate: isAggregate,
		IsWildcard:  isWildcard,
		Line:        line,
		Column:      column,
		Length:      length,
	})
}

// inferReturnType infers the type of a return expression from the schema.
func inferReturnType(expression string, ctx *queryContext) string {
	if ctx.schema == nil {
		return ""
	}

	// Parse property access: "u.name" -> variable="u", property="name"
	if idx := strings.Index(expression, "."); idx > 0 {
		varName := expression[:idx]
		propName := expression[idx+1:]

		// Handle nested property access (e.g., "u.address.city") - just get first property for now
		if nestedIdx := strings.Index(propName, "."); nestedIdx > 0 {
			propName = propName[:nestedIdx]
		}

		// Look up the variable binding
		if binding, ok := ctx.bindings[varName]; ok {
			for _, label := range binding.labels {
				if model, ok := ctx.schema.Models[label]; ok {
					for _, field := range model.Fields {
						if field.Name == propName && field.Type != nil {
							return field.Type.String()
						}
					}
				}
			}
		}
	}

	// Check if it's a plain variable (returning whole node)
	if binding, ok := ctx.bindings[expression]; ok {
		// Return the model name as the type (e.g., "*User")
		if len(binding.labels) > 0 {
			return "*" + binding.labels[0]
		}
	}

	return ""
}

// inferNameFromExpression infers a usable name from an expression.
// For "u.name" returns "name", for "u" returns "u", for "count(*)" returns "count".
func inferNameFromExpression(expr string) string {
	// If it contains a dot (property access), use the last part
	if idx := strings.LastIndex(expr, "."); idx >= 0 && idx < len(expr)-1 {
		return expr[idx+1:]
	}

	// If it looks like a function call, extract the function name
	if idx := strings.Index(expr, "("); idx > 0 {
		return expr[:idx]
	}

	return expr
}

// isAggregateExpression checks if an expression is an aggregate function.
func isAggregateExpression(exprCtx cyphergrammar.IExpressionContext) bool {
	if exprCtx == nil {
		return false
	}

	var check func(antlr.Tree) bool

	check = func(node antlr.Tree) bool {
		// Check for CountAllContext (count(*))
		if _, ok := node.(*cyphergrammar.CountAllContext); ok {
			return true
		}

		// Check if this is a function call context
		if funcCtx, ok := node.(*cyphergrammar.FunctionInvocationContext); ok {
			funcName := strings.ToLower(funcCtx.GetText())

			aggregates := []string{"count", "sum", "avg", "min", "max", "collect", "percentile", "stddev"}
			for _, agg := range aggregates {
				if strings.HasPrefix(funcName, agg) {
					return true
				}
			}
		}

		// Recursively check children
		if ruleCtx, ok := node.(antlr.RuleContext); ok {
			for i := 0; i < ruleCtx.GetChildCount(); i++ {
				if child := ruleCtx.GetChild(i); child != nil {
					if check(child) {
						return true
					}
				}
			}
		}

		return false
	}

	return check(exprCtx)
}

// checkUniqueFilter walks the parse tree to find MATCH clauses with property filters
// and checks if any filter is on a unique field.
func checkUniqueFilter(tree antlr.ParseTree, schema *analysis.TypeSchema) bool {
	var found bool

	var walk func(node antlr.Tree)
	walk = func(node antlr.Tree) {
		// Look for node patterns with properties like (u:User {id: $userId})
		if nodeCtx, ok := node.(*cyphergrammar.NodePatternContext); ok {
			if checkNodePatternForUnique(nodeCtx, schema) {
				found = true
				return
			}
		}

		// Recursively walk children
		if ruleCtx, ok := node.(antlr.RuleContext); ok {
			for i := 0; i < ruleCtx.GetChildCount(); i++ {
				if child := ruleCtx.GetChild(i); child != nil {
					walk(child)
					if found {
						return
					}
				}
			}
		}
	}

	walk(tree)
	return found
}

// checkNodePatternForUnique checks if a node pattern filters on a unique field.
// Pattern like (u:User {id: $userId}) - checks if "id" is unique on "User".
func checkNodePatternForUnique(nodeCtx *cyphergrammar.NodePatternContext, schema *analysis.TypeSchema) bool {
	// Get the label(s) from the node pattern
	labels := nodeCtx.NodeLabels()
	if labels == nil {
		return false
	}

	// Get all label names - NodeLabels has AllName() returning []INameContext
	var labelNames []string
	for _, nameCtx := range labels.AllName() {
		if nameCtx != nil {
			labelNames = append(labelNames, nameCtx.GetText())
		}
	}

	if len(labelNames) == 0 {
		return false
	}

	// Get the properties from the node pattern
	props := nodeCtx.Properties()
	if props == nil {
		return false
	}

	mapLit := props.MapLit()
	if mapLit == nil {
		return false
	}

	// Get property names being filtered - MapLit has AllMapPair() returning []IMapPairContext
	var propNames []string
	for _, pairCtx := range mapLit.AllMapPair() {
		if pair, ok := pairCtx.(*cyphergrammar.MapPairContext); ok {
			if nameCtx := pair.Name(); nameCtx != nil {
				propNames = append(propNames, nameCtx.GetText())
			}
		}
	}

	// Check if any property is unique on any of the labels
	for _, label := range labelNames {
		model, ok := schema.Models[label]
		if !ok {
			continue
		}

		for _, propName := range propNames {
			for _, field := range model.Fields {
				if field.Name == propName && field.Unique {
					return true
				}
			}
		}
	}

	return false
}

// Ensure Analyzer implements scaf.QueryAnalyzer and analysis.SchemaAwareAnalyzer.
var _ scaf.QueryAnalyzer = (*Analyzer)(nil)
var _ analysis.SchemaAwareAnalyzer = (*Analyzer)(nil)
