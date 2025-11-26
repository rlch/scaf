package runner

import "github.com/charmbracelet/lipgloss"

// Semantic colors following nextest/vitest conventions.
var (
	// Status colors.
	colorPass    = lipgloss.Color("#10b981") // green-500
	colorFail    = lipgloss.Color("#ef4444") // red-500
	colorSkip    = lipgloss.Color("#eab308") // yellow-500
	colorRunning = lipgloss.Color("#06b6d4") // cyan-500
	colorSlow    = lipgloss.Color("#f59e0b") // amber-500
	colorRetry   = lipgloss.Color("#d946ef") // fuchsia-500

	// UI colors.
	colorDim    = lipgloss.Color("#6b7280") // gray-500
	colorMuted  = lipgloss.Color("#9ca3af") // gray-400
	colorBorder = lipgloss.Color("#374151") // gray-700
	colorAccent = lipgloss.Color("#3b82f6") // blue-500
)

// Styles holds all lipgloss styles for the TUI.
type Styles struct {
	// Status badges
	Pass    lipgloss.Style
	Fail    lipgloss.Style
	Skip    lipgloss.Style
	Running lipgloss.Style
	Slow    lipgloss.Style
	Retry   lipgloss.Style
	Error   lipgloss.Style

	// Text styles
	Dim      lipgloss.Style
	Muted    lipgloss.Style
	Bold     lipgloss.Style
	TestName lipgloss.Style
	Duration lipgloss.Style
	Path     lipgloss.Style

	// Symbols
	SymbolPass    string
	SymbolFail    string
	SymbolSkip    string
	SymbolRunning string
	SymbolSlow    string
	SymbolPointer string

	// Tree characters
	TreeMiddle string
	TreeEnd    string
	TreeBar    string

	// Progress bar
	ProgressFilled lipgloss.Style
	ProgressEmpty  lipgloss.Style

	// Layout
	StatusWidth int
}

// DefaultStyles returns the default TUI styles.
func DefaultStyles() *Styles {
	return &Styles{
		// Status badges - bold colored text
		Pass:    lipgloss.NewStyle().Foreground(colorPass).Bold(true),
		Fail:    lipgloss.NewStyle().Foreground(colorFail).Bold(true),
		Skip:    lipgloss.NewStyle().Foreground(colorSkip).Bold(true),
		Running: lipgloss.NewStyle().Foreground(colorRunning).Bold(true),
		Slow:    lipgloss.NewStyle().Foreground(colorSlow).Bold(true),
		Retry:   lipgloss.NewStyle().Foreground(colorRetry).Bold(true),
		Error:   lipgloss.NewStyle().Foreground(colorFail).Bold(true),

		// Text styles
		Dim:      lipgloss.NewStyle().Foreground(colorDim),
		Muted:    lipgloss.NewStyle().Foreground(colorMuted),
		Bold:     lipgloss.NewStyle().Bold(true),
		TestName: lipgloss.NewStyle().Foreground(lipgloss.Color("#f8fafc")), // slate-50
		Duration: lipgloss.NewStyle().Foreground(colorDim),
		Path:     lipgloss.NewStyle().Foreground(colorAccent),

		// Unicode symbols
		SymbolPass:    "✓",
		SymbolFail:    "✗",
		SymbolSkip:    "↓",
		SymbolRunning: "◐",
		SymbolSlow:    "⏱",
		SymbolPointer: "❯",

		// Tree characters for nested output (rounded style)
		TreeMiddle: "├─",
		TreeEnd:    "╰─",
		TreeBar:    "│ ",

		// Progress bar characters
		ProgressFilled: lipgloss.NewStyle().Foreground(colorAccent),
		ProgressEmpty:  lipgloss.NewStyle().Foreground(colorBorder),

		// Fixed width for status alignment (like nextest's 12)
		StatusWidth: 8,
	}
}

// SpinnerFrames returns the braille spinner animation frames.
func SpinnerFrames() []string {
	return []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
}

// ProgressChars returns the progress bar characters.
func ProgressChars() (string, string) {
	return "█", "░"
}
