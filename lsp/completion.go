package lsp

import (
	"context"
	"strings"
	"unicode"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// Completion handles textDocument/completion requests.
// Following gopls pattern: single path, token-based dispatch.
func (s *Server) Completion(_ context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	s.logger.Debug("Completion",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok {
		return nil, nil //nolint:nilnil
	}

	// Get trigger character from LSP context
	var triggerChar string
	if params.Context != nil && params.Context.TriggerCharacter != "" {
		triggerChar = params.Context.TriggerCharacter
	}

	// Build completion context and determine what completions to offer
	cc := s.buildCompletionContext(doc, params.Position, triggerChar)
	s.logger.Debug("Completion context",
		zap.String("kind", string(cc.Kind)),
		zap.String("prefix", cc.Prefix),
		zap.String("moduleAlias", cc.ModuleAlias),
		zap.String("inScope", cc.InScope),
		zap.Bool("inTest", cc.InTest))

	// Dispatch to appropriate completion handler
	var items []protocol.CompletionItem
	switch cc.Kind {
	case CompletionKindNone:
		// No completions available
	case CompletionKindQueryName:
		items = s.completeQueryNames(doc, cc)
	case CompletionKindKeyword:
		items = s.completeKeywords(cc)
	case CompletionKindParameter:
		items = s.completeParameters(doc, cc)
	case CompletionKindReturnField:
		items = s.completeReturnFields(doc, cc)
	case CompletionKindImportAlias:
		items = s.completeImportAliases(doc, cc)
	case CompletionKindSetupFunction:
		items = s.completeSetupFunctions(doc, cc)
	}

	// Filter by prefix
	// Skip filtering for return fields when prefix contains a dot (e.g., "u.")
	// because completeReturnFields already handles prefix matching internally
	if cc.Prefix != "" && !(cc.Kind == CompletionKindReturnField && strings.Contains(cc.Prefix, ".")) {
		items = filterByPrefix(items, cc.Prefix)
	}

	return &protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil
}

// CompletionKind indicates what kind of completion is expected at a position.
type CompletionKind string

const (
	CompletionKindNone          CompletionKind = "none"
	CompletionKindQueryName     CompletionKind = "query_name"
	CompletionKindKeyword       CompletionKind = "keyword"
	CompletionKindParameter     CompletionKind = "parameter"
	CompletionKindReturnField   CompletionKind = "return_field"
	CompletionKindImportAlias   CompletionKind = "import_alias"
	CompletionKindSetupFunction CompletionKind = "setup_function"
)

// CompletionContext holds information about where completion was triggered.
type CompletionContext struct {
	Kind        CompletionKind
	Prefix      string // Text being typed (for filtering)
	InScope     string // Name of enclosing QueryScope
	InTest      bool   // Inside a test body
	InSetup     bool   // Inside a setup clause
	InAssert    bool   // Inside an assert block
	ModuleAlias string // Import alias for module.function completion
	TriggerChar string // The trigger character (., $)
}

// buildCompletionContext analyzes the document and returns completion context.
// Single-path approach: inspect tokens before cursor, dispatch based on what we find.
func (s *Server) buildCompletionContext(doc *Document, pos protocol.Position, triggerChar string) *CompletionContext {
	cc := &CompletionContext{
		Kind:        CompletionKindNone,
		TriggerChar: triggerChar,
	}

	// Get document content and line text
	content := doc.Content
	lines := strings.Split(content, "\n")
	if int(pos.Line) >= len(lines) {
		return cc
	}
	lineText := lines[pos.Line]
	col := min(int(pos.Character), len(lineText))
	textBeforeCursor := lineText[:col]

	// Extract prefix (identifier being typed)
	cc.Prefix = extractPrefix(textBeforeCursor)

	// Detect trigger character from text if not provided by LSP
	if triggerChar == "" && col > 0 {
		lastChar := lineText[col-1]
		if lastChar == '$' || lastChar == '.' {
			cc.TriggerChar = string(lastChar)
		}
	}

	// Get analysis - prefer current, fall back to last valid for symbol lookup
	af := doc.Analysis
	if af == nil {
		return cc
	}

	// For symbol lookups, use last valid analysis if current has parse errors
	symbolsAnalysis := af
	if af.ParseError != nil && doc.LastValidAnalysis != nil {
		symbolsAnalysis = doc.LastValidAnalysis
	}

	// Convert to lexer position (1-indexed)
	lexPos := analysis.PositionToLexer(pos.Line, pos.Character)

	// Determine positional context (InScope, InTest, InSetup, InAssert)
	s.determinePositionalContext(cc, symbolsAnalysis, lexPos)

	// === SINGLE DISPATCH: Look at token before cursor ===
	// This is the gopls approach: one decision tree based on prev token
	cc.Kind = s.determineCompletionKind(cc, doc, symbolsAnalysis, lexPos, textBeforeCursor)

	return cc
}

// determinePositionalContext sets InScope, InTest, InSetup, InAssert based on AST position.
func (s *Server) determinePositionalContext(cc *CompletionContext, af *analysis.AnalyzedFile, pos lexer.Position) {
	if af == nil || af.Suite == nil {
		return
	}

	for _, scope := range af.Suite.Scopes {
		if !containsLexerPosition(scope.Span(), pos) {
			continue
		}
		cc.InScope = scope.QueryName

		// Check scope-level setup
		if scope.Setup != nil && containsLexerPosition(scope.Setup.Span(), pos) {
			cc.InSetup = true
		}

		// Check items (tests and groups)
		for _, item := range scope.Items {
			if item.Test != nil && containsLexerPosition(item.Test.Span(), pos) {
				cc.InTest = true
				if item.Test.Setup != nil && containsLexerPosition(item.Test.Setup.Span(), pos) {
					cc.InSetup = true
				}
				for _, assert := range item.Test.Asserts {
					if containsLexerPosition(assert.Span(), pos) {
						cc.InAssert = true
					}
				}
			}
			if item.Group != nil && containsLexerPosition(item.Group.Span(), pos) {
				s.checkGroupContext(cc, item.Group, pos)
			}
		}
	}
}

// checkGroupContext recursively checks context within a group.
func (s *Server) checkGroupContext(cc *CompletionContext, group *scaf.Group, pos lexer.Position) {
	if group.Setup != nil && containsLexerPosition(group.Setup.Span(), pos) {
		cc.InSetup = true
	}
	for _, item := range group.Items {
		if item.Test != nil && containsLexerPosition(item.Test.Span(), pos) {
			cc.InTest = true
			if item.Test.Setup != nil && containsLexerPosition(item.Test.Setup.Span(), pos) {
				cc.InSetup = true
			}
		}
		if item.Group != nil && containsLexerPosition(item.Group.Span(), pos) {
			s.checkGroupContext(cc, item.Group, pos)
		}
	}
}

// determineCompletionKind is the single dispatch point.
// It looks at the token/text before cursor and decides what completion to offer.
// Uses both token-based and text-based detection for robustness.
func (s *Server) determineCompletionKind(cc *CompletionContext, doc *Document, af *analysis.AnalyzedFile, pos lexer.Position, textBeforeCursor string) CompletionKind {
	// Find token before cursor position
	prevToken := s.findPrevToken(doc, af, pos)
	trimmedBefore := strings.TrimSpace(textBeforeCursor)

	// === DISPATCH BASED ON TRIGGER CHARACTER AND CONTEXT ===

	// Case 1: Dot trigger - module.function completion
	if cc.TriggerChar == "." {
		return s.handleDotCompletion(cc, doc, af, pos, prevToken)
	}

	// Case 2: Dollar trigger - parameter completion
	if cc.TriggerChar == "$" || strings.HasPrefix(cc.Prefix, "$") {
		if cc.InTest || cc.InSetup || cc.InAssert || cc.InScope != "" {
			return CompletionKindParameter
		}
	}

	// Case 3: After 'setup' keyword - import alias completion
	// Check both token and text (text is more reliable when file has parse errors)
	if prevToken != nil && prevToken.Type == scaf.TokenSetup {
		return CompletionKindImportAlias
	}
	if strings.HasSuffix(trimmedBefore, "setup") || strings.HasSuffix(trimmedBefore, "setup ") {
		return CompletionKindImportAlias
	}
	// Also check if we're typing after "setup " with a partial identifier
	if strings.Contains(trimmedBefore, "setup ") {
		// Extract what comes after "setup "
		idx := strings.LastIndex(trimmedBefore, "setup ")
		afterSetup := trimmedBefore[idx+6:] // len("setup ") = 6
		if afterSetup == "" || isIdentifierPrefix(afterSetup) {
			return CompletionKindImportAlias
		}
	}

	// Case 4: Inside test body
	if cc.InTest {
		// After colon - value position, no completion
		if prevToken != nil && prevToken.Type == scaf.TokenColon {
			return CompletionKindNone
		}
		// Dollar prefix - parameters
		if strings.HasPrefix(cc.Prefix, "$") {
			return CompletionKindParameter
		}
		// At identifier position - return fields
		if cc.Prefix != "" || prevToken == nil || prevToken.Type == scaf.TokenLBrace ||
			prevToken.Type == scaf.TokenSemi || prevToken.Type == scaf.TokenString ||
			prevToken.Type == scaf.TokenNumber || prevToken.Type == scaf.TokenRBrace {
			// If typing an identifier (not $), offer return fields
			if !strings.HasPrefix(cc.Prefix, "$") {
				return CompletionKindReturnField
			}
		}
		return CompletionKindKeyword
	}

	// Case 5: Top level - query names or keywords
	if cc.InScope == "" {
		if startsWithUpper(cc.Prefix) {
			return CompletionKindQueryName
		}
		return CompletionKindKeyword
	}

	// Case 6: Inside scope but not in test - keywords
	return CompletionKindKeyword
}

// isIdentifierPrefix checks if s looks like an identifier being typed.
func isIdentifierPrefix(s string) bool {
	if s == "" {
		return true
	}
	for _, c := range s {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
			return false
		}
	}
	return true
}

// handleDotCompletion handles the "module." completion case.
// Finds the identifier before the dot and checks if it's an import alias.
func (s *Server) handleDotCompletion(cc *CompletionContext, doc *Document, af *analysis.AnalyzedFile, pos lexer.Position, prevToken *lexer.Token) CompletionKind {
	if af == nil || af.Symbols == nil {
		return CompletionKindNone
	}

	// Find the identifier before the dot
	// prevToken might be the dot itself, or the identifier
	var identValue string

	if prevToken != nil {
		if prevToken.Type == scaf.TokenDot {
			// Token before cursor is the dot - find token before the dot
			tokenBeforeDot := s.findPrevToken(doc, af, prevToken.Pos)
			if tokenBeforeDot != nil && tokenBeforeDot.Type == scaf.TokenIdent {
				identValue = tokenBeforeDot.Value
			}
		} else if prevToken.Type == scaf.TokenIdent {
			// Token before cursor is an identifier (cursor might be on the dot)
			identValue = prevToken.Value
		}
	}

	// Fallback: extract from text before cursor
	// Note: pos is lexer.Position (1-indexed), convert to 0-indexed for text lookup
	if identValue == "" {
		identValue = s.extractIdentifierBeforeDot(doc.Content, pos.Line-1, pos.Column-1)
	}

	// Check if it's an import alias - for module.function completion in setup
	if identValue != "" {
		if _, ok := af.Symbols.Imports[identValue]; ok {
			cc.ModuleAlias = identValue
			cc.Prefix = "" // Reset prefix - we're after the dot
			return CompletionKindSetupFunction
		}
	}

	// If we're in a test, dot could be for return field property access (e.g., "u." -> "u.name")
	// Set the prefix to include the identifier and dot so completeReturnFields can handle it
	if cc.InTest && identValue != "" {
		cc.Prefix = identValue + "."
		return CompletionKindReturnField
	}

	return CompletionKindNone
}

// findPrevToken finds the non-whitespace token immediately before pos.
func (s *Server) findPrevToken(doc *Document, af *analysis.AnalyzedFile, pos lexer.Position) *lexer.Token {
	// Try from current analysis
	if af != nil {
		if tok := analysis.PrevTokenAtPosition(af, pos); tok != nil {
			return tok
		}
	}
	// Try from last valid analysis
	if doc.LastValidAnalysis != nil {
		return analysis.PrevTokenAtPosition(doc.LastValidAnalysis, pos)
	}
	return nil
}

// extractIdentifierBeforeDot extracts the identifier before a dot from text.
// This is a fallback when token lookup fails (e.g., during typing).
func (s *Server) extractIdentifierBeforeDot(content string, line, col int) string {
	lines := strings.Split(content, "\n")
	if line >= len(lines) {
		return ""
	}
	lineText := lines[line]
	if col > len(lineText) {
		col = len(lineText)
	}

	// Find the dot position (should be at col-1 or nearby)
	dotPos := -1
	for i := col - 1; i >= 0; i-- {
		if lineText[i] == '.' {
			dotPos = i
			break
		}
		// Stop if we hit non-identifier chars (except dot)
		c := lineText[i]
		if !unicode.IsLetter(rune(c)) && !unicode.IsDigit(rune(c)) && c != '_' && c != '.' {
			break
		}
	}

	if dotPos < 0 {
		return ""
	}

	// Extract identifier before the dot
	end := dotPos
	start := end
	for i := end - 1; i >= 0; i-- {
		c := rune(lineText[i])
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
			start = i
		} else {
			break
		}
	}

	if start < end {
		return lineText[start:end]
	}
	return ""
}

