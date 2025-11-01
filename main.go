package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SafetyLevel represents how safe it is to remove a directory
type SafetyLevel int

const (
	Safe SafetyLevel = iota
	Moderate
	Risky
)

func (s SafetyLevel) String() string {
	safeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	moderateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	riskyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	unknownStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true)

	switch s {
	case Safe:
		return safeStyle.Render("SAFE")
	case Moderate:
		return moderateStyle.Render("MODERATE")
	case Risky:
		return riskyStyle.Render("RISKY")
	default:
		return unknownStyle.Render("UNKNOWN")
	}
}

// DevDirectory represents a development output directory
type DevDirectory struct {
	Path        string
	Description string
	Safety      SafetyLevel
	Selected    bool
	Size        int64
}

// Common development directories to scan for
var commonDevDirs = map[string]DevDirectory{
	// Very safe - these are clearly build artifacts/dependencies
	"node_modules":     {Description: "Node.js dependencies", Safety: Safe},
	"target":           {Description: "Maven/Cargo build output", Safety: Safe},
	"__pycache__":      {Description: "Python bytecode cache", Safety: Safe},
	".pytest_cache":    {Description: "Pytest cache", Safety: Safe},
	".tox":             {Description: "Tox testing cache", Safety: Safe},
	"DerivedData":      {Description: "Xcode derived data", Safety: Safe},
	".next":            {Description: "Next.js build cache", Safety: Safe},
	".nuxt":            {Description: "Nuxt.js build cache", Safety: Safe},
	".gradle":          {Description: "Gradle cache", Safety: Safe},
	".pub-cache":       {Description: "Dart/Flutter pub cache", Safety: Safe},
	".flutter-plugins": {Description: "Flutter plugins cache", Safety: Safe},
	"coverage":         {Description: "Test coverage reports", Safety: Safe},
	"tmp":              {Description: "Temporary files", Safety: Safe},
	"temp":             {Description: "Temporary files", Safety: Safe},
	".cache":           {Description: "Hidden cache directory", Safety: Safe},
	"obj":              {Description: "Object files", Safety: Safe},
	"intermediate":     {Description: "Intermediate build files", Safety: Safe},
	"android/build":    {Description: "Android build output", Safety: Safe},

	// Moderate safety - could contain important files
	"dist":      {Description: "Distribution/build output", Safety: Moderate},
	"build":     {Description: "Build output", Safety: Moderate},
	"out":       {Description: "Output directory", Safety: Moderate},
	"bin":       {Description: "Binary output", Safety: Moderate},
	"release":   {Description: "Release builds", Safety: Moderate},
	"debug":     {Description: "Debug builds", Safety: Moderate},
	"logs":      {Description: "Log files", Safety: Moderate},
	"cache":     {Description: "Cache directory", Safety: Moderate},
	"Pods":      {Description: "CocoaPods dependencies", Safety: Moderate},
	"lib":       {Description: "Library files", Safety: Moderate},
	"generated": {Description: "Generated code", Safety: Moderate},
	"artifacts": {Description: "Build artifacts", Safety: Moderate},
	"output":    {Description: "Build output", Safety: Moderate},
	".m2":       {Description: "Maven local repository", Safety: Moderate},
	"packages":  {Description: "Package cache", Safety: Moderate},
	"ios/Pods":  {Description: "iOS CocoaPods", Safety: Moderate},

	// Risky - version control and important dependencies
	"vendor": {Description: "Vendor dependencies", Safety: Risky},
	".git":   {Description: "Git repository", Safety: Risky},
	".svn":   {Description: "SVN repository", Safety: Risky},
}

