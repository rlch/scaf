package scaf

import "github.com/alecthomas/participle/v2/lexer"

// Span represents a range in source code.
type Span struct {
	Start lexer.Position
	End   lexer.Position
}

// Trivia represents non-semantic tokens like comments and whitespace.
type Trivia struct {
	Type  TriviaType
	Text  string
	Span  Span
	// HasNewlineBefore is true if there was a blank line before this trivia.
	// Useful for distinguishing "detached" comments.
	HasNewlineBefore bool
}

// TriviaType distinguishes different kinds of trivia.
type TriviaType int

// TriviaType constants define the types of trivia (comments, whitespace).
const (
	// TriviaComment represents a comment trivia.
	TriviaComment TriviaType = iota
	// TriviaWhitespace represents whitespace trivia.
	TriviaWhitespace
)

// TriviaList holds all trivia collected during lexing.
// It's associated with tokens by position - trivia "attaches" to the next
// real token as leading trivia, except trailing comments (same line) attach
// to the previous token.
type TriviaList struct {
	items []Trivia
}

// Add appends trivia to the list.
func (t *TriviaList) Add(trivia Trivia) {
	t.items = append(t.items, trivia)
}

// All returns all collected trivia.
func (t *TriviaList) All() []Trivia {
	return t.items
}

// Reset clears the trivia list.
func (t *TriviaList) Reset() {
	t.items = t.items[:0]
}

// commentMap stores comments for AST nodes, keyed by their Span.
// This is used internally during comment attachment.
type commentMap map[Span]*nodeComments

// nodeComments holds comments attached to an AST node.
type nodeComments struct {
	leading  []string // Comments before the node
	trailing string   // Comment on same line after node (empty if none)
}

// attachComments associates collected trivia with AST nodes based on positions,
// applying the comments directly to the node fields.
func attachComments(suite *Suite, trivia *TriviaList) {
	if trivia == nil || len(trivia.items) == 0 {
		return
	}

	cm := make(commentMap)
	allTrivia := trivia.All()

	// Build a list of all node spans
	var spans []Span
	collectSpans(suite, &spans)

	// For each trivia item, find which node it belongs to
	for _, t := range allTrivia {
		if t.Type != TriviaComment {
			continue
		}

		commentText := t.Text

		// Check if it's a trailing comment (same line as end of some node)
		attached := false

		for _, span := range spans {
			// Trailing: comment starts on same line as node ends, after the node
			if t.Span.Start.Line == span.End.Line && t.Span.Start.Offset > span.End.Offset {
				if cm[span] == nil {
					cm[span] = &nodeComments{}
				}

				cm[span].trailing = commentText
				attached = true

				break
			}
		}

		if attached {
			continue
		}

		// Leading: find the node that starts after this comment
		for _, span := range spans {
			// Comment ends before node starts (on previous line or same line before)
			if t.Span.End.Line < span.Start.Line ||
				(t.Span.End.Line == span.Start.Line && t.Span.End.Offset < span.Start.Offset) {
				// Check there's no other node between the comment and this node
				if isClosestNode(t.Span, span, spans) {
					if cm[span] == nil {
						cm[span] = &nodeComments{}
					}

					cm[span].leading = append(cm[span].leading, commentText)

					break
				}
			}
		}
	}

	// Now apply the comment map to the actual nodes
	applyComments(suite, cm)
}

// applyComments transfers comments from the map to the AST node fields.
func applyComments(suite *Suite, cm commentMap) {
	if suite == nil {
		return
	}

	// Suite
	if c := cm[suite.Span()]; c != nil {
		suite.LeadingComments = c.leading
		suite.TrailingComment = c.trailing
	}

	// Imports
	for _, imp := range suite.Imports {
		if c := cm[imp.Span()]; c != nil {
			imp.LeadingComments = c.leading
			imp.TrailingComment = c.trailing
		}
	}

	// Queries
	for _, q := range suite.Queries {
		if c := cm[q.Span()]; c != nil {
			q.LeadingComments = c.leading
			q.TrailingComment = c.trailing
		}
	}

	// Scopes
	for _, scope := range suite.Scopes {
		applyScopeComments(scope, cm)
	}
}

func applyScopeComments(scope *QueryScope, cm commentMap) {
	if scope == nil {
		return
	}

	if c := cm[scope.Span()]; c != nil {
		scope.LeadingComments = c.leading
		scope.TrailingComment = c.trailing
	}

	for _, item := range scope.Items {
		if item.Test != nil {
			if c := cm[item.Test.Span()]; c != nil {
				item.Test.LeadingComments = c.leading
				item.Test.TrailingComment = c.trailing
			}
		}

		if item.Group != nil {
			applyGroupComments(item.Group, cm)
		}
	}
}

func applyGroupComments(group *Group, cm commentMap) {
	if group == nil {
		return
	}

	if c := cm[group.Span()]; c != nil {
		group.LeadingComments = c.leading
		group.TrailingComment = c.trailing
	}

	for _, item := range group.Items {
		if item.Test != nil {
			if c := cm[item.Test.Span()]; c != nil {
				item.Test.LeadingComments = c.leading
				item.Test.TrailingComment = c.trailing
			}
		}

		if item.Group != nil {
			applyGroupComments(item.Group, cm)
		}
	}
}

// isClosestNode checks if targetSpan is the closest node after the comment.
func isClosestNode(commentSpan, targetSpan Span, allSpans []Span) bool {
	for _, span := range allSpans {
		// Skip the target itself
		if span == targetSpan {
			continue
		}
		// Check if this span is between the comment and target
		if span.Start.Line > commentSpan.End.Line && span.Start.Line < targetSpan.Start.Line {
			return false
		}

		if span.Start.Line == commentSpan.End.Line &&
			span.Start.Offset > commentSpan.End.Offset &&
			span.Start.Line < targetSpan.Start.Line {
			return false
		}
	}

	return true
}

// collectSpans gathers all spans from the AST.
func collectSpans(suite *Suite, spans *[]Span) {
	if suite == nil {
		return
	}

	*spans = append(*spans, suite.Span())

	for _, imp := range suite.Imports {
		*spans = append(*spans, imp.Span())
	}

	for _, q := range suite.Queries {
		*spans = append(*spans, q.Span())
	}

	for _, scope := range suite.Scopes {
		collectScopeSpans(scope, spans)
	}
}

func collectScopeSpans(scope *QueryScope, spans *[]Span) {
	if scope == nil {
		return
	}

	*spans = append(*spans, scope.Span())

	for _, item := range scope.Items {
		if item.Test != nil {
			*spans = append(*spans, item.Test.Span())
		}

		if item.Group != nil {
			collectGroupSpans(item.Group, spans)
		}
	}
}

func collectGroupSpans(group *Group, spans *[]Span) {
	if group == nil {
		return
	}

	*spans = append(*spans, group.Span())

	for _, item := range group.Items {
		if item.Test != nil {
			*spans = append(*spans, item.Test.Span())
		}

		if item.Group != nil {
			collectGroupSpans(item.Group, spans)
		}
	}
}