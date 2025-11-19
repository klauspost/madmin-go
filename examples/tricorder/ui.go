package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/minio/madmin-go/v4"
)

// TricorderModel represents the main TUI model
type TricorderModel struct {
	nav          *NavigationState
	renderer     *Renderer
	config       Config
	width        int
	height       int
	quitting     bool
	lastPath     string // Track the current path to detect navigation changes
	needsClear   bool   // Flag to indicate when we need to clear screen
	scrollOffset int    // Current scroll position in content
}

// refreshMsg is sent when auto-refresh timer fires
type refreshMsg struct{}

// refreshCompleteMsg is sent when refresh operation completes
type refreshCompleteMsg struct {
	err    error
	manual bool // true if this was a manual refresh (should clear screen)
}

// NewTricorderModel creates a new TUI model
func NewTricorderModel(adminClient *madmin.AdminClient, metrics *madmin.RealtimeMetrics, config Config) *TricorderModel {
	nav := NewNavigationState(adminClient, metrics, config)
	renderer := NewRenderer(80, 24) // Default size

	return &TricorderModel{
		nav:          nav,
		renderer:     renderer,
		config:       config,
		quitting:     false,
		lastPath:     nav.GetCurrentPath(),
		needsClear:   true, // Clear on initial render
		scrollOffset: 0,    // Start at top
	}
}

// Init implements tea.Model
func (m *TricorderModel) Init() tea.Cmd {
	// Do initial refresh and start auto-refresh timer
	return tea.Batch(
		m.doRefresh(false), // false = auto refresh for initial load
		tea.Tick(m.config.RefreshPeriod, func(t time.Time) tea.Msg {
			return refreshMsg{}
		}),
	)
}

// Update implements tea.Model
func (m *TricorderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.renderer.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case refreshMsg:
		// Auto-refresh triggered
		if !m.nav.IsRefreshing() {
			return m, m.doRefresh(false) // false = auto refresh
		}
		// If already refreshing, just set up next timer
		return m, tea.Tick(m.config.RefreshPeriod, func(t time.Time) tea.Msg {
			return refreshMsg{}
		})

	case refreshCompleteMsg:
		// Refresh completed
		var nextCmd tea.Cmd
		if msg.err == nil {
			// Clear any previous errors
			m.nav.ClearError()
			// Only clear screen for manual refresh, not auto-refresh
			if msg.manual {
				m.needsClear = true
			}
		}
		// Schedule next auto-refresh
		nextCmd = tea.Tick(m.config.RefreshPeriod, func(t time.Time) tea.Msg {
			return refreshMsg{}
		})
		return m, nextCmd

	default:
		return m, nil
	}
}

// handleKeyPress processes keyboard input
func (m *TricorderModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle global keys first
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit

	case "r":
		// Manual refresh
		if !m.nav.IsRefreshing() {
			return m, m.doRefresh(true) // true = manual refresh
		}
		return m, nil
	}

	// Handle navigation vs scrolling keys based on context
	children := m.nav.GetChildren()
	hasChildren := len(children) > 0 && !m.nav.IsLeaf()

	// Check if we're in a context where scrolling makes sense
	mainContent := m.renderer.RenderContent(m.nav)
	contentLines := strings.Split(strings.TrimSpace(mainContent), "\n")
	height := m.height
	if height <= 0 {
		height = 24
	}
	availableHeight := height - 8 // Rough estimate for header/footer space
	needsScrolling := len(contentLines) > availableHeight

	switch msg.String() {
	case "up", "k":
		if hasChildren {
			// Navigate through menu items
			m.nav.MoveSelection(-1)
			return m, nil
		} else if needsScrolling {
			// Scroll content up
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
			return m, nil
		}

	case "down", "j":
		if hasChildren {
			// Navigate through menu items
			m.nav.MoveSelection(1)
			return m, nil
		} else if needsScrolling {
			// Scroll content down
			maxScroll := len(contentLines) - availableHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
			return m, nil
		}

	case "enter", " ":
		if hasChildren {
			selectedIndex := m.nav.GetSelectedIndex()

			// Check if ".." back navigation is selected
			if m.nav.CanNavigateBack() && selectedIndex == 0 {
				// Navigate back using ..
				oldPath := m.nav.GetCurrentPath()
				err := m.nav.NavigateBack()
				if err != nil {
					// Error is stored in navigation state
				} else {
					// Check if navigation actually changed
					newPath := m.nav.GetCurrentPath()
					if newPath != oldPath {
						m.needsClear = true
						m.scrollOffset = 0 // Reset scroll on navigation
					}
				}
			} else {
				// Regular navigation into child
				oldPath := m.nav.GetCurrentPath()
				err := m.nav.NavigateInto()
				if err != nil {
					// Error is stored in navigation state
				} else {
					// Check if navigation actually changed
					newPath := m.nav.GetCurrentPath()
					if newPath != oldPath {
						m.needsClear = true
						m.scrollOffset = 0 // Reset scroll on navigation
					}
				}
			}
			return m, nil
		} else if m.nav.CanNavigateBack() && m.nav.GetSelectedIndex() == 0 {
			// Handle ".." navigation on leaf pages (no children)
			oldPath := m.nav.GetCurrentPath()
			err := m.nav.NavigateBack()
			if err != nil {
				// Error is stored in navigation state
			} else {
				// Check if navigation actually changed
				newPath := m.nav.GetCurrentPath()
				if newPath != oldPath {
					m.needsClear = true
					m.scrollOffset = 0 // Reset scroll on navigation
				}
			}
			return m, nil
		}

	case "pgup", "page_up":
		if needsScrolling {
			// Page up - scroll by half the available height
			pageSize := availableHeight / 2
			if pageSize < 1 {
				pageSize = 1
			}
			m.scrollOffset -= pageSize
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
			return m, nil
		}

	case "pgdown", "page_down":
		if needsScrolling {
			// Page down - scroll by half the available height
			pageSize := availableHeight / 2
			if pageSize < 1 {
				pageSize = 1
			}
			maxScroll := len(contentLines) - availableHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scrollOffset += pageSize
			if m.scrollOffset > maxScroll {
				m.scrollOffset = maxScroll
			}
			return m, nil
		}
	}

	// Handle back navigation
	switch msg.String() {
	case "esc", "backspace":
		if m.nav.CanNavigateBack() {
			oldPath := m.nav.GetCurrentPath()
			err := m.nav.NavigateBack()
			if err != nil {
				// Error is stored in navigation state
			} else {
				// Check if navigation actually changed
				newPath := m.nav.GetCurrentPath()
				if newPath != oldPath {
					m.needsClear = true
					m.scrollOffset = 0 // Reset scroll on navigation
				}
			}
		}
		return m, nil
	}

	return m, nil
}

