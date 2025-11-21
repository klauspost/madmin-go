package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/minio/madmin-go/v4"
)

// NavigationState manages the current state of navigation through metrics
type NavigationState struct {
	adminClient      *madmin.AdminClient
	config           Config
	navigator        madmin.MetricNavigator
	currentNode      madmin.MetricNode
	currentPath      string
	pathHistory      []string // Navigation history for back functionality
	selectedIndex    int      // Currently selected child index
	lastSelectedChild string   // Name of child we navigated into (for smart back navigation)
	lastRefresh      time.Time
	refreshing       bool
	errorMessage     string
}

// NewNavigationState creates a new navigation state
func NewNavigationState(adminClient *madmin.AdminClient, metrics *madmin.RealtimeMetrics, config Config) *NavigationState {
	nav := &NavigationState{
		adminClient:   adminClient,
		config:        config,
		currentPath:   "/",
		pathHistory:   []string{},
		selectedIndex: 0,
		refreshing:    false,
	}

	// If initial metrics provided, use them
	if metrics != nil {
		navigator := madmin.NewRealtimeMetricsNavigator(metrics)
		root := navigator.Root()
		nav.navigator = navigator
		nav.currentNode = root
		nav.lastRefresh = time.Now()
	}

	return nav
}

// GetCurrentPath returns the current navigation path
func (ns *NavigationState) GetCurrentPath() string {
	return ns.currentPath
}
func (ns *NavigationState) ShouldPauseRefresh() bool {
	// Check if any node in the current path requires pausing refresh
	if ns.currentNode != nil && ns.currentNode.ShouldPauseRefresh() {
		return true
	}

	// Traverse up the path to check parent nodes
	// We need to check all nodes in the current navigation path
	currentPath := ns.currentPath
	if currentPath == "" || currentPath == "/" {
		return false
	}

	// Split the path into segments and check each parent path
	pathParts := strings.Split(strings.Trim(currentPath, "/"), "/")
	currentTestPath := "/"

	for i := 0; i < len(pathParts); i++ {
		if pathParts[i] == "" {
			continue
		}

		if currentTestPath == "/" {
			currentTestPath = "/" + pathParts[i]
		} else {
			currentTestPath = currentTestPath + "/" + pathParts[i]
		}

		// Navigate to this path and check if it should pause refresh
		if ns.navigator != nil {
			if node, err := ns.navigator.Navigate(currentTestPath); err == nil {
				if node.ShouldPauseRefresh() {
					return true
				}
			}
		}
	}

	return false
}

// GetChildren returns the available children of the current node
func (ns *NavigationState) GetChildren() []madmin.MetricChild {
	if ns.currentNode == nil {
		return []madmin.MetricChild{}
	}
	return ns.currentNode.GetChildren()
}

// GetLeafData returns leaf data if the current node is a leaf
func (ns *NavigationState) GetLeafData() map[string]string {
	if ns.currentNode == nil {
		return map[string]string{}
	}
	return ns.currentNode.GetLeafData()
}

// IsLeaf returns true if current node is a leaf node
func (ns *NavigationState) IsLeaf() bool {
	if ns.currentNode == nil {
		return false
	}
	// A node is a leaf if it has no children
	children := ns.currentNode.GetChildren()
	return len(children) == 0
}

// GetSelectedIndex returns the currently selected child index
func (ns *NavigationState) GetSelectedIndex() int {
	return ns.selectedIndex
}

// SetSelectedIndex sets the selected child index
func (ns *NavigationState) SetSelectedIndex(index int) {
	children := ns.GetChildren()

	// Calculate total options including .. entry
	totalOptions := len(children)
	if ns.CanNavigateBack() {
		totalOptions++
	}

	if index < 0 {
		ns.selectedIndex = 0
	} else if index >= totalOptions {
		ns.selectedIndex = totalOptions - 1
	} else {
		ns.selectedIndex = index
	}
}

// MoveSelection moves the selection up or down
func (ns *NavigationState) MoveSelection(delta int) {
	ns.SetSelectedIndex(ns.selectedIndex + delta)
}

