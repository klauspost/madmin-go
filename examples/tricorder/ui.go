package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/minio/madmin-go/v4"
)

// TricorderModel represents the main TUI model
type TricorderModel struct {
	nav           *NavigationState
	renderer      *Renderer
	config        Config
	width         int
	height        int
	quitting      bool
	lastPath      string    // Track the current path to detect navigation changes
	scrollOffset  int       // Current scroll position in content
	menuScrollTop int       // Top index of visible menu items
	lastScroll    int       // Track scroll changes
	lastSelection int       // Track selection changes
	lastEscTime   time.Time // Track last Esc press for double-Esc exit
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
		nav:           nav,
		renderer:      renderer,
		config:        config,
		quitting:      false,
		lastPath:      nav.GetCurrentPath(),
		scrollOffset:  0,
		menuScrollTop: 0,
		lastScroll:    0,
		lastSelection: nav.GetSelectedIndex(),
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

		// Use actual terminal dimensions for full screen
		if m.height <= 0 {
			m.height = 24 // Fallback if size detection fails
		}
		if m.width <= 0 {
			m.width = 80 // Fallback if size detection fails
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
			// Refresh completed successfully
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
			// Navigate through menu items with auto-scrolling
			oldSelection := m.nav.GetSelectedIndex()
			m.nav.MoveSelection(-1)
			newSelection := m.nav.GetSelectedIndex()

			// Auto-scroll menu if needed
			m.adjustMenuScroll(newSelection, oldSelection)
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
			// Navigate through menu items with auto-scrolling
			oldSelection := m.nav.GetSelectedIndex()
			m.nav.MoveSelection(1)
			newSelection := m.nav.GetSelectedIndex()

			// Auto-scroll menu if needed
			m.adjustMenuScroll(newSelection, oldSelection)
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

// adjustMenuScroll adjusts the menu viewport to keep selection visible
func (m *TricorderModel) adjustMenuScroll(newSelection, oldSelection int) {
	// Only adjust if selection actually changed
	if newSelection == oldSelection {
		return
	}

	// Calculate available height for menu (conservative estimate)
	// Leave room for header, separator, help, and some data
	maxMenuHeight := m.height - 8
	if maxMenuHeight < 5 {
		maxMenuHeight = 5
	}

	// Calculate total menu items
	children := m.nav.GetChildren()
	totalItems := len(children)
	if m.nav.CanNavigateBack() {
		totalItems++ // Account for .. entry
	}

	// If all items fit, no scrolling needed
	if totalItems <= maxMenuHeight {
		m.menuScrollTop = 0
		return
	}

	// Adjust scroll to keep selected item visible
	if newSelection < m.menuScrollTop {
		// Selection moved above visible area, scroll up
		m.menuScrollTop = newSelection
	} else if newSelection >= m.menuScrollTop+maxMenuHeight {
		// Selection moved below visible area, scroll down
		m.menuScrollTop = newSelection - maxMenuHeight + 1
	}

	// Ensure scroll bounds
	if m.menuScrollTop < 0 {
		m.menuScrollTop = 0
	}
	maxScroll := totalItems - maxMenuHeight
	if m.menuScrollTop > maxScroll {
		m.menuScrollTop = maxScroll
	}
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

	// Check for changes that require screen clearing
	currentPath := m.nav.GetCurrentPath()
	currentSelection := m.nav.GetSelectedIndex()

	// Update tracking variables
	if currentPath != m.lastPath {
		m.lastPath = currentPath
		m.scrollOffset = 0  // Reset scroll on navigation
		m.menuScrollTop = 0 // Reset menu scroll on navigation
	}
	m.lastScroll = m.scrollOffset
	m.lastSelection = currentSelection

	// Simple approach: build content and always show help at bottom
	var parts []string

	// Header
	header := m.renderer.RenderHeader(m.nav)
	parts = append(parts, header)

	// Error if any
	if errorMsg := m.renderer.RenderError(m.nav); errorMsg != "" {
		parts = append(parts, errorMsg)
	}

	// Main content with menu scrolling
	maxMenuHeight := m.height - 8 // Conservative estimate for available menu space
	if maxMenuHeight < 5 {
		maxMenuHeight = 5
	}
	mainContent := m.renderer.RenderContentWithScroll(m.nav, m.menuScrollTop, maxMenuHeight)
	parts = append(parts, mainContent)

	// Join all parts
	content := strings.Join(parts, "\n")
	contentLines := strings.Split(content, "\n")

	// Determine available space for content (leave 2 lines for separator + help + buffer)
	maxContentLines := m.height - 2
	if maxContentLines < 10 {
		maxContentLines = 10 // minimum
	}

	// Apply scrolling if needed
	if len(contentLines) > maxContentLines {
		// Scrolling is handled - path change was already processed above

		// Scrolling bounds
		maxScroll := len(contentLines) - maxContentLines
		if m.scrollOffset > maxScroll {
			m.scrollOffset = maxScroll
		}
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}

		// Get visible lines
		startIdx := m.scrollOffset
		endIdx := startIdx + maxContentLines
		if endIdx > len(contentLines) {
			endIdx = len(contentLines)
		}
		contentLines = contentLines[startIdx:endIdx]
	}

	// Build final output
	for _, line := range contentLines {
		output.WriteString(line)
		output.WriteString("\n")
	}

	// Add separator (only at bottom before help)
	if m.width > 0 {
		output.WriteString(strings.Repeat("─", m.width))
		output.WriteString("\n")
	}

	// Add help
	help := m.renderer.RenderHelp()
	output.WriteString(help)

	return lipgloss.NewStyle().MaxWidth(m.width).Height(m.height).Render(output.String())
}
