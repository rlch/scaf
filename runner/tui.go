package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/rlch/scaf"
)

// TUIFormatter implements Formatter with an animated terminal UI.
type TUIFormatter struct {
	program  *tea.Program
	model    *tuiModel
	mu       sync.Mutex
	finished bool
}

// NewTUIFormatter creates a TUI formatter with animations.
func NewTUIFormatter(w io.Writer, suites []SuiteTree) *TUIFormatter {
	model := newTUIModel(suites)

	opts := []tea.ProgramOption{
		tea.WithOutput(w),
		tea.WithoutSignalHandler(),
		tea.WithAltScreen(), // Use alternate screen so animation doesn't pollute scrollback
	}

	// Only use input if we have a TTY
	if f, ok := w.(*os.File); !ok || !isatty.IsTerminal(f.Fd()) {
		// Non-TTY mode - disable input
		opts = append(opts, tea.WithInput(nil))
	}

	p := tea.NewProgram(model, opts...)

	return &TUIFormatter{
		program: p,
		model:   model,
	}
}

// Start begins the TUI event loop. Call this before running tests.
func (t *TUIFormatter) Start() error {
	go func() {
		_, _ = t.program.Run()
	}()

	// Give the program a moment to initialize
	time.Sleep(20 * time.Millisecond)

	return nil
}

// Format sends an event to the TUI.
func (t *TUIFormatter) Format(event Event, _ *Result) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.finished {
		return nil
	}

	t.program.Send(testEventMsg(event))

	return nil
}

// Summary waits for user to quit and renders final output.
func (t *TUIFormatter) Summary(result *Result) error {
	t.mu.Lock()
	t.finished = true
	t.mu.Unlock()

	// Send done signal - this shows the "press ESC to exit" message
	t.program.Send(doneMsg{result: result})

	// Wait for the program to exit (user pressed ESC/q)
	// The Run() goroutine will exit when user quits
	t.program.Wait()

	// Print the final static output. The TUI used the alternate screen,
	// so exiting it returns us to the main screen with clean scrollback.
	_, _ = io.WriteString(os.Stdout, t.model.FinalView()+"\n")

	return nil
}

// -----------------------------------------------------------------------------
// Tree Model - Built from Suite before tests run
// -----------------------------------------------------------------------------

// nodeKind identifies what type of tree node this is.
type nodeKind int

const (
	kindSuite nodeKind = iota
	kindScope
	kindGroup
	kindTest
)

// nodeStatus tracks the execution state of a node.
type nodeStatus int

const (
	statusPending nodeStatus = iota
	statusRunning
	statusPass
	statusFail
	statusSkip
	statusError
)

// treeNode represents a single node in the test tree.
type treeNode struct {
	name     string
	kind     nodeKind
	status   nodeStatus
	children []*treeNode
	parent   *treeNode

	// For leaf nodes (tests)
	elapsed time.Duration
	field   string
	expect  any
	actual  any
	err     error
}

// SuiteTree holds a parsed suite and its tree representation.
type SuiteTree struct {
	path string                // file path
	root *treeNode             // tree root for this suite
	idx  map[string]*treeNode  // "suite::path" -> node lookup
}

// BuildSuiteTree creates a tree representation from a parsed Suite.
func BuildSuiteTree(suite *scaf.Suite, suitePath string) SuiteTree {
	st := SuiteTree{
		path: suitePath,
		idx:  make(map[string]*treeNode),
	}

	// Root is the suite file
	st.root = &treeNode{
		name: suitePath,
		kind: kindSuite,
	}

	// Add query scopes
	for _, scope := range suite.Scopes {
		scopeNode := &treeNode{
			name:   scope.QueryName,
			kind:   kindScope,
			parent: st.root,
		}
		st.root.children = append(st.root.children, scopeNode)

		// Add items (tests and groups) - include suite path to avoid collisions
		addItems(scopeNode, scope.Items, []string{scope.QueryName}, suitePath, st.idx)
	}

	return st
}