// NavigateInto navigates into the currently selected child
func (ns *NavigationState) NavigateInto() error {
	children := ns.GetChildren()

	// Adjust for .. entry if present
	childIndex := ns.selectedIndex
	if ns.CanNavigateBack() {
		childIndex-- // Account for .. entry at index 0
	}

	if len(children) == 0 || childIndex < 0 || childIndex >= len(children) {
		return fmt.Errorf("no children to navigate into")
	}

	selectedChild := children[childIndex]

	// Get the child node
	childNode, err := ns.currentNode.GetChild(selectedChild.Name)
	if err != nil {
		ns.errorMessage = fmt.Sprintf("Navigation error: %v", err)
		return err
	}

	// Check if new node requires different flags than what we have
	currentFlags := ns.getCurrentMetricFlags()
	newFlags := childNode.GetMetricFlags()
	needsRefresh := (newFlags != 0) && (currentFlags&newFlags != newFlags)

	// Update navigation state
	ns.pathHistory = append(ns.pathHistory, ns.currentPath)
	ns.lastSelectedChild = selectedChild.Name // Track the child we're navigating into
	ns.currentNode = childNode
	ns.currentPath = childNode.GetPath()
	// Set cursor to first actual item (skip .. entry since we now have history)
	if len(childNode.GetChildren()) > 0 {
		ns.selectedIndex = 1 // Skip .. entry, start on first real item
	} else {
		ns.selectedIndex = 0
	}
	ns.errorMessage = ""

	// Trigger refresh if needed for new flags
	if needsRefresh && !ns.refreshing {
		go func() {
			ns.Refresh()
		}()
	}

	return nil
}

// NavigateBack goes back to the parent node
func (ns *NavigationState) NavigateBack() error {
	if len(ns.pathHistory) == 0 {
		return fmt.Errorf("already at root")
	}

	// Get parent from history
	previousPath := ns.pathHistory[len(ns.pathHistory)-1]
	ns.pathHistory = ns.pathHistory[:len(ns.pathHistory)-1]

	// Navigate to parent
	parentNode, err := ns.navigator.Navigate(previousPath)
	if err != nil {
		ns.errorMessage = fmt.Sprintf("Back navigation error: %v", err)
		return err
	}

	ns.currentNode = parentNode
	ns.currentPath = previousPath

	// Smart cursor positioning - try to position cursor on the item we just returned from
	targetIndex := 0
	if ns.lastSelectedChild != "" && len(parentNode.GetChildren()) > 0 {
		// Find the child we came from
		children := parentNode.GetChildren()
		for i, child := range children {
			if child.Name == ns.lastSelectedChild {
				targetIndex = i
				if ns.CanNavigateBack() {
					targetIndex++ // Account for .. entry at index 0
				}
				break
			}
		}
	}

	// Fallback if not found or no lastSelectedChild
	if targetIndex == 0 {
		if len(parentNode.GetChildren()) > 0 && len(ns.pathHistory) > 0 {
			targetIndex = 1 // Skip .. entry, start on first real item
		}
	}

	ns.selectedIndex = targetIndex
	ns.lastSelectedChild = "" // Clear after use
	ns.errorMessage = ""

	return nil
}

// NavigateToPath navigates directly to a specific path
func (ns *NavigationState) NavigateToPath(path string) error {
	node, err := ns.navigator.Navigate(path)
	if err != nil {
		ns.errorMessage = fmt.Sprintf("Path navigation error: %v", err)
		return err
	}

	// Add current path to history if different
	if ns.currentPath != path {
		ns.pathHistory = append(ns.pathHistory, ns.currentPath)
	}

	ns.currentNode = node
	ns.currentPath = path
	ns.selectedIndex = 0
	ns.errorMessage = ""

	return nil
}