// completeQueryNames returns completion items for query names.
func (s *Server) completeQueryNames(doc *Document, _ *CompletionContext) []protocol.CompletionItem {
	af := s.getSymbolsAnalysis(doc)
	if af == nil || af.Symbols == nil {
		return nil
	}

	items := make([]protocol.CompletionItem, 0, len(af.Symbols.Queries))
	for name, q := range af.Symbols.Queries {
		item := protocol.CompletionItem{
			Label:            name,
			Kind:             protocol.CompletionItemKindFunction,
			Detail:           "query",
			InsertText:       name + " {\n\t$0\n}",
			InsertTextFormat: protocol.InsertTextFormatSnippet,
		}

		if q.Body != "" {
			preview := strings.TrimSpace(q.Body)
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			item.Documentation = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: s.markdownCodeBlock(preview),
			}
		}
		items = append(items, item)
	}
	return items
}

// keywordSnippet defines a keyword completion with its snippet.
type keywordSnippet struct {
	label      string
	detail     string
	snippet    string
	doc        string
}

// completeKeywords returns keyword completions based on context.
func (s *Server) completeKeywords(cc *CompletionContext) []protocol.CompletionItem {
	var snippets []keywordSnippet

	if cc.InScope == "" {
		// Top level keywords with snippets
		snippets = []keywordSnippet{
			{
				label:   "query",
				detail:  "Define a new query",
				snippet: "query ${1:QueryName} `${2:MATCH (n) RETURN n}`",
				doc:     "Defines a named database query that can be tested.",
			},
			{
				label:   "import",
				detail:  "Import a module",
				snippet: "import ${1:alias} \"${2:./path/to/module}\"",
				doc:     "Imports queries and setup functions from another .scaf file.",
			},
			{
				label:   "setup",
				detail:  "Global setup",
				snippet: "setup ${1|`query`,$module,$module.Query()|}",
				doc:     "Setup to run before all tests in this file.",
			},
			{
				label:   "teardown",
				detail:  "Global teardown",
				snippet: "teardown `${1:MATCH (n) DETACH DELETE n}`",
				doc:     "Teardown to run after all tests in this file.",
			},
		}
	} else if cc.InTest {
		// Inside test
		snippets = []keywordSnippet{
			{
				label:   "setup",
				detail:  "Test-specific setup",
				snippet: "setup ${1|$module.Query(),$module|}",
				doc:     "Setup to run before this specific test.",
			},
			{
				label:   "assert",
				detail:  "Add assertion query",
				snippet: "assert ${1:QueryName}(${2:params}) {\n\t$0\n}",
				doc:     "Assert the results of another query after the main query runs.",
			},
		}
	} else {
		// Inside scope, not in test
		snippets = []keywordSnippet{
			{
				label:   "test",
				detail:  "Define a test case",
				snippet: "test \"${1:test name}\" {\n\t${2:\\$param: value}\n\t${3:field: expected}\n}",
				doc:     "Defines a test case with inputs and expected outputs.",
			},
			{
				label:   "group",
				detail:  "Group related tests",
				snippet: "group \"${1:group name}\" {\n\t${0}\n}",
				doc:     "Groups related test cases together. Can have its own setup/teardown.",
			},
			{
				label:   "setup",
				detail:  "Scope-level setup",
				snippet: "setup ${1|$module.Query(),$module|}",
				doc:     "Setup to run before all tests in this scope.",
			},
		}
	}

	items := make([]protocol.CompletionItem, 0, len(snippets))
	for _, ks := range snippets {
		item := protocol.CompletionItem{
			Label:            ks.label,
			Kind:             protocol.CompletionItemKindKeyword,
			Detail:           ks.detail,
			InsertText:       ks.snippet,
			InsertTextFormat: protocol.InsertTextFormatSnippet,
		}
		if ks.doc != "" {
			item.Documentation = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: ks.doc,
			}
		}
		items = append(items, item)
	}
	return items
}