type model struct {
	table            table.Model
	directories      []DevDirectory
	scanning         bool
	scanPath         string
	progress         progress.Model
	scanProgress     float64
	totalDirs        int
	scannedDirs      int
	showConfirmation bool
	pendingAction    string // "delete" or "deleteAll"
	width            int
	height           int
	showSplash       bool
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		scanDirectories(m.scanPath),
		tickProgress(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetWidth(msg.Width - 16)
		tableHeight := msg.Height - 18
		if tableHeight < 8 {
			tableHeight = 8
		}
		m.table.SetHeight(tableHeight)

		// Update table columns with responsive widths
		availableWidth := msg.Width - 16
		if availableWidth < 80 {
			availableWidth = 80
		}

		pathWidth := int(float64(availableWidth) * 0.45)
		descWidth := int(float64(availableWidth) * 0.25)
		if pathWidth < 30 {
			pathWidth = 30
		}
		if descWidth < 15 {
			descWidth = 15
		}

		columns := []table.Column{
			{Title: "Select", Width: 8},
			{Title: "Path", Width: pathWidth},
			{Title: "Description", Width: descWidth},
			{Title: "Safety", Width: 18},
			{Title: "Size", Width: 12},
		}
		m.table.SetColumns(columns)
	case tea.KeyMsg:
		if m.showConfirmation {
			switch msg.String() {
			case "y", "Y":
				// Confirm deletion
				m.showConfirmation = false
				if m.pendingAction == "delete" {
					return m, deleteSelected(m.directories)
				} else if m.pendingAction == "deleteAll" {
					return m, deleteSelected(m.directories)
				}
			case "n", "N", "esc":
				// Cancel deletion
				m.showConfirmation = false
				m.pendingAction = ""
			}
			return m, nil
		}

		switch msg.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "enter", " ":
			// Toggle selection
			if m.table.Cursor() < len(m.directories) {
				m.directories[m.table.Cursor()].Selected = !m.directories[m.table.Cursor()].Selected
				m.updateTableRows()
			}
		case "d":
			// Show confirmation for delete selected directories
			m.showConfirmation = true
			m.pendingAction = "delete"
		case "D":
			// Select all SAFE directories and show confirmation
			for i := range m.directories {
				if m.directories[i].Safety == Safe {
					m.directories[i].Selected = true
				}
			}
			m.updateTableRows()
			m.showConfirmation = true
			m.pendingAction = "deleteAll"
		case "r":
			// Refresh scan
			m.scanning = true
			return m, scanDirectories(m.scanPath)
		}
	case scanCompleteMsg:
		m.directories = msg.directories
		// Sort directories by safety level: SAFE, MODERATE, RISKY
		sort.Slice(m.directories, func(i, j int) bool {
			return m.directories[i].Safety < m.directories[j].Safety
		})
		m.scanning = false
		m.scanProgress = 1.0
		m.showSplash = false
		m.updateTableRows()
	case scanProgressMsg:
		m.scanProgress = msg.progress
		m.scannedDirs = msg.scannedDirs
		m.totalDirs = msg.totalDirs
	case progressTickMsg:
		if m.scanning && m.scanProgress < 1.0 {
			// Simulate progress increment
			m.scanProgress += 0.02 // Increment by 2% each tick
			if m.scanProgress > 1.0 {
				m.scanProgress = 1.0
			}
			m.scannedDirs += 50 // Simulate scanning directories
			return m, tickProgress()
		}
	case deleteCompleteMsg:
		// Refresh after deletion
		m.scanning = true
		return m, scanDirectories(m.scanPath)
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *model) updateTableRows() {
	rows := make([]table.Row, len(m.directories))
	checkboxStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)

	// Calculate responsive column widths based on terminal size
	availableWidth := m.width - 12 // Account for outer container padding + borders + margin + extra space
	if availableWidth < 80 {
		availableWidth = 80 // Minimum width
	}

	// Distribute widths proportionally
	pathWidth := int(float64(availableWidth) * 0.45) // 45% for path
	descWidth := int(float64(availableWidth) * 0.25) // 25% for description

	// Ensure minimum widths
	if pathWidth < 30 {
		pathWidth = 30
	}
	if descWidth < 15 {
		descWidth = 15
	}

	for i, dir := range m.directories {
		checkbox := checkboxStyle.Render("[ ]")
		if dir.Selected {
			checkbox = selectedStyle.Render("[X]")
		}

		// Convert to tilde path and truncate if too long
		path := convertToTildePath(dir.Path)
		if len(path) > pathWidth-3 {
			path = "..." + path[len(path)-(pathWidth-6):]
		}

		// Truncate description if too long
		description := dir.Description
		if len(description) > descWidth-3 {
			description = description[:descWidth-6] + "..."
		}

		sizeStr := formatSize(dir.Size)

		rows[i] = table.Row{
			checkbox,
			path,
			description,
			dir.Safety.String(),
			sizeStr,
		}
	}
	m.table.SetRows(rows)
}