// Refresh reloads the metrics and updates the navigator
func (ns *NavigationState) Refresh() error {
	ns.refreshing = true
	ns.errorMessage = ""

	// Get fresh metrics using a short context to get just one sample
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := getMetricOptions(ns.config)

	// Add current node's required metric flags
	if ns.currentNode != nil {
		opts.Flags |= ns.currentNode.GetMetricFlags()
	}
	var metrics madmin.RealtimeMetrics
	var gotMetrics bool

	// Use a channel to get the first metrics sample and then cancel
	done := make(chan error, 1)

	go func() {
		err := ns.adminClient.Metrics(ctx, opts, func(m madmin.RealtimeMetrics) {
			if !gotMetrics {
				metrics = m
				gotMetrics = true
				cancel() // Cancel after getting first sample
			}
		})
		done <- err
	}()

	// Wait for either metrics or timeout
	select {
	case err := <-done:
		if err != nil && !gotMetrics {
			ns.refreshing = false
			ns.errorMessage = fmt.Sprintf("Refresh error: %v", err)
			return err
		}
	case <-ctx.Done():
		if !gotMetrics {
			ns.refreshing = false
			ns.errorMessage = "Refresh timeout: no metrics received"
			return fmt.Errorf("timeout waiting for metrics")
		}
	}

	// Create new navigator
	ns.navigator = madmin.NewRealtimeMetricsNavigator(&metrics)

	// Try to navigate back to current path
	currentPath := ns.currentPath
	if currentPath == "" {
		currentPath = "/"
	}

	node, err := ns.navigator.Navigate(currentPath)
	if err != nil {
		// If current path no longer exists, go to root
		node = ns.navigator.Root()
		ns.currentPath = "/"
		ns.pathHistory = []string{}
		ns.selectedIndex = 0 // Reset selection when path changes
	} else {
		// Try to preserve selection by name, then by index
		children := node.GetChildren()
		if len(children) > 0 {
			// Get the name of the previously selected item
			oldChildren := []madmin.MetricChild{}
			if ns.currentNode != nil {
				oldChildren = ns.currentNode.GetChildren()
			}

			var selectedName string
			// Account for .. entry when getting selected child name
			childIndex := ns.selectedIndex
			if ns.CanNavigateBack() {
				childIndex-- // Account for .. entry at index 0
			}
			if childIndex >= 0 && childIndex < len(oldChildren) {
				selectedName = oldChildren[childIndex].Name
			}

			// Try to find the same name in the new children
			newIndex := -1
			if selectedName != "" {
				for i, child := range children {
					if child.Name == selectedName {
						newIndex = i
						break
					}
				}
			}

			if newIndex >= 0 {
				// Found the same name, account for .. entry when setting selection
				if ns.CanNavigateBack() {
					ns.selectedIndex = newIndex + 1 // Account for .. entry at index 0
				} else {
					ns.selectedIndex = newIndex
				}
			} else {
				// Name not found, ensure current index is still valid
				totalOptions := len(children)
				if ns.CanNavigateBack() {
					totalOptions++ // Account for .. entry
				}

				if ns.selectedIndex >= totalOptions {
					ns.selectedIndex = totalOptions - 1
				}
				if ns.selectedIndex < 0 {
					ns.selectedIndex = 0
				}
			}
		} else {
			ns.selectedIndex = 0
		}
	}

	ns.currentNode = node
	ns.lastRefresh = time.Now()
	ns.refreshing = false

	return nil
}

// GetLastRefresh returns the time of last successful refresh
func (ns *NavigationState) GetLastRefresh() time.Time {
	return ns.lastRefresh
}

// IsRefreshing returns true if currently refreshing
func (ns *NavigationState) IsRefreshing() bool {
	return ns.refreshing
}

// GetErrorMessage returns the current error message
func (ns *NavigationState) GetErrorMessage() string {
	return ns.errorMessage
}

// ClearError clears the current error message
func (ns *NavigationState) ClearError() {
	ns.errorMessage = ""
}

// GetBreadcrumbs returns a breadcrumb trail for the current path
func (ns *NavigationState) GetBreadcrumbs() []string {
	if ns.currentPath == "/" {
		return []string{"root"}
	}

	parts := strings.Split(strings.Trim(ns.currentPath, "/"), "/")
	breadcrumbs := []string{"root"}

	for _, part := range parts {
		if part != "" {
			breadcrumbs = append(breadcrumbs, part)
		}
	}

	return breadcrumbs
}

// GetMetricInfo returns information about the current node's metric type and flags
func (ns *NavigationState) GetMetricInfo() (madmin.MetricType, madmin.MetricFlags) {
	if ns.currentNode == nil {
		return madmin.MetricsNone, 0
	}
	return ns.currentNode.GetMetricType(), ns.currentNode.GetMetricFlags()
}

// CanNavigateBack returns true if we can navigate back
func (ns *NavigationState) CanNavigateBack() bool {
	return len(ns.pathHistory) > 0
}

// getCurrentMetricFlags returns the metric flags currently being used
func (ns *NavigationState) getCurrentMetricFlags() madmin.MetricFlags {
	// For now, return the current node's flags if any
	if ns.currentNode != nil {
		return ns.currentNode.GetMetricFlags()
	}
	return 0
}