// doRefresh performs a refresh operation
func (m *TricorderModel) doRefresh(manual bool) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		err := m.nav.Refresh()
		return refreshCompleteMsg{err: err, manual: manual}
	})
}

// View implements tea.Model
func (m *TricorderModel) View() string {
	if m.quitting {
		return "Goodbye! 👋"
	}

	var output strings.Builder

	// Only clear screen when we need to (navigation change, refresh, etc.)
	if m.needsClear {
		output.WriteString("\033[2J\033[H") // Clear screen and move cursor to top
		m.needsClear = false // Reset the flag
	}

	// Build all sections first to measure content
	header := m.renderer.RenderHeader(m.nav)
	headerLines := strings.Split(header, "\n")

	var errorLines []string
	if errorMsg := m.renderer.RenderError(m.nav); errorMsg != "" {
		errorLines = strings.Split(strings.TrimSpace(errorMsg), "\n")
	}

	help := m.renderer.RenderHelp()
	helpLines := strings.Split(help, "\n")

	// Ensure we have valid height (fallback to reasonable default)
	height := m.height
	if height <= 0 {
		height = 24 // Default terminal height
	}

	// Calculate available space for main content
	reservedLines := len(headerLines) + len(errorLines) + len(helpLines) + 3 // +3 for separators and spacing
	availableHeight := height - reservedLines
	if availableHeight < 5 { // Minimum content area
		availableHeight = 5
	}

	// Get main content and handle scrolling
	mainContent := m.renderer.RenderContent(m.nav)
	contentLines := strings.Split(strings.TrimSpace(mainContent), "\n")

	// Calculate scroll bounds
	maxScroll := len(contentLines) - availableHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Ensure scroll offset is within bounds
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	// Apply scrolling
	var displayLines []string
	if len(contentLines) > availableHeight {
		// Show scrolled portion
		endIndex := m.scrollOffset + availableHeight - 1 // -1 for scroll indicator
		if endIndex >= len(contentLines) {
			endIndex = len(contentLines) - 1
		}

		displayLines = contentLines[m.scrollOffset:endIndex+1]

		// Add scroll indicators
		scrollInfo := ""
		if m.scrollOffset > 0 && m.scrollOffset < maxScroll {
			scrollInfo = fmt.Sprintf("↑↓ Scroll %d/%d", m.scrollOffset+availableHeight-1, len(contentLines))
		} else if m.scrollOffset > 0 {
			scrollInfo = fmt.Sprintf("↑ Scroll (at bottom) %d/%d", len(contentLines), len(contentLines))
		} else if maxScroll > 0 {
			scrollInfo = fmt.Sprintf("↓ Scroll (at top) %d/%d", availableHeight-1, len(contentLines))
		}

		if scrollInfo != "" {
			displayLines = append(displayLines, scrollInfo)
		}
	} else {
		displayLines = contentLines
	}

	// Assemble final output
	output.WriteString(header)
	output.WriteString("\n")

	if len(errorLines) > 0 {
		for _, line := range errorLines {
			output.WriteString(line)
			output.WriteString("\n")
		}
		output.WriteString("\n")
	}

	for _, line := range displayLines {
		output.WriteString(line)
		output.WriteString("\n")
	}
	output.WriteString("\n")

	// Separator and help
	if m.width > 0 {
		output.WriteString(strings.Repeat("─", m.width))
		output.WriteString("\n")
	}
	output.WriteString(help)

	return output.String()
}