func (m model) View() string {
	scanningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)

	// Create outer container style
	outerContainerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		Margin(1)

	if m.showSplash {
		artStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("45")).
			Bold(true)

		progressView := m.progress.ViewAs(m.scanProgress)
		statusText := fmt.Sprintf("Scanning %s", m.scanPath)

		content := artStyle.Render(splashArt) + "\n" + scanningStyle.Render(statusText) + "\n\n" + progressView
		return outerContainerStyle.Render(content)
	}

	if m.scanning {
		progressView := m.progress.ViewAs(m.scanProgress)
		statusText := fmt.Sprintf("SCANNING %s\nDirectories scanned: %d", m.scanPath, m.scannedDirs)
		content := scanningStyle.Render(statusText) + "\n\n" + progressView + "\n\nPress 'q' to quit"
		return outerContainerStyle.Render(content)
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("\n\nControls:\n• Space/Enter: Toggle selection\n• 'd': Delete selected\n• 'D': Delete all SAFE directories\n• 'r': Refresh scan\n• 'q': Quit")

	totalSelected := 0
	totalSize := int64(0)
	for _, dir := range m.directories {
		if dir.Selected {
			totalSelected++
			totalSize += dir.Size
		}
	}

	status := fmt.Sprintf("\nSelected: %d directories (%s)", totalSelected, formatSize(totalSize))
	mainContent := m.table.View() + status + help
	mainView := outerContainerStyle.Render(mainContent)

	// If showing confirmation dialog, overlay it in the center
	if m.showConfirmation {
		// Use actual terminal dimensions for the overlay
		containerWidth := m.width
		containerHeight := m.height
		if containerWidth == 0 {
			containerWidth = 80
		}
		if containerHeight == 0 {
			containerHeight = 24
		}
		return m.renderConfirmationDialog(mainView, totalSelected, totalSize, containerWidth, containerHeight)
	}

	return mainView
}

func (m model) renderConfirmationDialog(mainView string, totalSelected int, totalSize int64, containerWidth, containerHeight int) string {
	// Calculate responsive dialog width (max 60 chars, but adjust for smaller terminals)
	dialogWidth := 60
	if containerWidth < 70 {
		dialogWidth = containerWidth - 10 // Leave some margin
	}

	// Red confirmation dialog styles
	dangerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")). // Bright red
		Background(lipgloss.Color("52")).        // Dark red background
		Foreground(lipgloss.Color("255")).       // White text
		Bold(true).
		Padding(1, 2).
		Width(dialogWidth).
		Align(lipgloss.Center)

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)

	var message string
	if m.pendingAction == "deleteAll" {
		safeCount := 0
		for _, dir := range m.directories {
			if dir.Selected && dir.Safety == Safe {
				safeCount++
			}
		}
		message = fmt.Sprintf(
			"%s\n\nYou are about to DELETE %d SAFE directories!\nThis will permanently remove %s of data.\n\n%s\n\nType 'y' to confirm, 'n' to cancel",
			warningStyle.Render("⚠ BULK DELETE CONFIRMATION ⚠"),
			safeCount,
			formatSize(totalSize),
			warningStyle.Render("THIS ACTION CANNOT BE UNDONE!"),
		)
	} else {
		message = fmt.Sprintf(
			"%s\n\nYou are about to DELETE %d selected directories!\nThis will permanently remove %s of data.\n\n%s\n\nType 'y' to confirm, 'n' to cancel",
			warningStyle.Render("⚠ DELETE CONFIRMATION ⚠"),
			totalSelected,
			formatSize(totalSize),
			warningStyle.Render("THIS ACTION CANNOT BE UNDONE!"),
		)
	}

	confirmDialog := dangerStyle.Render(message)

	// Use lipgloss.Place to center the dialog using actual container dimensions
	return lipgloss.Place(containerWidth, containerHeight, lipgloss.Center, lipgloss.Center, confirmDialog, lipgloss.WithWhitespaceForeground(lipgloss.Color("240")))
}

type scanCompleteMsg struct {
	directories []DevDirectory
}

type scanProgressMsg struct {
	progress    float64
	scannedDirs int
	totalDirs   int
}

type progressTickMsg struct{}

type deleteCompleteMsg struct{}

func tickProgress() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return progressTickMsg{}
	})
}