func addItems(parent *treeNode, items []*scaf.TestOrGroup, pathPrefix []string, suitePath string, idx map[string]*treeNode) {
	for _, item := range items {
		if item.Test != nil {
			testNode := &treeNode{
				name:   item.Test.Name,
				kind:   kindTest,
				parent: parent,
			}
			parent.children = append(parent.children, testNode)

			// Index by "suite::path" to avoid collisions between files
			path := make([]string, len(pathPrefix)+1)
			copy(path, pathPrefix)
			path[len(pathPrefix)] = item.Test.Name
			key := suitePath + "::" + strings.Join(path, "/")
			idx[key] = testNode
		}

		if item.Group != nil {
			groupNode := &treeNode{
				name:   item.Group.Name,
				kind:   kindGroup,
				parent: parent,
			}
			parent.children = append(parent.children, groupNode)

			// Recurse into group (make a copy to avoid slice aliasing)
			groupPath := make([]string, len(pathPrefix)+1)
			copy(groupPath, pathPrefix)
			groupPath[len(pathPrefix)] = item.Group.Name
			addItems(groupNode, item.Group.Items, groupPath, suitePath, idx)
		}
	}
}

// -----------------------------------------------------------------------------
// Bubbletea Model
// -----------------------------------------------------------------------------

// tuiModel is the bubbletea model for the test runner UI.
type tuiModel struct {
	styles  *Styles
	spinner spinner.Model

	// State
	width  int
	height int

	// Test tree
	suites []SuiteTree
	allIdx map[string]*treeNode // combined index across all suites

	// Counters
	counters counters

	// Timing
	startTime time.Time
	endTime   time.Time

	// Final result
	finalResult *Result
	isDone      bool

	// Scrolling
	scrollOffset int
	totalLines   int

	// User quit
	userQuit bool
}

type counters struct {
	total   int
	passed  int
	failed  int
	skipped int
	errors  int
}

// Messages.
type (
	tickMsg      time.Time
	testEventMsg Event
	doneMsg      struct{ result *Result }
)

func newTUIModel(suites []SuiteTree) *tuiModel {
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: SpinnerFrames(),
		FPS:    time.Second / 10,
	}
	s.Style = DefaultStyles().Running

	// Build combined index
	allIdx := make(map[string]*treeNode)
	totalTests := 0

	for i := range suites {
		for path, node := range suites[i].idx {
			allIdx[path] = node
			totalTests++
		}
	}

	return &tuiModel{
		styles:    DefaultStyles(),
		spinner:   s,
		suites:    suites,
		allIdx:    allIdx,
		startTime: time.Now(),
		width:     80,
		height:    24,
		counters:  counters{total: totalTests},
	}
}

func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.tick(),
	)
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn // bubbletea.Model interface required by tea.Program
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.QuitMsg:
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// Always allow ctrl+c to quit
			m.userQuit = true

			return m, tea.Quit
		case "esc", "q":
			if m.isDone {
				m.userQuit = true

				return m, tea.Quit
			}
		case "j", "down":
			// Scroll down
			maxScroll := max(m.totalLines-m.viewportHeight(), 0)

			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
		case "k", "up":
			// Scroll up
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "g", "home":
			// Jump to top
			m.scrollOffset = 0
		case "G", "end":
			// Jump to bottom
			m.scrollOffset = max(m.totalLines-m.viewportHeight(), 0)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m, nil

	case tickMsg:
		if !m.isDone {
			cmds = append(cmds, m.tick())
		}

	case spinner.TickMsg:
		if !m.isDone {
			var cmd tea.Cmd

			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case testEventMsg:
		m.handleEvent(Event(msg))

	case doneMsg:
		m.isDone = true
		m.endTime = time.Now()
		m.finalResult = msg.result
	}

	return m, tea.Batch(cmds...)
}

