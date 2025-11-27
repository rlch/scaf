package cypher

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/rlch/scaf"
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

// AnalyzeQuery parses a Cypher query and extracts metadata.
func (a *Analyzer) AnalyzeQuery(query string) (*scaf.QueryMetadata, error) {
	tree, err := parseCypherQuery(query)
	if err != nil {
		return nil, err
	}

	result := &scaf.QueryMetadata{
		Parameters: []scaf.ParameterInfo{},
		Returns:    []scaf.ReturnInfo{},
	}

	// Extract parameters
	extractParameters(tree, result)

	// Extract return items
	extractReturns(tree, result)

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

// extractParameters walks the tree to find all $parameters.
func extractParameters(tree antlr.ParseTree, result *scaf.QueryMetadata) {
	indexByKey := make(map[string]int) // map name -> index in Parameters slice

	var walk func(node antlr.Tree)

	walk = func(node antlr.Tree) {
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

				if idx, exists := indexByKey[paramName]; exists {
					result.Parameters[idx].Count++
				} else {
					indexByKey[paramName] = len(result.Parameters)
					result.Parameters = append(result.Parameters, scaf.ParameterInfo{
						Name:     paramName,
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
					walk(child)
				}
			}
		}
	}

	walk(tree)
}

// extractReturns walks the tree to find RETURN clause items.
func extractReturns(tree antlr.ParseTree, result *scaf.QueryMetadata) {
	var walk func(node antlr.Tree)

	walk = func(node antlr.Tree) {
		if returnCtx, ok := node.(*cyphergrammar.ReturnStContext); ok {
			extractReturnInfo(returnCtx, result)
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
func extractReturnInfo(returnCtx *cyphergrammar.ReturnStContext, result *scaf.QueryMetadata) {
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
			extractProjectionItem(projItemCtx, result)
		}
	}
}

// extractProjectionItem processes a single ProjectionItemContext.
func extractProjectionItem(itemCtx *cyphergrammar.ProjectionItemContext, result *scaf.QueryMetadata) {
	if itemCtx == nil {
		return
	}

	exprCtx := itemCtx.Expression()
	if exprCtx == nil {
		return
	}

	expression := exprCtx.GetText()

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

	result.Returns = append(result.Returns, scaf.ReturnInfo{
		Name:        name,
		Expression:  expression,
		Alias:       alias,
		IsAggregate: isAggregate,
		IsWildcard:  isWildcard,
	})
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

// Ensure Analyzer implements scaf.QueryAnalyzer.
var _ scaf.QueryAnalyzer = (*Analyzer)(nil)