// completeParameters returns parameter completions from the query in scope.
func (s *Server) completeParameters(doc *Document, cc *CompletionContext) []protocol.CompletionItem {
	af := s.getSymbolsAnalysis(doc)
	if af == nil || af.Symbols == nil || cc.InScope == "" {
		return nil
	}

	q, ok := af.Symbols.Queries[cc.InScope]
	if !ok {
		return nil
	}

	items := make([]protocol.CompletionItem, 0, len(q.Params))
	for _, param := range q.Params {
		items = append(items, protocol.CompletionItem{
			Label:      "$" + param,
			Kind:       protocol.CompletionItemKindVariable,
			Detail:     "parameter",
			InsertText: "$" + param + ": ",
		})
	}
	return items
}

// completeReturnFields returns return field completions from the query in scope.
func (s *Server) completeReturnFields(doc *Document, cc *CompletionContext) []protocol.CompletionItem {
	af := s.getSymbolsAnalysis(doc)
	if af == nil || af.Symbols == nil || cc.InScope == "" {
		return nil
	}

	q, ok := af.Symbols.Queries[cc.InScope]
	if !ok || q.Body == "" {
		return nil
	}

	if s.queryAnalyzer == nil {
		return nil
	}

	metadata, err := s.queryAnalyzer.AnalyzeQuery(q.Body)
	if err != nil {
		s.logger.Debug("Failed to analyze query for return fields", zap.Error(err))
		return nil
	}

	// Check if the prefix contains a dot (e.g., "u." or "u.na")
	// If so, we need to match the prefix and show only the property part
	var prefixBase string // e.g., "u" from "u." or "u.na"
	var prefixProp string // e.g., "" from "u." or "na" from "u.na"
	if dotIdx := strings.LastIndex(cc.Prefix, "."); dotIdx >= 0 {
		prefixBase = cc.Prefix[:dotIdx]
		prefixProp = cc.Prefix[dotIdx+1:]
	}

	items := make([]protocol.CompletionItem, 0, len(metadata.Returns))
	for _, ret := range metadata.Returns {
		// Use the full expression (e.g., "u.name") as the base
		// If there's an alias, use that instead (it's the actual column name)
		fullExpr := ret.Expression
		if ret.Alias != "" {
			fullExpr = ret.Alias
		}

		// Determine label and insertText based on whether user typed a prefix with dot
		var label, insertText string
		if prefixBase != "" {
			// User typed something like "u." - check if this expression starts with "u."
			if !strings.HasPrefix(fullExpr, prefixBase+".") {
				continue // This field doesn't match the prefix base
			}
			// Extract just the property part after the prefix
			propPart := fullExpr[len(prefixBase)+1:]
			// Filter by property prefix if any
			if prefixProp != "" && !strings.HasPrefix(strings.ToLower(propPart), strings.ToLower(prefixProp)) {
				continue
			}
			label = propPart
			insertText = propPart + ": "
		} else {
			// No dot prefix - show full expression
			label = fullExpr
			insertText = fullExpr + ": "
		}

		item := protocol.CompletionItem{
			Label:      label,
			Kind:       protocol.CompletionItemKindField,
			Detail:     "return field",
			InsertText: insertText,
		}
		if ret.Alias != "" && ret.Expression != ret.Alias {
			item.Documentation = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: "Expression: `" + ret.Expression + "`",
			}
		}
		if ret.IsAggregate {
			item.Detail = "aggregate field"
		}
		items = append(items, item)
	}
	return items
}

