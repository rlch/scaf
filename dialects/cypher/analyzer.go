package cypher

import (
	"fmt"
	"strings"

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
	optional bool     // true if from OPTIONAL MATCH (properties can be null)
}

// queryContext holds context during query analysis.
type queryContext struct {
	bindings map[string]*variableBinding // variable name -> binding (from MATCH)
	locals   map[string]*analysis.Type   // local variable types (from list comprehensions, UNWIND, etc.)
	schema   *analysis.TypeSchema
}

func newQueryContext(schema *analysis.TypeSchema) *queryContext {
	return &queryContext{
		bindings: make(map[string]*variableBinding),
		locals:   make(map[string]*analysis.Type),
		schema:   schema,
	}
}

// withLocal returns a new queryContext with an additional local variable binding.
// This is used for scoped bindings like list comprehension variables.
func (qctx *queryContext) withLocal(name string, typ *analysis.Type) *queryContext {
	newLocals := make(map[string]*analysis.Type, len(qctx.locals)+1)
	for k, v := range qctx.locals {
		newLocals[k] = v
	}
	newLocals[name] = typ
	return &queryContext{
		bindings: qctx.bindings,
		locals:   newLocals,
		schema:   qctx.schema,
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
	ast, err := cyphergrammar.Parse(query)
	if err != nil {
		// Return partial results even on parse errors - we still want completion
		// for partially valid queries
		return &scaf.QueryMetadata{
			Parameters: []scaf.ParameterInfo{},
			Returns:    []scaf.ReturnInfo{},
		}, nil
	}

	ctx := newQueryContext(schema)
	result := &scaf.QueryMetadata{
		Parameters: []scaf.ParameterInfo{},
		Returns:    []scaf.ReturnInfo{},
		Bindings:   make(map[string][]string),
	}

	// First pass: extract variable bindings from MATCH clauses
	extractBindings(ast, ctx)

	// Export bindings to result for hover/completion
	for varName, binding := range ctx.bindings {
		if len(binding.labels) > 0 {
			result.Bindings[varName] = binding.labels
		}
	}

	// Extract parameters with type inference
	extractParameters(ast, result, ctx)

	// Extract return items with type inference
	extractReturns(ast, result, ctx)

	// Check for unique field filters if schema is provided
	if schema != nil {
		result.ReturnsOne = checkUniqueFilter(ast, schema)
	}

	return result, nil
}

// extractBindings walks the AST to find variable bindings from node patterns.
// E.g., MATCH (u:User) binds variable "u" to label "User".
func extractBindings(ast *cyphergrammar.Script, ctx *queryContext) {
	if ast == nil || ast.Query == nil {
		return
	}

	// Handle regular query
	if rq := ast.Query.RegularQuery; rq != nil {
		extractBindingsFromRegularQuery(rq, ctx)
	}
}

func extractBindingsFromRegularQuery(rq *cyphergrammar.RegularQuery, ctx *queryContext) {
	if rq == nil || rq.SingleQuery == nil {
		return
	}

	for _, clause := range rq.SingleQuery.Clauses {
		// Extract from MATCH clauses
		if clause.Reading != nil && clause.Reading.Match != nil {
			optional := clause.Reading.Match.Optional
			extractBindingsFromPattern(clause.Reading.Match.Pattern, ctx, optional)
		}
		// Extract from CREATE clauses
		if clause.Updating != nil && clause.Updating.Create != nil && clause.Updating.Create.Pattern != nil {
			extractBindingsFromPattern(clause.Updating.Create.Pattern, ctx, false)
		}
		// Extract from MERGE clauses
		if clause.Updating != nil && clause.Updating.Merge != nil && clause.Updating.Merge.Pattern != nil {
			extractBindingsFromPatternElement(clause.Updating.Merge.Pattern.Element, ctx, false)
		}
	}

	// Also check union queries
	for _, union := range rq.Unions {
		if union.Query != nil {
			for _, clause := range union.Query.Clauses {
				if clause.Reading != nil && clause.Reading.Match != nil {
					optional := clause.Reading.Match.Optional
					extractBindingsFromPattern(clause.Reading.Match.Pattern, ctx, optional)
				}
				if clause.Updating != nil && clause.Updating.Create != nil && clause.Updating.Create.Pattern != nil {
					extractBindingsFromPattern(clause.Updating.Create.Pattern, ctx, false)
				}
				if clause.Updating != nil && clause.Updating.Merge != nil && clause.Updating.Merge.Pattern != nil {
					extractBindingsFromPatternElement(clause.Updating.Merge.Pattern.Element, ctx, false)
				}
			}
		}
	}
}

func extractBindingsFromPattern(pattern *cyphergrammar.Pattern, ctx *queryContext, optional bool) {
	if pattern == nil {
		return
	}

	for _, part := range pattern.Parts {
		if part.Element != nil {
			extractBindingsFromPatternElement(part.Element, ctx, optional)
		}
	}
}

func extractBindingsFromPatternElement(elem *cyphergrammar.PatternElement, ctx *queryContext, optional bool) {
	if elem == nil {
		return
	}

	// Handle parenthesized pattern
	if elem.Paren != nil {
		extractBindingsFromPatternElement(elem.Paren, ctx, optional)
		return
	}

	// Extract from node pattern
	if elem.Node != nil {
		extractNodeBinding(elem.Node, ctx, optional)
	}

	// Extract from chain
	for _, chain := range elem.Chain {
		if chain.Node != nil {
			extractNodeBinding(chain.Node, ctx, optional)
		}
	}
}

func extractNodeBinding(node *cyphergrammar.NodePattern, ctx *queryContext, optional bool) {
	if node == nil || node.Variable == "" {
		return
	}

	var labels []string
	if node.Labels != nil {
		labels = node.Labels.Labels
	}

	ctx.bindings[node.Variable] = &variableBinding{
		variable: node.Variable,
		labels:   labels,
		optional: optional,
	}
}

// walkAddSubExprSimple walks an AddSubExpr to find atoms containing parameters.
func walkAddSubExprSimple(add *cyphergrammar.AddSubExpr, propName string, labels []string, walkAtom func(*cyphergrammar.Atom, string, []string)) {
	if add == nil {
		return
	}

	var walkMult func(*cyphergrammar.MultDivExpr)
	walkMult = func(mult *cyphergrammar.MultDivExpr) {
		if mult == nil {
			return
		}
		var walkPow func(*cyphergrammar.PowerExpr)
		walkPow = func(pow *cyphergrammar.PowerExpr) {
			if pow == nil {
				return
			}
			var walkUnary func(*cyphergrammar.UnaryExpr)
			walkUnary = func(unary *cyphergrammar.UnaryExpr) {
				if unary == nil || unary.Expr == nil {
					return
				}
				walkAtom(unary.Expr.Atom, propName, labels)
				// Note: we don't recurse into suffixes here to avoid infinite loops
			}
			walkUnary(pow.Left)
			for _, t := range pow.Right {
				walkUnary(t.Expr)
			}
		}
		walkPow(mult.Left)
		for _, t := range mult.Right {
			walkPow(t.Expr)
		}
	}

	walkMult(add.Left)
	for _, t := range add.Right {
		walkMult(t.Expr)
	}
}

// extractParameters walks the AST to find all $parameters.
func extractParameters(ast *cyphergrammar.Script, result *scaf.QueryMetadata, ctx *queryContext) {
	if ast == nil {
		return
	}

	indexByKey := make(map[string]int)

	var walkExpr func(expr *cyphergrammar.Expression, propName string, labels []string)
	var walkAtom func(atom *cyphergrammar.Atom, propName string, labels []string)

	walkAtom = func(atom *cyphergrammar.Atom, propName string, labels []string) {
		if atom == nil {
			return
		}

		// Check for parameter
		if atom.Parameter != nil {
			param := atom.Parameter
			paramName := param.Name

			if paramName != "" {
				// Get position info
				line := param.Pos.Line
				column := param.Pos.Column
				position := param.Pos.Offset
				length := len(paramName) + 1 // +1 for $

				// Infer type from schema
				paramType := inferParameterType(propName, labels, ctx)

				if idx, exists := indexByKey[paramName]; exists {
					result.Parameters[idx].Count++
					if paramType != nil && result.Parameters[idx].Type == nil {
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

		// Walk nested expressions
		if atom.Literal != nil {
			if atom.Literal.List != nil {
				for _, item := range atom.Literal.List.Items {
					walkExpr(item, propName, labels)
				}
			}
			if atom.Literal.Map != nil {
				for _, pair := range atom.Literal.Map.Pairs {
					walkExpr(pair.Value, pair.Key, labels)
				}
			}
		}

		if atom.ListComprehension != nil {
			walkExpr(atom.ListComprehension.Source, propName, labels)
			if atom.ListComprehension.Mapping != nil {
				walkExpr(atom.ListComprehension.Mapping, propName, labels)
			}
		}

		if atom.FunctionCall != nil {
			for _, arg := range atom.FunctionCall.Args {
				walkExpr(arg, propName, labels)
			}
		}

		if atom.Parenthesized != nil {
			walkExpr(atom.Parenthesized, propName, labels)
		}

		if atom.CaseExpr != nil {
			if atom.CaseExpr.Input != nil {
				walkExpr(atom.CaseExpr.Input, propName, labels)
			}
			for _, when := range atom.CaseExpr.Whens {
				walkExpr(when.When, propName, labels)
				walkExpr(when.Then, propName, labels)
			}
			if atom.CaseExpr.Else != nil {
				walkExpr(atom.CaseExpr.Else, propName, labels)
			}
		}
	}

	walkExpr = func(expr *cyphergrammar.Expression, propName string, labels []string) {
		if expr == nil {
			return
		}

		// Walk through the expression tree
		walkXorExpr := func(xor *cyphergrammar.XorExpr) {
			if xor == nil {
				return
			}

			walkAndExpr := func(and *cyphergrammar.AndExpr) {
				if and == nil {
					return
				}

				walkNotExpr := func(not *cyphergrammar.NotExpr) {
					if not == nil || not.Expr == nil {
						return
					}

					walkCompExpr := func(comp *cyphergrammar.ComparisonExpr) {
						if comp == nil {
							return
						}

						// Helper to walk an AddSubExpr with given type context
						walkAddSubExprWithCtx := func(add *cyphergrammar.AddSubExpr, ctxProp string, ctxLabels []string) {
							if add == nil {
								return
							}

							walkMultDivExpr := func(mult *cyphergrammar.MultDivExpr) {
								if mult == nil {
									return
								}

								walkPowerExpr := func(pow *cyphergrammar.PowerExpr) {
									if pow == nil {
										return
									}

									walkUnaryExpr := func(unary *cyphergrammar.UnaryExpr) {
										if unary == nil || unary.Expr == nil {
											return
										}

										walkAtom(unary.Expr.Atom, ctxProp, ctxLabels)

										// Walk suffixes
										for _, suffix := range unary.Expr.Suffixes {
											if suffix.Index != nil {
												walkExpr(suffix.Index.Start, ctxProp, ctxLabels)
												walkExpr(suffix.Index.End, ctxProp, ctxLabels)
											}
											if suffix.In != nil && suffix.In.Expr != nil {
												walkAddSubExprSimple(suffix.In.Expr, ctxProp, ctxLabels, walkAtom)
											}
											if suffix.StringPred != nil {
												if suffix.StringPred.StartsWith != nil {
													walkAddSubExprSimple(suffix.StringPred.StartsWith, ctxProp, ctxLabels, walkAtom)
												}
												if suffix.StringPred.EndsWith != nil {
													walkAddSubExprSimple(suffix.StringPred.EndsWith, ctxProp, ctxLabels, walkAtom)
												}
												if suffix.StringPred.Contains != nil {
													walkAddSubExprSimple(suffix.StringPred.Contains, ctxProp, ctxLabels, walkAtom)
												}
											}
										}
									}

									walkUnaryExpr(pow.Left)
									for _, term := range pow.Right {
										walkUnaryExpr(term.Expr)
									}
								}

								walkPowerExpr(mult.Left)
								for _, term := range mult.Right {
									walkPowerExpr(term.Expr)
								}
							}

							walkMultDivExpr(add.Left)
							for _, term := range add.Right {
								walkMultDivExpr(term.Expr)
							}
						}

						// For equality comparisons (p.name = $param), infer type from property access
						if len(comp.Right) == 1 && comp.Right[0].Op == "=" {
							leftInfo := extractPropertyAccess(comp.Left, ctx)
							rightInfo := extractPropertyAccess(comp.Right[0].Expr, ctx)

							if leftInfo != nil && leftInfo.propName != "" {
								walkAddSubExprWithCtx(comp.Right[0].Expr, leftInfo.propName, leftInfo.labels)
								walkAddSubExprWithCtx(comp.Left, propName, labels)
								return
							}
							if rightInfo != nil && rightInfo.propName != "" {
								walkAddSubExprWithCtx(comp.Left, rightInfo.propName, rightInfo.labels)
								walkAddSubExprWithCtx(comp.Right[0].Expr, propName, labels)
								return
							}
						}

						// Default: walk with passed-in context
						walkAddSubExprWithCtx(comp.Left, propName, labels)
						for _, term := range comp.Right {
							walkAddSubExprWithCtx(term.Expr, propName, labels)
						}
					}

					walkCompExpr(not.Expr)
				}

				walkNotExpr(and.Left)
				for _, term := range and.Right {
					walkNotExpr(term.Expr)
				}
			}

			walkAndExpr(xor.Left)
			for _, term := range xor.Right {
				walkAndExpr(term.Expr)
			}
		}

		walkXorExpr(expr.Left)
		for _, term := range expr.Right {
			walkXorExpr(term.Expr)
		}
	}

	// Walk map literals in node patterns to get property context
	var walkNodePattern func(node *cyphergrammar.NodePattern)
	walkNodePattern = func(node *cyphergrammar.NodePattern) {
		if node == nil || node.Properties == nil {
			return
		}

		var labels []string
		if node.Labels != nil {
			labels = node.Labels.Labels
		}

		if node.Properties.Map != nil {
			for _, pair := range node.Properties.Map.Pairs {
				walkExpr(pair.Value, pair.Key, labels)
			}
		}
		if node.Properties.Param != nil {
			// Parameter as properties
			param := node.Properties.Param
			paramName := param.Name
			if paramName != "" {
				if idx, exists := indexByKey[paramName]; exists {
					result.Parameters[idx].Count++
				} else {
					indexByKey[paramName] = len(result.Parameters)
					result.Parameters = append(result.Parameters, scaf.ParameterInfo{
						Name:     paramName,
						Position: param.Pos.Offset,
						Line:     param.Pos.Line,
						Column:   param.Pos.Column,
						Length:   len(paramName) + 1,
						Count:    1,
					})
				}
			}
		}
	}

	// Walk the entire AST
	if ast.Query != nil {
		if rq := ast.Query.RegularQuery; rq != nil && rq.SingleQuery != nil {
			for _, clause := range rq.SingleQuery.Clauses {
				if clause.Reading != nil {
					if clause.Reading.Match != nil && clause.Reading.Match.Pattern != nil {
						for _, part := range clause.Reading.Match.Pattern.Parts {
							if part.Element != nil {
								walkPatternElement(part.Element, walkNodePattern)
							}
						}
						if clause.Reading.Match.Where != nil {
							walkExpr(clause.Reading.Match.Where.Expr, "", nil)
						}
					}
					if clause.Reading.Unwind != nil {
						walkExpr(clause.Reading.Unwind.Expr, "", nil)
					}
				}
				if clause.Updating != nil {
					if clause.Updating.Create != nil && clause.Updating.Create.Pattern != nil {
						for _, part := range clause.Updating.Create.Pattern.Parts {
							if part.Element != nil {
								walkPatternElement(part.Element, walkNodePattern)
							}
						}
					}
					if clause.Updating.Merge != nil && clause.Updating.Merge.Pattern != nil {
						if clause.Updating.Merge.Pattern.Element != nil {
							walkPatternElement(clause.Updating.Merge.Pattern.Element, walkNodePattern)
						}
						for _, action := range clause.Updating.Merge.Actions {
							if action.Set != nil {
								for _, item := range action.Set.Items {
									walkSetItem(item, walkExpr)
								}
							}
						}
					}
					if clause.Updating.Delete != nil {
						for _, expr := range clause.Updating.Delete.Exprs {
							walkExpr(expr, "", nil)
						}
					}
					if clause.Updating.Set != nil {
						for _, item := range clause.Updating.Set.Items {
							walkSetItem(item, walkExpr)
						}
					}
					// REMOVE clause doesn't typically contain parameters
				}
				if clause.Return != nil && clause.Return.Body != nil {
					if clause.Return.Body.Items != nil {
						for _, item := range clause.Return.Body.Items.Items {
							walkExpr(item.Expr, "", nil)
						}
					}
				}
				if clause.With != nil && clause.With.Body != nil {
					if clause.With.Body.Items != nil {
						for _, item := range clause.With.Body.Items.Items {
							walkExpr(item.Expr, "", nil)
						}
					}
					if clause.With.Where != nil {
						walkExpr(clause.With.Where.Expr, "", nil)
					}
				}
			}
		}
	}
}

func walkPatternElement(elem *cyphergrammar.PatternElement, walkNode func(*cyphergrammar.NodePattern)) {
	if elem == nil {
		return
	}
	if elem.Paren != nil {
		walkPatternElement(elem.Paren, walkNode)
		return
	}
	if elem.Node != nil {
		walkNode(elem.Node)
	}
	for _, chain := range elem.Chain {
		if chain.Node != nil {
			walkNode(chain.Node)
		}
	}
}

// walkSetItem walks a SET item to find expressions that may contain parameters.
func walkSetItem(item *cyphergrammar.SetItem, walkExpr func(*cyphergrammar.Expression, string, []string)) {
	if item == nil {
		return
	}
	if item.PropertyExpr != nil {
		walkExpr(item.PropertyExpr, "", nil)
	}
	if item.VarExpr != nil {
		walkExpr(item.VarExpr, "", nil)
	}
}

// inferParameterType looks up the type of a property in the schema.
func inferParameterType(propName string, labels []string, ctx *queryContext) *analysis.Type {
	if ctx.schema == nil || propName == "" {
		return nil
	}

	// Try each label
	for _, label := range labels {
		if model, ok := ctx.schema.Models[label]; ok {
			for _, field := range model.Fields {
				if field.Name == propName && field.Type != nil {
					return field.Type
				}
			}
		}
	}

	return nil
}

// propertyAccessInfo holds information about a property access expression like "p.name"
type propertyAccessInfo struct {
	varName  string   // The variable name (e.g., "p")
	propName string   // The property name (e.g., "name")
	labels   []string // The labels bound to the variable (e.g., ["Person"])
}

// extractPropertyAccess analyzes an AddSubExpr to detect property access patterns like "p.name".
// Returns info if the expression is a simple property access on a bound variable, nil otherwise.
func extractPropertyAccess(add *cyphergrammar.AddSubExpr, ctx *queryContext) *propertyAccessInfo {
	if add == nil || add.Left == nil {
		return nil
	}

	// Must be a simple expression (no + or - operations)
	if len(add.Right) > 0 {
		return nil
	}

	mult := add.Left
	if mult == nil || mult.Left == nil || len(mult.Right) > 0 {
		return nil
	}

	pow := mult.Left
	if pow == nil || pow.Left == nil || len(pow.Right) > 0 {
		return nil
	}

	unary := pow.Left
	if unary == nil || unary.Expr == nil {
		return nil
	}

	post := unary.Expr
	if post.Atom == nil || post.Atom.Variable == "" {
		return nil
	}

	varName := post.Atom.Variable

	// Need exactly one suffix that is a property access
	if len(post.Suffixes) != 1 {
		return nil
	}

	suffix := post.Suffixes[0]
	if suffix.Property == "" {
		return nil
	}

	propName := suffix.Property

	// Look up the variable's labels from bindings
	var labels []string
	if binding, ok := ctx.bindings[varName]; ok {
		labels = binding.labels
	}

	return &propertyAccessInfo{
		varName:  varName,
		propName: propName,
		labels:   labels,
	}
}

// extractReturns walks the AST to find RETURN clause items.
func extractReturns(ast *cyphergrammar.Script, result *scaf.QueryMetadata, ctx *queryContext) {
	if ast == nil || ast.Query == nil {
		return
	}

	if rq := ast.Query.RegularQuery; rq != nil && rq.SingleQuery != nil {
		for _, clause := range rq.SingleQuery.Clauses {
			if clause.Return != nil {
				extractReturnInfo(clause.Return, result, ctx)
			}
		}
	}
}

func extractReturnInfo(ret *cyphergrammar.ReturnClause, result *scaf.QueryMetadata, ctx *queryContext) {
	if ret == nil || ret.Body == nil || ret.Body.Items == nil {
		return
	}

	items := ret.Body.Items

	// Check for RETURN *
	if items.Star {
		result.Returns = append(result.Returns, scaf.ReturnInfo{
			Name:       "*",
			Expression: "*",
			IsWildcard: true,
		})
		return
	}

	// Process each projection item
	for _, item := range items.Items {
		extractProjectionItem(item, result, ctx)
	}
}

func extractProjectionItem(item *cyphergrammar.ProjectionItem, result *scaf.QueryMetadata, ctx *queryContext) {
	if item == nil || item.Expr == nil {
		return
	}

	expression := expressionToString(item.Expr)

	// Get position info
	line := item.Pos.Line
	column := item.Pos.Column
	length := len(expression)

	alias := item.Alias

	// Determine the name
	name := alias
	if name == "" {
		name = inferNameFromExpression(expression)
	}

	// Check for aggregate
	isAggregate := isAggregateExpression(item.Expr)

	// Check for wildcard
	isWildcard := expression == "*" || strings.HasSuffix(expression, ".*")

	// Infer type
	returnType := inferExpressionType(item.Expr, ctx)

	// Get Required from schema for simple property access (e.g., "u.name")
	// Conservative default: assume nullable for complex expressions we can't analyze.
	// Only use schema's Required for simple "variable.property" patterns where we can
	// confidently determine nullability. This avoids generating non-pointer types for
	// expressions that might return null (function calls, CASE, aggregates, etc.)
	required := false
	if field, binding := lookupFieldFromExpression(expression, ctx); field != nil {
		// If binding is from OPTIONAL MATCH, always treat as nullable
		if binding != nil && binding.optional {
			required = false
		} else {
			required = field.Required
		}
	}

	result.Returns = append(result.Returns, scaf.ReturnInfo{
		Name:        name,
		Type:        returnType,
		Expression:  expression,
		Alias:       alias,
		IsAggregate: isAggregate,
		IsWildcard:  isWildcard,
		Required:    required,
		Line:        line,
		Column:      column,
		Length:      length,
	})
}

// lookupFieldFromExpression extracts variable.property from expression and looks up the field.
// Returns (nil, nil) if expression is not a simple property access or field not found.
// Returns the binding as well so caller can check if it's from OPTIONAL MATCH.
func lookupFieldFromExpression(expression string, ctx *queryContext) (*analysis.Field, *variableBinding) {
	if ctx.schema == nil {
		return nil, nil
	}

	// Parse "variable.property" pattern
	parts := strings.SplitN(expression, ".", 2)
	if len(parts) != 2 {
		return nil, nil
	}

	varName := parts[0]
	propName := parts[1]

	// Check for additional operations (not a simple property access)
	// e.g., "u.name IS NULL", "u.name + 'x'", "u.tags[0]"
	if strings.ContainsAny(propName, " []()+<>=!") {
		return nil, nil
	}

	// Look up the binding to get the model
	binding, ok := ctx.bindings[varName]
	if !ok || len(binding.labels) == 0 {
		return nil, nil
	}

	modelName := binding.labels[0]
	model, ok := ctx.schema.Models[modelName]
	if !ok {
		return nil, binding
	}

	// Find the field
	for _, field := range model.Fields {
		if field.Name == propName {
			return field, binding
		}
	}

	return nil, binding
}

// expressionToString converts an Expression AST back to a string representation.
func expressionToString(expr *cyphergrammar.Expression) string {
	if expr == nil {
		return ""
	}

	var sb strings.Builder
	xorToString(&sb, expr.Left)
	for _, term := range expr.Right {
		sb.WriteString(" OR ")
		xorToString(&sb, term.Expr)
	}
	return sb.String()
}

func xorToString(sb *strings.Builder, xor *cyphergrammar.XorExpr) {
	if xor == nil {
		return
	}
	andToString(sb, xor.Left)
	for _, term := range xor.Right {
		sb.WriteString(" XOR ")
		andToString(sb, term.Expr)
	}
}

func andToString(sb *strings.Builder, and *cyphergrammar.AndExpr) {
	if and == nil {
		return
	}
	notToString(sb, and.Left)
	for _, term := range and.Right {
		sb.WriteString(" AND ")
		notToString(sb, term.Expr)
	}
}

func notToString(sb *strings.Builder, not *cyphergrammar.NotExpr) {
	if not == nil {
		return
	}
	if not.Not {
		sb.WriteString("NOT ")
	}
	compToString(sb, not.Expr)
}

func compToString(sb *strings.Builder, comp *cyphergrammar.ComparisonExpr) {
	if comp == nil {
		return
	}
	addSubToString(sb, comp.Left)
	for _, term := range comp.Right {
		sb.WriteString(" ")
		sb.WriteString(term.Op)
		sb.WriteString(" ")
		addSubToString(sb, term.Expr)
	}
}

func addSubToString(sb *strings.Builder, add *cyphergrammar.AddSubExpr) {
	if add == nil {
		return
	}
	multDivToString(sb, add.Left)
	for _, term := range add.Right {
		sb.WriteString(" ")
		sb.WriteString(term.Op)
		sb.WriteString(" ")
		multDivToString(sb, term.Expr)
	}
}

func multDivToString(sb *strings.Builder, mult *cyphergrammar.MultDivExpr) {
	if mult == nil {
		return
	}
	powerToString(sb, mult.Left)
	for _, term := range mult.Right {
		sb.WriteString(" ")
		sb.WriteString(term.Op)
		sb.WriteString(" ")
		powerToString(sb, term.Expr)
	}
}

func powerToString(sb *strings.Builder, pow *cyphergrammar.PowerExpr) {
	if pow == nil {
		return
	}
	unaryToString(sb, pow.Left)
	for _, term := range pow.Right {
		sb.WriteString(" ^ ")
		unaryToString(sb, term.Expr)
	}
}

func unaryToString(sb *strings.Builder, unary *cyphergrammar.UnaryExpr) {
	if unary == nil {
		return
	}
	if unary.Op != "" {
		sb.WriteString(unary.Op)
	}
	postfixToString(sb, unary.Expr)
}

func postfixToString(sb *strings.Builder, post *cyphergrammar.PostfixExpr) {
	if post == nil {
		return
	}
	atomToString(sb, post.Atom)
	for _, suffix := range post.Suffixes {
		if suffix.Property != "" {
			sb.WriteString(".")
			sb.WriteString(suffix.Property)
		}
		if suffix.Index != nil {
			sb.WriteString("[")
			if suffix.Index.Start != nil {
				sb.WriteString(expressionToString(suffix.Index.Start))
			}
			if suffix.Index.Range {
				sb.WriteString("..")
			}
			if suffix.Index.End != nil {
				sb.WriteString(expressionToString(suffix.Index.End))
			}
			sb.WriteString("]")
		}
		if suffix.IsNull != nil {
			sb.WriteString(" IS ")
			if suffix.IsNull.Not {
				sb.WriteString("NOT ")
			}
			sb.WriteString("NULL")
		}
		if suffix.In != nil {
			sb.WriteString(" IN ")
			addSubToString(sb, suffix.In.Expr)
		}
		if suffix.StringPred != nil {
			if suffix.StringPred.StartsWith != nil {
				sb.WriteString(" STARTS WITH ")
				addSubToString(sb, suffix.StringPred.StartsWith)
			}
			if suffix.StringPred.EndsWith != nil {
				sb.WriteString(" ENDS WITH ")
				addSubToString(sb, suffix.StringPred.EndsWith)
			}
			if suffix.StringPred.Contains != nil {
				sb.WriteString(" CONTAINS ")
				addSubToString(sb, suffix.StringPred.Contains)
			}
		}
	}
}

func atomToString(sb *strings.Builder, atom *cyphergrammar.Atom) {
	if atom == nil {
		return
	}

	if atom.Literal != nil {
		literalToString(sb, atom.Literal)
		return
	}
	if atom.Parameter != nil {
		sb.WriteString("$")
		sb.WriteString(atom.Parameter.Name)
		return
	}
	if atom.CountAll {
		sb.WriteString("count(*)")
		return
	}
	if atom.ListComprehension != nil {
		sb.WriteString("[")
		sb.WriteString(atom.ListComprehension.Variable)
		sb.WriteString(" IN ")
		sb.WriteString(expressionToString(atom.ListComprehension.Source))
		if atom.ListComprehension.Where != nil {
			sb.WriteString(" WHERE ")
			sb.WriteString(expressionToString(atom.ListComprehension.Where.Expr))
		}
		if atom.ListComprehension.Mapping != nil {
			sb.WriteString(" | ")
			sb.WriteString(expressionToString(atom.ListComprehension.Mapping))
		}
		sb.WriteString("]")
		return
	}
	if atom.FunctionCall != nil {
		sb.WriteString(atom.FunctionCall.Name.String())
		sb.WriteString("(")
		if atom.FunctionCall.Distinct {
			sb.WriteString("DISTINCT ")
		}
		for i, arg := range atom.FunctionCall.Args {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(expressionToString(arg))
		}
		sb.WriteString(")")
		return
	}
	if atom.Parenthesized != nil {
		sb.WriteString("(")
		sb.WriteString(expressionToString(atom.Parenthesized))
		sb.WriteString(")")
		return
	}
	if atom.CaseExpr != nil {
		sb.WriteString("CASE")
		if atom.CaseExpr.Input != nil {
			sb.WriteString(" ")
			sb.WriteString(expressionToString(atom.CaseExpr.Input))
		}
		for _, when := range atom.CaseExpr.Whens {
			sb.WriteString(" WHEN ")
			sb.WriteString(expressionToString(when.When))
			sb.WriteString(" THEN ")
			sb.WriteString(expressionToString(when.Then))
		}
		if atom.CaseExpr.Else != nil {
			sb.WriteString(" ELSE ")
			sb.WriteString(expressionToString(atom.CaseExpr.Else))
		}
		sb.WriteString(" END")
		return
	}
	if atom.Variable != "" {
		sb.WriteString(atom.Variable)
		return
	}
}

func literalToString(sb *strings.Builder, lit *cyphergrammar.Literal) {
	if lit == nil {
		return
	}

	if lit.Null {
		sb.WriteString("null")
		return
	}
	if lit.True {
		sb.WriteString("true")
		return
	}
	if lit.False {
		sb.WriteString("false")
		return
	}
	if lit.Int != nil {
		sb.WriteString(fmt.Sprintf("%d", *lit.Int))
		return
	}
	if lit.Float != nil {
		sb.WriteString(fmt.Sprintf("%g", *lit.Float))
		return
	}
	if lit.String != nil {
		sb.WriteString(*lit.String)
		return
	}
	if lit.List != nil {
		sb.WriteString("[")
		for i, item := range lit.List.Items {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(expressionToString(item))
		}
		sb.WriteString("]")
		return
	}
	if lit.Map != nil {
		sb.WriteString("{")
		for i, pair := range lit.Map.Pairs {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(pair.Key)
			sb.WriteString(": ")
			sb.WriteString(expressionToString(pair.Value))
		}
		sb.WriteString("}")
		return
	}
}

// inferNameFromExpression infers a usable name from an expression.
func inferNameFromExpression(expr string) string {
	if idx := strings.LastIndex(expr, "."); idx >= 0 && idx < len(expr)-1 {
		return expr[idx+1:]
	}
	if idx := strings.Index(expr, "("); idx > 0 {
		return expr[:idx]
	}
	return expr
}

// isAggregateExpression checks if an expression contains an aggregate function.
func isAggregateExpression(expr *cyphergrammar.Expression) bool {
	if expr == nil {
		return false
	}

	var check func(*cyphergrammar.Atom) bool
	check = func(atom *cyphergrammar.Atom) bool {
		if atom == nil {
			return false
		}
		if atom.CountAll {
			return true
		}
		if atom.FunctionCall != nil {
			name := strings.ToLower(atom.FunctionCall.Name.String())
			aggregates := []string{"count", "sum", "avg", "min", "max", "collect", "percentile", "stddev"}
			for _, agg := range aggregates {
				if strings.HasPrefix(name, agg) {
					return true
				}
			}
			// Check arguments recursively
			for _, arg := range atom.FunctionCall.Args {
				if isAggregateExpression(arg) {
					return true
				}
			}
		}
		if atom.Parenthesized != nil {
			return isAggregateExpression(atom.Parenthesized)
		}
		if atom.ListComprehension != nil {
			if isAggregateExpression(atom.ListComprehension.Source) {
				return true
			}
			if atom.ListComprehension.Mapping != nil && isAggregateExpression(atom.ListComprehension.Mapping) {
				return true
			}
		}
		return false
	}

	// Walk the expression tree to find atoms
	var walkExpr func(*cyphergrammar.Expression) bool
	walkExpr = func(e *cyphergrammar.Expression) bool {
		if e == nil {
			return false
		}

		var walkXor func(*cyphergrammar.XorExpr) bool
		walkXor = func(x *cyphergrammar.XorExpr) bool {
			if x == nil {
				return false
			}

			var walkAnd func(*cyphergrammar.AndExpr) bool
			walkAnd = func(a *cyphergrammar.AndExpr) bool {
				if a == nil {
					return false
				}

				var walkNot func(*cyphergrammar.NotExpr) bool
				walkNot = func(n *cyphergrammar.NotExpr) bool {
					if n == nil || n.Expr == nil {
						return false
					}

					var walkComp func(*cyphergrammar.ComparisonExpr) bool
					walkComp = func(c *cyphergrammar.ComparisonExpr) bool {
						if c == nil {
							return false
						}

						var walkAdd func(*cyphergrammar.AddSubExpr) bool
						walkAdd = func(as *cyphergrammar.AddSubExpr) bool {
							if as == nil {
								return false
							}

							var walkMult func(*cyphergrammar.MultDivExpr) bool
							walkMult = func(m *cyphergrammar.MultDivExpr) bool {
								if m == nil {
									return false
								}

								var walkPow func(*cyphergrammar.PowerExpr) bool
								walkPow = func(p *cyphergrammar.PowerExpr) bool {
									if p == nil {
										return false
									}

									var walkUnary func(*cyphergrammar.UnaryExpr) bool
									walkUnary = func(u *cyphergrammar.UnaryExpr) bool {
										if u == nil || u.Expr == nil {
											return false
										}
										return check(u.Expr.Atom)
									}

									if walkUnary(p.Left) {
										return true
									}
									for _, t := range p.Right {
										if walkUnary(t.Expr) {
											return true
										}
									}
									return false
								}

								if walkPow(m.Left) {
									return true
								}
								for _, t := range m.Right {
									if walkPow(t.Expr) {
										return true
									}
								}
								return false
							}

							if walkMult(as.Left) {
								return true
							}
							for _, t := range as.Right {
								if walkMult(t.Expr) {
									return true
								}
							}
							return false
						}

						if walkAdd(c.Left) {
							return true
						}
						for _, t := range c.Right {
							if walkAdd(t.Expr) {
								return true
							}
						}
						return false
					}

					return walkComp(n.Expr)
				}

				if walkNot(a.Left) {
					return true
				}
				for _, t := range a.Right {
					if walkNot(t.Expr) {
						return true
					}
				}
				return false
			}

			if walkAnd(x.Left) {
				return true
			}
			for _, t := range x.Right {
				if walkAnd(t.Expr) {
					return true
				}
			}
			return false
		}

		if walkXor(e.Left) {
			return true
		}
		for _, t := range e.Right {
			if walkXor(t.Expr) {
				return true
			}
		}
		return false
	}

	return walkExpr(expr)
}

// checkUniqueFilter checks if the query filters on a unique field.
func checkUniqueFilter(ast *cyphergrammar.Script, schema *analysis.TypeSchema) bool {
	if ast == nil || ast.Query == nil || schema == nil {
		return false
	}

	if rq := ast.Query.RegularQuery; rq != nil && rq.SingleQuery != nil {
		for _, clause := range rq.SingleQuery.Clauses {
			if clause.Reading != nil && clause.Reading.Match != nil {
				if checkPatternForUnique(clause.Reading.Match.Pattern, schema) {
					return true
				}
			}
		}
	}
	return false
}

func checkPatternForUnique(pattern *cyphergrammar.Pattern, schema *analysis.TypeSchema) bool {
	if pattern == nil {
		return false
	}

	for _, part := range pattern.Parts {
		if part.Element != nil && checkPatternElementForUnique(part.Element, schema) {
			return true
		}
	}
	return false
}

func checkPatternElementForUnique(elem *cyphergrammar.PatternElement, schema *analysis.TypeSchema) bool {
	if elem == nil {
		return false
	}

	if elem.Paren != nil {
		return checkPatternElementForUnique(elem.Paren, schema)
	}

	if elem.Node != nil && checkNodePatternForUnique(elem.Node, schema) {
		return true
	}

	for _, chain := range elem.Chain {
		if chain.Node != nil && checkNodePatternForUnique(chain.Node, schema) {
			return true
		}
	}

	return false
}

func checkNodePatternForUnique(node *cyphergrammar.NodePattern, schema *analysis.TypeSchema) bool {
	if node == nil || node.Labels == nil || node.Properties == nil {
		return false
	}

	labels := node.Labels.Labels
	if len(labels) == 0 {
		return false
	}

	var propNames []string
	if node.Properties.Map != nil {
		for _, pair := range node.Properties.Map.Pairs {
			propNames = append(propNames, pair.Key)
		}
	}

	// Check if any property is unique on any label
	for _, label := range labels {
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
var (
	_ scaf.QueryAnalyzer           = (*Analyzer)(nil)
	_ analysis.SchemaAwareAnalyzer = (*Analyzer)(nil)
)