func (m *tuiModel) tick() tea.Cmd { //nolint:funcorder
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

//nolint:funcorder // viewportHeight returns the number of lines available for content.
func (m *tuiModel) viewportHeight() int {
	// Reserve lines for header, spacing, and footer
	reserved := 5 // header + blank + summary + footer hint + blank

	return max(m.height-reserved, 1)
}

func (m *tuiModel) handleEvent(event Event) { //nolint:funcorder
	// Key format: "suite::path/to/test"
	key := event.Suite + "::" + event.PathString()
	node, ok := m.allIdx[key]

	if !ok {
		return // Unknown test path
	}

	switch event.Action {
	case ActionRun:
		node.status = statusRunning

	case ActionPass:
		node.status = statusPass
		node.elapsed = event.Elapsed
		m.counters.passed++

	case ActionFail:
		node.status = statusFail
		node.elapsed = event.Elapsed
		node.field = event.Field
		node.expect = event.Expected
		node.actual = event.Actual
		m.counters.failed++

	case ActionSkip:
		node.status = statusSkip
		node.elapsed = event.Elapsed
		m.counters.skipped++

	case ActionError:
		node.status = statusError
		node.elapsed = event.Elapsed
		node.err = event.Error
		m.counters.errors++

	case ActionOutput, ActionSetup:
		// These actions don't affect the test tree display
	}
}

// clearEOL is the ANSI escape sequence to clear from cursor to end of line.
const clearEOL = "\033[K"

// FinalView renders the complete final output for printing after the TUI exits.
// Unlike View(), this doesn't include clear-to-EOL sequences and uses a static
// checkmark instead of the spinner for any "running" items (shouldn't happen).
func (m *tuiModel) FinalView() string {
	var lines []string

	// Header
	lines = append(lines, m.renderHeader())
	lines = append(lines, "") // blank line

	// Test tree
	for _, st := range m.suites {
		treeLines := strings.Split(strings.TrimSuffix(m.renderTree(st), "\n"), "\n")
		lines = append(lines, treeLines...)
	}

	// Summary with progress
	lines = append(lines, "")
	lines = append(lines, m.renderSummaryWithProgress())

	return strings.Join(lines, "\n")
}

func (m *tuiModel) View() string {
	var headerLines []string

	var contentLines []string

	var footerLines []string

	// Header (fixed at top)
	headerLines = append(headerLines, m.renderHeader())
	headerLines = append(headerLines, "") // blank line

	// Test tree (scrollable content)
	for _, st := range m.suites {
		treeLines := strings.Split(strings.TrimSuffix(m.renderTree(st), "\n"), "\n")
		contentLines = append(contentLines, treeLines...)
	}

	// Track total lines for scroll calculation
	m.totalLines = len(contentLines)

	// Footer: summary with progress bar (always shown)
	footerLines = append(footerLines, "")
	footerLines = append(footerLines, m.renderSummaryWithProgress())

	if m.isDone {
		footerLines = append(footerLines, m.styles.Dim.Render("  Press ESC or q to exit"))
	}

	// Apply scrolling to content
	viewportH := m.viewportHeight()
	startIdx := m.scrollOffset
	endIdx := startIdx + viewportH

	if startIdx > len(contentLines) {
		startIdx = len(contentLines)
	}

	if endIdx > len(contentLines) {
		endIdx = len(contentLines)
	}

	if startIdx < 0 {
		startIdx = 0
	}

	visibleContent := contentLines[startIdx:endIdx]

	// Show scroll indicators if needed
	if m.scrollOffset > 0 {
		headerLines = append(headerLines, m.styles.Dim.Render("  ↑ more above"))
	}

	// Combine all lines
	lines := make([]string, 0, len(headerLines)+len(visibleContent)+5)
	lines = append(lines, headerLines...)
	lines = append(lines, visibleContent...)

	// Pad to fill viewport if content is shorter
	for len(visibleContent) < viewportH && len(contentLines) <= viewportH {
		lines = append(lines, "")
	}

	if endIdx < len(contentLines) {
		lines = append(lines, m.styles.Dim.Render("  ↓ more below"))
	}

	lines = append(lines, footerLines...)

	// Add clear-to-EOL to each line to prevent rendering artifacts
	for i := range lines {
		lines[i] += clearEOL
	}

	return strings.Join(lines, "\n") + "\n"
}

func (m *tuiModel) renderHeader() string {
	logo := m.styles.Bold.Render("scaf")
	subtitle := m.styles.Dim.Render(" test")

	var status string
	if m.isDone {
		if m.counters.failed > 0 || m.counters.errors > 0 {
			status = m.styles.Fail.Render("FAIL")
		} else {
			status = m.styles.Pass.Render("PASS")
		}
	} else {
		running := m.countRunning()
		if running > 0 {
			status = m.styles.Running.Render(fmt.Sprintf("running %d", running))
		} else {
			status = m.styles.Dim.Render("starting")
		}
	}

	return fmt.Sprintf("%s%s  %s", logo, subtitle, status)
}

func (m *tuiModel) countRunning() int {
	count := 0

	for _, node := range m.allIdx {
		if node.status == statusRunning {
			count++
		}
	}

	return count
}

func (m *tuiModel) renderTree(st SuiteTree) string {
	var b strings.Builder

	// Simple file header - just the path, dimmed
	b.WriteString(m.styles.Path.Render(st.path))
	b.WriteString("\n")

	// Render the tree
	for i, child := range st.root.children {
		isLast := i == len(st.root.children)-1
		m.renderNode(&b, child, "", isLast)
	}

	b.WriteString("\n")

	return b.String()
}

// computeGroupStatus calculates status for a group based on its children.
func (m *tuiModel) computeGroupStatus(node *treeNode) nodeStatus {
	if node.kind == kindTest {
		return node.status
	}

	hasRunning := false
	hasFailed := false
	hasPending := false
	allPassed := true

	for _, child := range node.children {
		childStatus := m.computeGroupStatus(child)
		switch childStatus {
		case statusRunning:
			hasRunning = true
			allPassed = false
		case statusFail, statusError:
			hasFailed = true
			allPassed = false
		case statusPending:
			hasPending = true
			allPassed = false
		case statusSkip:
			// Skip doesn't affect pass status
		case statusPass:
			// Good
		}
	}

	if hasRunning {
		return statusRunning
	}

	if hasFailed {
		return statusFail
	}

	if hasPending {
		return statusPending
	}

	if allPassed && len(node.children) > 0 {
		return statusPass
	}

	return statusPending
}

func (m *tuiModel) renderNode(b *strings.Builder, node *treeNode, prefix string, isLast bool) {
	// Tree branch character
	branch := "├─"
	if isLast {
		branch = "╰─"
	}

	// Status symbol (with group status inheritance)
	symbol := m.renderSymbol(node)

	// Name with appropriate styling
	name := node.name

	switch node.kind {
	case kindSuite:
		// Suite nodes are rendered as file headers, not in the tree
	case kindScope:
		name = m.styles.Bold.Render(name)
	case kindGroup:
		name = m.styles.Muted.Render(name)
	case kindTest:
		name = m.styles.TestName.Render(name)
	}

	// Duration (for completed tests only)
	dur := ""
	if node.kind == kindTest && node.status != statusPending && node.status != statusRunning {
		dur = m.styles.Dim.Render(fmt.Sprintf("  [%s]", formatDuration(node.elapsed)))
	}

	// Render the line
	b.WriteString(m.styles.Dim.Render(prefix + branch + " "))
	b.WriteString(symbol)
	b.WriteString(" ")
	b.WriteString(name)
	b.WriteString(dur)
	b.WriteString("\n")

	// Failure details (indented under the test)
	if node.status == statusFail && node.field != "" {
		detailPrefix := prefix
		if isLast {
			detailPrefix += "  "
		} else {
			detailPrefix += "│ "
		}

		detail := fmt.Sprintf("%s: expected %v, got %v", node.field, node.expect, node.actual)

		b.WriteString(m.styles.Dim.Render(detailPrefix + "   "))
		b.WriteString(m.styles.Fail.Render(detail))
		b.WriteString("\n")
	}

	if node.status == statusError && node.err != nil {
		detailPrefix := prefix
		if isLast {
			detailPrefix += "  "
		} else {
			detailPrefix += "│ "
		}

		b.WriteString(m.styles.Dim.Render(detailPrefix + "   "))
		b.WriteString(m.styles.Error.Render(node.err.Error()))
		b.WriteString("\n")
	}

	// Render children
	childPrefix := prefix
	if isLast {
		childPrefix += "  "
	} else {
		childPrefix += "│ "
	}

	for i, child := range node.children {
		childIsLast := i == len(node.children)-1
		m.renderNode(b, child, childPrefix, childIsLast)
	}
}

func (m *tuiModel) renderSymbol(node *treeNode) string {
	// For groups/scopes, compute status from children
	status := node.status
	if node.kind != kindTest {
		status = m.computeGroupStatus(node)
	}

	switch status {
	case statusPending:
		return m.styles.Dim.Render("⋯")
	case statusRunning:
		return m.spinner.View()
	case statusPass:
		return m.styles.Pass.Render(m.styles.SymbolPass)
	case statusFail:
		return m.styles.Fail.Render(m.styles.SymbolFail)
	case statusSkip:
		return m.styles.Skip.Render(m.styles.SymbolSkip)
	case statusError:
		return m.styles.Error.Render(m.styles.SymbolFail)
	default:
		return " "
	}
}

func (m *tuiModel) renderSummaryWithProgress() string {
	done := m.counters.passed + m.counters.failed + m.counters.skipped + m.counters.errors
	total := m.counters.total

	// Always show all stats to keep width constant
	sep := m.styles.Dim.Render(" │ ")

	passedStyle := m.styles.Dim
	if m.counters.passed > 0 {
		passedStyle = m.styles.Pass
	}

	failedStyle := m.styles.Dim
	if m.counters.failed > 0 {
		failedStyle = m.styles.Fail
	}

	skippedStyle := m.styles.Dim
	if m.counters.skipped > 0 {
		skippedStyle = m.styles.Skip
	}

	errorsStyle := m.styles.Dim
	if m.counters.errors > 0 {
		errorsStyle = m.styles.Error
	}

	summaryText := passedStyle.Render(fmt.Sprintf("%d passed", m.counters.passed)) + sep +
		failedStyle.Render(fmt.Sprintf("%d failed", m.counters.failed)) + sep +
		errorsStyle.Render(fmt.Sprintf("%d errors", m.counters.errors)) + " " +
		m.styles.Muted.Render(fmt.Sprintf("(%d total)", total))

	// Elapsed time
	elapsed := time.Since(m.startTime)
	if !m.endTime.IsZero() {
		elapsed = m.endTime.Sub(m.startTime)
	}

	elapsedStr := m.styles.Dim.Render(fmt.Sprintf("[%s]", formatDuration(elapsed)))

	// Fixed width progress bar
	barWidth := 20

	// Build progress bar
	pct := 0.0
	if total > 0 {
		pct = float64(done) / float64(total)
	}

	pct = min(pct, 1.0)

	filled := max(min(int(pct*float64(barWidth)), barWidth), 0)

	empty := barWidth - filled

	filledChar, emptyChar := ProgressChars()
	bar := m.styles.ProgressFilled.Render(strings.Repeat(filledChar, filled)) +
		m.styles.ProgressEmpty.Render(strings.Repeat(emptyChar, empty))

	// Skip skipped count to save space (less common)
	_ = skippedStyle

	return "  " + summaryText + " " + bar + " " + elapsedStr
}



// Helper functions

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return "<1ms"
	}

	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}

	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