// completeImportAliases returns import alias completions.
func (s *Server) completeImportAliases(doc *Document, _ *CompletionContext) []protocol.CompletionItem {
	af := s.getSymbolsAnalysis(doc)
	if af == nil || af.Symbols == nil {
		return nil
	}

	items := make([]protocol.CompletionItem, 0, len(af.Symbols.Imports))
	for alias, imp := range af.Symbols.Imports {
		items = append(items, protocol.CompletionItem{
			Label:  alias,
			Kind:   protocol.CompletionItemKindModule,
			Detail: imp.Path,
		})
	}
	return items
}

// completeSetupFunctions returns setup function completions from imported module.
func (s *Server) completeSetupFunctions(doc *Document, cc *CompletionContext) []protocol.CompletionItem {
	if cc.ModuleAlias == "" {
		return nil
	}

	af := s.getSymbolsAnalysis(doc)
	if af == nil || af.Symbols == nil {
		return nil
	}

	if s.fileLoader == nil {
		s.logger.Debug("FileLoader not available for cross-file completion")
		return nil
	}

	imp, ok := af.Symbols.Imports[cc.ModuleAlias]
	if !ok {
		s.logger.Debug("Import not found for module alias", zap.String("alias", cc.ModuleAlias))
		return nil
	}

	// Resolve and load the imported file
	docPath := URIToPath(doc.URI)
	importedPath := s.fileLoader.ResolveImportPath(docPath, imp.Path)

	s.logger.Debug("Resolving import for setup completion",
		zap.String("alias", cc.ModuleAlias),
		zap.String("importPath", imp.Path),
		zap.String("resolvedPath", importedPath))

	importedFile, err := s.fileLoader.LoadAndAnalyze(importedPath)
	if err != nil {
		s.logger.Debug("Failed to load imported file",
			zap.String("path", importedPath),
			zap.Error(err))
		return nil
	}

	if importedFile.Symbols == nil {
		return nil
	}

	var items []protocol.CompletionItem

	// Add queries from imported file as setup functions
	for name, q := range importedFile.Symbols.Queries {
		item := protocol.CompletionItem{
			Label:  name,
			Kind:   protocol.CompletionItemKindFunction,
			Detail: "query from " + cc.ModuleAlias,
		}

		if q.Body != "" {
			preview := strings.TrimSpace(q.Body)
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			item.Documentation = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: s.markdownCodeBlock(preview),
			}
		}

		// Build snippet with parameter placeholders
		if len(q.Params) > 0 {
			insertText := name + "("
			for i, p := range q.Params {
				if i > 0 {
					insertText += ", "
				}
				insertText += "$" + p + ": ${" + string(rune('1'+i)) + "}"
			}
			insertText += ")"
			item.InsertText = insertText
			item.InsertTextFormat = protocol.InsertTextFormatSnippet
		} else {
			item.InsertText = name + "()"
		}

		items = append(items, item)
	}

	return items
}

