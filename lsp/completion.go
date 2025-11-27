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
func (s *Server) Completion(_ context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	s.logger.Debug("Completion",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok {
		return nil, nil //nolint:nilnil
	}

	// Use last valid analysis for symbol lookups if current parse failed
	// This allows completion to work while the user is typing (and the file is temporarily invalid)
	analysisForCompletion := doc.Analysis
	if doc.Analysis != nil && doc.Analysis.ParseError != nil && doc.LastValidAnalysis != nil {
		s.logger.Debug("Using last valid analysis for completion (current has parse error)")
		analysisForCompletion = doc.LastValidAnalysis
	}

	// Create a temporary document view with the valid analysis for completion
	completionDoc := &Document{
		URI:      doc.URI,
		Version:  doc.Version,
		Content:  doc.Content,
		Analysis: analysisForCompletion,
	}

	// Get trigger character from LSP context if provided
	var triggerChar string
	if params.Context != nil && params.Context.TriggerCharacter != "" {
		triggerChar = params.Context.TriggerCharacter
	}

	// Get completion context
	cc := s.getCompletionContext(completionDoc, params.Position, triggerChar)
	s.logger.Debug("Completion context", zap.String("kind", string(cc.Kind)))

	var items []protocol.CompletionItem

	switch cc.Kind {
	case CompletionKindNone:
		// No completions available at this position
	case CompletionKindQueryName:
		items = s.completeQueryNames(completionDoc, cc)
	case CompletionKindKeyword:
		items = s.completeKeywords(cc)
	case CompletionKindParameter:
		items = s.completeParameters(completionDoc, cc)
	case CompletionKindReturnField:
		items = s.completeReturnFields(completionDoc, cc)
	case CompletionKindImportAlias:
		items = s.completeImportAliases(completionDoc, cc)
	case CompletionKindSetupFunction:
		items = s.completeSetupFunctions(completionDoc, cc)
	}

	// Filter by prefix if present
	if cc.Prefix != "" {
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
	// CompletionKindNone indicates no specific completion context.
	CompletionKindNone CompletionKind = "none"
	// CompletionKindQueryName indicates completion for query names.
	CompletionKindQueryName CompletionKind = "query_name"
	// CompletionKindKeyword indicates completion for keywords.
	CompletionKindKeyword CompletionKind = "keyword"
	// CompletionKindParameter indicates completion for query parameters.
	CompletionKindParameter CompletionKind = "parameter"
	// CompletionKindReturnField indicates completion for query return fields.
	CompletionKindReturnField CompletionKind = "return_field"
	// CompletionKindImportAlias indicates completion for import aliases.
	CompletionKindImportAlias CompletionKind = "import_alias"
	// CompletionKindSetupFunction indicates completion for setup functions from imports.
	CompletionKindSetupFunction CompletionKind = "setup_function"
)

// CompletionContext holds information about where completion was triggered.
type CompletionContext struct {
	Kind         CompletionKind
	Prefix       string // Text before cursor (for filtering)
	InScope      string // Name of enclosing QueryScope, if any
	InTest       bool   // True if inside a test body
	InSetup      bool   // True if inside a setup clause
	InAssert     bool   // True if inside an assert block
	LineText     string // Full text of the current line
	TriggerChar  string // The trigger character, if any
	ModuleAlias  string // Import alias before the dot (for setup function completion)
	lineNumber   int    // 0-based line number (for internal use)
	columnNumber int    // 0-based column number (for internal use)
}

// getCompletionContext analyzes the document to determine completion context.
func (s *Server) getCompletionContext(doc *Document, pos protocol.Position, triggerChar string) *CompletionContext {
	cc := &CompletionContext{
		Kind:         CompletionKindNone,
		lineNumber:   int(pos.Line),
		columnNumber: int(pos.Character),
	}

	// Get the current line text
	lines := strings.Split(doc.Content, "\n")
	if int(pos.Line) >= len(lines) {
		return cc
	}

	cc.LineText = lines[pos.Line]

	// Get text before cursor on this line
	col := min(int(pos.Character), len(cc.LineText))

	textBeforeCursor := cc.LineText[:col]
	trimmedBefore := strings.TrimLeft(textBeforeCursor, " \t")

	// Extract prefix (identifier being typed)
	cc.Prefix = extractPrefix(textBeforeCursor)

	// Check for trigger character - prefer LSP-provided, fallback to text detection
	if triggerChar != "" {
		cc.TriggerChar = triggerChar
	} else if col > 0 {
		lastChar := cc.LineText[col-1]
		if lastChar == '$' || lastChar == '.' {
			cc.TriggerChar = string(lastChar)
		}
	}

	// Determine context based on AST position
	if doc.Analysis != nil && doc.Analysis.Suite != nil {
		lexPos := analysis.PositionToLexer(pos.Line, pos.Character)
		
		// Find enclosing scope
		for _, scope := range doc.Analysis.Suite.Scopes {
			if containsLexerPosition(scope.Span(), lexPos) {
				cc.InScope = scope.QueryName
				
				// Check if in setup
				if scope.Setup != nil && isInSetup(scope.Setup, lexPos, doc.Content) {
					cc.InSetup = true
				}
				
				// Check if in test/group
				for _, item := range scope.Items {
					if item.Test != nil && containsLexerPosition(item.Test.Span(), lexPos) {
						cc.InTest = true
						// Check for assert
						for _, assert := range item.Test.Asserts {
							if isInAssertBlock(assert, lexPos, doc.Content) {
								cc.InAssert = true
							}
						}
					}

					if item.Group != nil && containsLexerPosition(item.Group.Span(), lexPos) {
						cc.InTest = checkInTestWithinGroup(item.Group, lexPos)
					}
				}
			}
		}
	}

	// Determine completion kind based on context
	cc.Kind = s.determineCompletionKind(cc, trimmedBefore, doc)

	return cc
}

// determineCompletionKind figures out what kind of completions to offer.
// Uses AST-based detection via token types instead of string matching.
func (s *Server) determineCompletionKind(cc *CompletionContext, trimmedBefore string, doc *Document) CompletionKind {
	// Get token context for AST-based detection
	var tokenCtx *analysis.TokenContext
	if doc.Analysis != nil {
		lexPos := analysis.PositionToLexer(uint32(cc.lineNumber), uint32(cc.columnNumber))
		tokenCtx = analysis.GetTokenContext(doc.Analysis, lexPos)
	}

	// AST-based: Check for parameter completion ($ trigger or $-prefixed identifier)
	if cc.TriggerChar == "$" || strings.HasPrefix(cc.Prefix, "$") {
		if cc.InTest || cc.InSetup || cc.InAssert {
			return CompletionKindParameter
		}
	}

	// AST-based: Check for setup function completion (. trigger after module alias)
	if cc.TriggerChar == "." {
		// Use token context to find what's before the dot
		if tokenCtx != nil && tokenCtx.PrevToken != nil {
			if tokenCtx.PrevToken.Type == scaf.TokenIdent {
				moduleAlias := tokenCtx.PrevToken.Value
				if doc.Analysis != nil && doc.Analysis.Symbols != nil {
					if _, ok := doc.Analysis.Symbols.Imports[moduleAlias]; ok {
						cc.ModuleAlias = moduleAlias
						return CompletionKindSetupFunction
					}
				}
			}
		}
		// Fallback to string-based extraction
		moduleAlias := extractModuleBeforeDot(trimmedBefore)
		if moduleAlias != "" && doc.Analysis != nil && doc.Analysis.Symbols != nil {
			if _, ok := doc.Analysis.Symbols.Imports[moduleAlias]; ok {
				cc.ModuleAlias = moduleAlias
				return CompletionKindSetupFunction
			}
		}
		// Dot in other contexts - no special completion
		return CompletionKindNone
	}

	// AST-based: Check if we're on or after a 'setup' keyword token
	if tokenCtx != nil {
		// If current token is the 'setup' keyword
		if tokenCtx.Token != nil && tokenCtx.Token.Type == scaf.TokenSetup {
			return CompletionKindImportAlias
		}
		// If previous token is 'setup' (cursor is right after "setup ")
		if tokenCtx.PrevToken != nil && tokenCtx.PrevToken.Type == scaf.TokenSetup {
			return CompletionKindImportAlias
		}
	}

	// AST-based: Check if we're in an incomplete setup clause
	if cc.InSetup && (cc.Prefix == "" || !strings.HasPrefix(cc.Prefix, "$")) {
		if doc.Analysis != nil && doc.Analysis.Suite != nil {
			node := s.findNodeAtPosition(doc, cc)
			if setup, ok := node.(*scaf.SetupClause); ok && !setup.IsComplete() {
				return CompletionKindImportAlias
			}
		}
	}

	// Fallback: string-based detection for 'setup' keyword (for parse errors)
	trimmedLower := strings.ToLower(strings.TrimSpace(trimmedBefore))
	if trimmedLower == "setup" || strings.HasPrefix(trimmedLower, "setup ") {
		return CompletionKindImportAlias
	}

	// AST-based: Inside a test - offer return fields or parameters
	if cc.InTest && !strings.HasPrefix(cc.Prefix, "$") {
		if tokenCtx != nil && tokenCtx.PrevToken != nil {
			// After a colon - we're in a value position, not completion context
			if tokenCtx.PrevToken.Type == scaf.TokenColon {
				return CompletionKindNone
			}
			// After open brace or at start of statement - offer return fields
			if tokenCtx.PrevToken.Type == scaf.TokenLBrace ||
				tokenCtx.PrevToken.Type == scaf.TokenIdent ||
				tokenCtx.PrevToken.Type == scaf.TokenNumber {
				return CompletionKindReturnField
			}
		}
		// Fallback: string-based check for colon
		if !strings.Contains(trimmedBefore, ":") {
			return CompletionKindReturnField
		}
	}

	// AST-based: Top level detection
	if tokenCtx != nil && cc.InScope == "" {
		// After 'query' keyword - expecting identifier
		if tokenCtx.PrevToken != nil && tokenCtx.PrevToken.Type == scaf.TokenQuery {
			return CompletionKindNone // Let user type query name
		}
		// After 'import' keyword - expecting alias or path
		if tokenCtx.PrevToken != nil && tokenCtx.PrevToken.Type == scaf.TokenImport {
			return CompletionKindNone // Let user type import
		}
		// At start of line or after specific tokens - offer query names
		if tokenCtx.PrevToken == nil || tokenCtx.PrevToken.Type == scaf.TokenRBrace {
			if startsWithUpper(cc.Prefix) {
				return CompletionKindQueryName
			}
			return CompletionKindKeyword
		}
	}

	// Fallback: At top level or start of scope - keywords and query names
	if cc.InScope == "" {
		if isAtLineStart(trimmedBefore) || cc.Prefix != "" {
			if startsWithUpper(cc.Prefix) {
				return CompletionKindQueryName
			}
			return CompletionKindKeyword
		}
	} else {
		// Inside a scope but not in test
		if !cc.InTest && isAtLineStart(trimmedBefore) {
			return CompletionKindKeyword
		}
	}

	// Default: try query names if we have a capital letter prefix
	if startsWithUpper(cc.Prefix) && doc.Analysis != nil {
		return CompletionKindQueryName
	}

	return CompletionKindKeyword
}

// findNodeAtPosition finds the AST node at the current completion position.
func (s *Server) findNodeAtPosition(doc *Document, cc *CompletionContext) scaf.Node {
	if doc.Analysis == nil {
		return nil
	}
	lexPos := analysis.PositionToLexer(uint32(cc.lineNumber), uint32(cc.columnNumber))
	return analysis.NodeAtPosition(doc.Analysis, lexPos)
}

// completeQueryNames returns completion items for query names.
func (s *Server) completeQueryNames(doc *Document, _ *CompletionContext) []protocol.CompletionItem {
	if doc.Analysis == nil || doc.Analysis.Symbols == nil {
		return nil
	}

	items := make([]protocol.CompletionItem, 0, len(doc.Analysis.Symbols.Queries))

	for name, q := range doc.Analysis.Symbols.Queries {
		item := protocol.CompletionItem{
			Label:      name,
			Kind:       protocol.CompletionItemKindFunction,
			Detail:     "query",
			InsertText: name + " {\n\t$0\n}",
			InsertTextFormat: protocol.InsertTextFormatSnippet,
		}

		// Add documentation with query preview
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

// completeKeywords returns completion items for scaf keywords.
func (s *Server) completeKeywords(cc *CompletionContext) []protocol.CompletionItem {
	var keywords []string

	switch {
	case cc.InScope == "":
		// Top-level keywords
		keywords = []string{"query", "import", "setup", "teardown"}
	case cc.InTest:
		// Inside test
		keywords = []string{"setup", "assert"}
	default:
		// Inside scope but not test
		keywords = []string{"setup", "teardown", "test", "group"}
	}

	items := make([]protocol.CompletionItem, 0, len(keywords))
	for _, kw := range keywords {
		items = append(items, protocol.CompletionItem{
			Label:  kw,
			Kind:   protocol.CompletionItemKindKeyword,
			Detail: "keyword",
		})
	}

	return items
}

// completeParameters returns completion items for query parameters.
func (s *Server) completeParameters(doc *Document, cc *CompletionContext) []protocol.CompletionItem {
	if doc.Analysis == nil || doc.Analysis.Symbols == nil || cc.InScope == "" {
		return nil
	}

	// Find the query for this scope
	q, ok := doc.Analysis.Symbols.Queries[cc.InScope]
	if !ok || q.Body == "" {
		return nil
	}

	// If no query analyzer is available, fall back to regex-extracted params
	if s.queryAnalyzer == nil {
		return s.completeParametersFromSymbols(q.Params)
	}

	// Use the dialect-specific analyzer to get parameters
	metadata, err := s.queryAnalyzer.AnalyzeQuery(q.Body)
	if err != nil {
		s.logger.Debug("Failed to analyze query for completion", zap.Error(err))
		// Fall back to regex-extracted params
		return s.completeParametersFromSymbols(q.Params)
	}

	items := make([]protocol.CompletionItem, 0, len(metadata.Parameters))
	for _, param := range metadata.Parameters {
		items = append(items, protocol.CompletionItem{
			Label:      "$" + param.Name,
			Kind:       protocol.CompletionItemKindVariable,
			Detail:     "parameter",
			InsertText: "$" + param.Name + ": ",
		})
	}

	return items
}

// completeParametersFromSymbols is a fallback when query analysis fails.
func (s *Server) completeParametersFromSymbols(params []string) []protocol.CompletionItem {
	items := make([]protocol.CompletionItem, 0, len(params))

	for _, param := range params {
		items = append(items, protocol.CompletionItem{
			Label:      "$" + param,
			Kind:       protocol.CompletionItemKindVariable,
			Detail:     "parameter",
			InsertText: "$" + param + ": ",
		})
	}

	return items
}

// completeReturnFields returns completion items for query return fields.
func (s *Server) completeReturnFields(doc *Document, cc *CompletionContext) []protocol.CompletionItem {
	if doc.Analysis == nil || doc.Analysis.Symbols == nil || cc.InScope == "" {
		return nil
	}

	// Find the query for this scope
	q, ok := doc.Analysis.Symbols.Queries[cc.InScope]
	if !ok || q.Body == "" {
		return nil
	}

	// If no query analyzer is available, we can't provide return field completion
	if s.queryAnalyzer == nil {
		return nil
	}

	// Use the dialect-specific analyzer to get return fields
	metadata, err := s.queryAnalyzer.AnalyzeQuery(q.Body)
	if err != nil {
		s.logger.Debug("Failed to analyze query for return fields", zap.Error(err))

		return nil
	}

	items := make([]protocol.CompletionItem, 0, len(metadata.Returns))

	for _, ret := range metadata.Returns {
		item := protocol.CompletionItem{
			Label:      ret.Name,
			Kind:       protocol.CompletionItemKindField,
			Detail:     "return field",
			InsertText: ret.Name + ": ",
		}

		// Add documentation showing the expression
		if ret.Expression != ret.Name {
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

// completeImportAliases returns completion items for import aliases.
func (s *Server) completeImportAliases(doc *Document, _ *CompletionContext) []protocol.CompletionItem {
	if doc.Analysis == nil || doc.Analysis.Symbols == nil {
		return nil
	}

	items := make([]protocol.CompletionItem, 0, len(doc.Analysis.Symbols.Imports))

	for alias, imp := range doc.Analysis.Symbols.Imports {
		items = append(items, protocol.CompletionItem{
			Label:  alias,
			Kind:   protocol.CompletionItemKindModule,
			Detail: imp.Path,
		})
	}

	return items
}

// completeSetupFunctions returns completion items for setup functions from imported modules.
// This is triggered after typing "module." where module is an import alias.
func (s *Server) completeSetupFunctions(doc *Document, cc *CompletionContext) []protocol.CompletionItem {
	if doc.Analysis == nil || doc.Analysis.Symbols == nil || cc.ModuleAlias == "" {
		return nil
	}

	// Ensure fileLoader is available
	if s.fileLoader == nil {
		s.logger.Debug("FileLoader not available for cross-file completion")
		return nil
	}

	// Get the import for this alias
	imp, ok := doc.Analysis.Symbols.Imports[cc.ModuleAlias]
	if !ok {
		s.logger.Debug("Import not found for module alias", zap.String("alias", cc.ModuleAlias))
		return nil
	}

	// Resolve the import path to an absolute file path
	docPath := URIToPath(doc.URI)
	importedPath := s.fileLoader.ResolveImportPath(docPath, imp.Path)

	s.logger.Debug("Resolving import for setup completion",
		zap.String("alias", cc.ModuleAlias),
		zap.String("importPath", imp.Path),
		zap.String("resolvedPath", importedPath))

	// Load and analyze the imported file
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

	// Add queries from the imported file as potential setup functions
	// In scaf, queries can be referenced as setup calls: module.QueryName($params)
	for name, q := range importedFile.Symbols.Queries {
		item := protocol.CompletionItem{
			Label:  name,
			Kind:   protocol.CompletionItemKindFunction,
			Detail: "query from " + cc.ModuleAlias,
		}

		// Add documentation with query preview
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

		// Build insert text with parameter placeholders
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

	// TODO: Add named setups if the imported file defines them

	return items
}

// filterByPrefix filters completion items by a prefix.
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
	// Walk backwards from end to find start of identifier
	end := len(text)
	start := end

	for i := end - 1; i >= 0; i-- {
		c := rune(text[i])
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '$' {
			start = i
		} else {
			break
		}
	}

	return text[start:end]
}

// isAtLineStart checks if we're at the start of meaningful content.
func isAtLineStart(trimmedBefore string) bool {
	return trimmedBefore == "" || len(trimmedBefore) == len(extractPrefix(trimmedBefore))
}

// startsWithUpper checks if a string starts with an uppercase letter.
func startsWithUpper(s string) bool {
	if s == "" {
		return false
	}
	// Skip $ prefix if present
	if s[0] == '$' && len(s) > 1 {
		s = s[1:]
	}

	return unicode.IsUpper(rune(s[0]))
}

// extractModuleBeforeDot extracts the identifier before the last dot.
// For "setup fixtures." returns "fixtures"
// For "fixtures." returns "fixtures"
// For "foo.bar." returns "bar"
func extractModuleBeforeDot(text string) string {
	// Trim trailing whitespace
	text = strings.TrimRight(text, " \t")

	// Must end with a dot (the trigger character)
	if !strings.HasSuffix(text, ".") {
		return ""
	}

	// Remove the trailing dot
	text = text[:len(text)-1]

	// Walk backwards to find the start of the identifier
	end := len(text)
	start := end

	for i := end - 1; i >= 0; i-- {
		c := rune(text[i])
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
			start = i
		} else {
			break
		}
	}

	if start == end {
		return ""
	}

	return text[start:end]
}

// containsLexerPosition checks if a span contains a lexer position.
func containsLexerPosition(span scaf.Span, pos lexer.Position) bool {
	// Check if pos is after start.
	if pos.Line < span.Start.Line {
		return false
	}

	if pos.Line == span.Start.Line && pos.Column < span.Start.Column {
		return false
	}

	// Check if pos is before end.
	if pos.Line > span.End.Line {
		return false
	}

	if pos.Line == span.End.Line && pos.Column > span.End.Column {
		return false
	}

	return true
}

// Helper functions for checking position within specific constructs

func isInSetup(setup *scaf.SetupClause, pos lexer.Position, _ string) bool {
	if setup == nil {
		return false
	}
	return containsLexerPosition(setup.Span(), pos)
}

func isInAssertBlock(assert *scaf.Assert, pos lexer.Position, _ string) bool {
	if assert == nil {
		return false
	}
	return containsLexerPosition(assert.Span(), pos)
}

func checkInTestWithinGroup(group *scaf.Group, pos lexer.Position) bool {
	for _, item := range group.Items {
		if item.Test != nil && containsLexerPosition(item.Test.Span(), pos) {
			return true
		}

		if item.Group != nil {
			if checkInTestWithinGroup(item.Group, pos) {
				return true
			}
		}
	}

	return false
}

// markdownCodeBlock wraps code in a markdown code block with the appropriate language.
func (s *Server) markdownCodeBlock(code string) string {
	lang := scaf.MarkdownLanguage(s.dialectName)
	return "```" + lang + "\n" + code + "\n```"
}