// -----------------------------------------------------------------------------
// TUIHandler - Bridges TUI to Handler interface
// -----------------------------------------------------------------------------

// TUIHandler wraps TUIFormatter to implement Handler.
type TUIHandler struct {
	formatter *TUIFormatter
	stderr    io.Writer
}

// NewTUIHandler creates a handler that uses the TUI formatter.
// Call SetSuites before Start to initialize the tree view.
func NewTUIHandler(_ io.Writer, stderr io.Writer) *TUIHandler {
	return &TUIHandler{
		stderr: stderr,
	}
}

// SetSuites initializes the TUI with parsed suites for tree display.
func (h *TUIHandler) SetSuites(suites []SuiteTree) {
	h.formatter = NewTUIFormatter(os.Stdout, suites)
}

// Start initializes the TUI.
func (h *TUIHandler) Start() error {
	if h.formatter == nil {
		// Fallback: empty tree
		h.formatter = NewTUIFormatter(os.Stdout, nil)
	}

	return h.formatter.Start()
}

// Event sends an event to the TUI.
func (h *TUIHandler) Event(_ context.Context, event Event, result *Result) error {
	if h.formatter == nil {
		return nil
	}

	return h.formatter.Format(event, result)
}

// Err writes to stderr.
func (h *TUIHandler) Err(text string) error {
	_, err := h.stderr.Write([]byte(text + "\n"))

	return err
}

// Summary renders the final summary.
func (h *TUIHandler) Summary(result *Result) error {
	if h.formatter == nil {
		return nil
	}

	return h.formatter.Summary(result)
}