// getSymbolsAnalysis returns the best analysis for symbol lookup.
func (s *Server) getSymbolsAnalysis(doc *Document) *analysis.AnalyzedFile {
	if doc.Analysis != nil && doc.Analysis.ParseError == nil {
		return doc.Analysis
	}
	if doc.LastValidAnalysis != nil {
		return doc.LastValidAnalysis
	}
	return doc.Analysis
}

// filterByPrefix filters completion items by prefix.
func filterByPrefix(items []protocol.CompletionItem, prefix string) []protocol.CompletionItem {
	if prefix == "" {
		return items
	}

	prefix = strings.ToLower(prefix)
	filtered := make([]protocol.CompletionItem, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Label), prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// extractPrefix extracts the identifier prefix being typed.
func extractPrefix(text string) string {
	end := len(text)
	start := end

	for i := end - 1; i >= 0; i-- {
		c := rune(text[i])
		// Include dots to support "u.name" style prefixes for return field completion
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '$' || c == '.' {
			start = i
		} else {
			break
		}
	}
	return text[start:end]
}

// startsWithUpper checks if a string starts with an uppercase letter.
func startsWithUpper(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '$' && len(s) > 1 {
		s = s[1:]
	}
	return unicode.IsUpper(rune(s[0]))
}

// containsLexerPosition checks if a span contains a lexer position.
func containsLexerPosition(span scaf.Span, pos lexer.Position) bool {
	if pos.Line < span.Start.Line || (pos.Line == span.Start.Line && pos.Column < span.Start.Column) {
		return false
	}
	if pos.Line > span.End.Line || (pos.Line == span.End.Line && pos.Column > span.End.Column) {
		return false
	}
	return true
}

// markdownCodeBlock wraps code in a markdown code block.
func (s *Server) markdownCodeBlock(code string) string {
	lang := scaf.MarkdownLanguage(s.dialectName)
	return "```" + lang + "\n" + code + "\n```"
}
