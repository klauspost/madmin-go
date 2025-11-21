package main

import (
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
	lastPath     string    // Track the current path to detect navigation changes
	needsClear   bool      // Flag to indicate when we need to clear screen
	scrollOffset int       // Current scroll position in content
	lastEscTime  time.Time // Track last Esc press for double-Esc exit
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

		// Sanity check for reasonable terminal dimensions
		// Terminal scrollback can be huge, but visible area is typically 20-60 lines
		if m.height > 60 || m.height <= 0 {
			m.height = 30 // Use reasonable default for visible area
		}
		if m.width > 200 || m.width <= 0 {
			m.width = 100 // Use reasonable default
		}

		m.renderer.SetSize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case refreshMsg:
		// Auto-refresh triggered
		if !m.nav.IsRefreshing() && !m.nav.ShouldPauseRefresh() {
			return m, m.doRefresh(false) // false = auto refresh
		}
		// If already refreshing or paused, just set up next timer
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

	case "f5":
		// Manual refresh
		if !m.nav.IsRefreshing() {
			return m, m.doRefresh(true) // true = manual refresh
		}
		return m, nil
	}

	// Handle navigation vs scrolling keys based on context
	children := m.nav.GetChildren()
	hasChildren := len(children) > 0 && !m.nav.IsLeaf()

	// Allow scrolling when there's more content than visible space
	// Get content to check if scrolling is needed
	mainContent := m.renderer.RenderContent(m.nav)
	contentLines := strings.Split(strings.TrimSpace(mainContent), "\n")

	// Use a simple height estimate for this check
	estimatedAvailableHeight := 15 // Conservative estimate
	needsScrolling := len(contentLines) > estimatedAvailableHeight

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
			// Just increment scroll and let the main View() method handle bounds
			// This avoids duplicate calculation inconsistencies
			m.scrollOffset++
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
						// Debug: Navigation should reset scroll to 0
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
			// Page up - just subtract a reasonable amount and let View() handle bounds
			m.scrollOffset -= 5
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
			return m, nil
		}

	case "pgdown", "page_down":
		if needsScrolling {
			// Page down - just add a reasonable amount and let View() handle bounds
			m.scrollOffset += 5
			return m, nil
		}

	case "home":
		if hasChildren {
			// Go to first item (.. if available, otherwise first child)
			m.nav.SetSelectedIndex(0)
			return m, nil
		}

	case "end":
		if hasChildren {
			// Go to last item
			children := m.nav.GetChildren()
			totalOptions := len(children)
			if m.nav.CanNavigateBack() {
				totalOptions++ // Account for .. entry
			}
			m.nav.SetSelectedIndex(totalOptions - 1)
			return m, nil
		}
	}

	// Handle back navigation
	switch msg.String() {
	case "esc", "backspace":
		// Check for double-Esc exit on root page
		if !m.nav.CanNavigateBack() {
			// We're at root - check for double-Esc within 1 second
			now := time.Now()
			if !m.lastEscTime.IsZero() && now.Sub(m.lastEscTime) < time.Second {
				// Double-Esc within 1 second - exit
				m.quitting = true
				return m, tea.Quit
			}
			m.lastEscTime = now
		} else {
			// Normal back navigation
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

	// Handle letter navigation - jump to next entry starting with that letter
	if len(msg.String()) == 1 && hasChildren {
		letter := strings.ToLower(msg.String())
		children := m.nav.GetChildren()
		selectedIndex := m.nav.GetSelectedIndex()

		// Start search from the item after current selection
		startIndex := selectedIndex + 1
		if m.nav.CanNavigateBack() {
			startIndex-- // Account for .. entry
		}

		// Search forward from current position
		for i := 0; i < len(children); i++ {
			checkIndex := (startIndex + i) % len(children)
			childName := strings.ToLower(children[checkIndex].Name)
			if strings.HasPrefix(childName, letter) {
				// Found match - set selection (account for .. entry)
				newSelection := checkIndex
				if m.nav.CanNavigateBack() {
					newSelection++ // Account for .. entry at index 0
				}
				m.nav.SetSelectedIndex(newSelection)
				return m, nil
			}
		}
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
	if height <= 0 || height > 60 {
		height = 30 // Default visible terminal height
	}

	// Calculate available space for main content more conservatively
	// Account for: headers, errors, help, separators, spacing, and some buffer
	reservedLines := len(headerLines) + len(errorLines) + len(helpLines) + 5 // +5 for separators, spacing, and buffer
	availableHeight := height - reservedLines

	// Ensure minimum but be less conservative now that scrolling works
	if availableHeight < 8 {
		availableHeight = 8
	}

	// Get main content and handle scrolling
	mainContent := m.renderer.RenderContent(m.nav)
	contentLines := strings.Split(strings.TrimSpace(mainContent), "\n")

	// Force scroll reset if we're at a different path than expected
	currentPath := m.nav.GetCurrentPath()
	if currentPath != m.lastPath {
		m.scrollOffset = 0 // Reset scroll when path changes
		m.lastPath = currentPath
		// Don't clear screen here - only on explicit navigation
	}

	// Calculate scroll bounds - how far we can scroll down
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

	// Apply scrolling - simpler approach
	var displayLines []string

	if len(contentLines) > availableHeight {
		// Need scrolling
		visibleLines := availableHeight
		startIdx := m.scrollOffset
		endIdx := startIdx + visibleLines

		// Ensure bounds are correct
		if startIdx < 0 {
			startIdx = 0
		}
		if startIdx >= len(contentLines) {
			startIdx = len(contentLines) - visibleLines
			if startIdx < 0 {
				startIdx = 0
			}
		}
		if endIdx > len(contentLines) {
			endIdx = len(contentLines)
		}

		displayLines = append([]string{}, contentLines[startIdx:endIdx]...)
	} else {
		// All content fits
		displayLines = append([]string{}, contentLines...)
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

	// Separator and help
	if m.width > 0 {
		output.WriteString(strings.Repeat("─", m.width))
		output.WriteString("\n")
	}
	output.WriteString(help)

	return output.String()
}