func scanDirectories(rootPath string) tea.Cmd {
	return func() tea.Msg {
		var foundDirs []DevDirectory

		// Resolve the root path if it's a symlink
		resolvedRoot, err := filepath.EvalSymlinks(rootPath)
		if err != nil {
			// If we can't resolve symlinks, try the original path
			resolvedRoot = rootPath
		}

		var scannedDirs int

		err = filepath.Walk(resolvedRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors, continue scanning
			}

			// Handle symlinks by resolving them
			if info.Mode()&os.ModeSymlink != 0 {
				realPath, err := filepath.EvalSymlinks(path)
				if err != nil {
					return nil // Skip broken symlinks
				}
				// Get info for the real path
				realInfo, err := os.Stat(realPath)
				if err != nil {
					return nil // Skip if we can't stat the real path
				}
				info = realInfo
			}

			if info.IsDir() {
				scannedDirs++
			}

			if !info.IsDir() {
				return nil
			}

			dirName := filepath.Base(path)

			// Check if this directory matches our common dev directories
			if template, exists := commonDevDirs[dirName]; exists {
				size := calculateDirSize(path)
				foundDir := DevDirectory{
					Path:        path,
					Description: template.Description,
					Safety:      template.Safety,
					Selected:    false,
					Size:        size,
				}
				foundDirs = append(foundDirs, foundDir)
			}

			// Continue scanning all directories recursively
			return nil
		})

		if err != nil {
			// Return empty result on error
			return scanCompleteMsg{directories: []DevDirectory{}}
		}

		return scanCompleteMsg{directories: foundDirs}
	}
}

func deleteSelected(directories []DevDirectory) tea.Cmd {
	return func() tea.Msg {
		for _, dir := range directories {
			if dir.Selected {
				os.RemoveAll(dir.Path)
			}
		}
		return deleteCompleteMsg{}
	}
}

func calculateDirSize(path string) int64 {
	var size int64

	// Use a timeout to avoid hanging on large directories
	done := make(chan bool, 1)
	go func() {
		filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				if info, err := d.Info(); err == nil {
					size += info.Size()
				}
			}
			return nil
		})
		done <- true
	}()

	select {
	case <-done:
		return size
	case <-time.After(2 * time.Second):
		return -1 // Indicate calculation timed out
	}
}

func formatSize(bytes int64) string {
	if bytes < 0 {
		return "calculating..."
	}

	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// convertToTildePath converts absolute paths to use ~ notation for home directory
func convertToTildePath(absPath string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return absPath // Return original path if we can't get home dir
	}

	if strings.HasPrefix(absPath, homeDir) {
		return "~" + absPath[len(homeDir):]
	}

	return absPath
}

const splashArt = `
 ██████████                          █████████  ████                               
░░███░░░░███                        ███░░░░░███░░███                               
 ░███   ░░███  ██████  █████ █████ ███     ░░░  ░███   ██████   ██████   ████████  
 ░███    ░███ ███░░███░░███ ░░███ ░███          ░███  ███░░███ ░░░░░███ ░░███░░███ 
 ░███    ░███░███████  ░███  ░███ ░███          ░███ ░███████   ███████  ░███ ░███ 
 ░███    ███ ░███░░░   ░░███ ███  ░░███     ███ ░███ ░███░░░   ███░░███  ░███ ░███ 
 ██████████  ░░██████   ░░█████    ░░█████████  █████░░██████ ░░████████ ████ █████
░░░░░░░░░░    ░░░░░░     ░░░░░      ░░░░░░░░░  ░░░░░  ░░░░░░   ░░░░░░░░ ░░░░ ░░░░░ 
`

func main() {
	// Get scan path from command line argument or use current directory
	scanPath := "."
	if len(os.Args) > 1 {
		scanPath = os.Args[1]
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(scanPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	columns := []table.Column{
		{Title: "Select", Width: 8},
		{Title: "Path", Width: 55},
		{Title: "Description", Width: 30},
		{Title: "Safety", Width: 18},
		{Title: "Size", Width: 12},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(20),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true).
		Padding(0, 1)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	s.Cell = s.Cell.Padding(0, 1)
	t.SetStyles(s)

	// Initialize progress bar
	prog := progress.New(progress.WithDefaultGradient())
	prog.Width = 50

	m := model{
		table:            t,
		scanning:         true,
		scanPath:         absPath,
		progress:         prog,
		scanProgress:     0.0,
		showConfirmation: false,
		pendingAction:    "",
		width:            80, // Default width
		height:           24, // Default height
		showSplash:       true,
	}

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}
