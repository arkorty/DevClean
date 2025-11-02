package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
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
	switch s {
	case Safe:
		return "SAFE"
	case Moderate:
		return "MODERATE"
	case Risky:
		return "RISKY"
	default:
		return "UNKNOWN"
	}
}

func (s SafetyLevel) Color() lipgloss.Color {
	switch s {
	case Safe:
		return lipgloss.Color("2")
	case Moderate:
		return lipgloss.Color("3")
	case Risky:
		return lipgloss.Color("1")
	default:
		return lipgloss.Color("8")
	}
}

// DevDirectory represents a development output directory
type DevDirectory struct {
	Path        string
	Description string
	Safety      SafetyLevel
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
	directories      []DevDirectory
	cursor           int
	selected         map[int]bool
	scanning         bool
	scanPath         string
	progress         progress.Model
	scanProgress     float64
	scannedDirs      int
	showConfirmation bool
	pendingAction    string // "delete" or "deleteAll"
	width            int
	height           int
	showSplash       bool
	viewportStart    int
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

	case tea.KeyMsg:
		if m.showConfirmation {
			switch msg.String() {
			case "y", "Y":
				m.showConfirmation = false
				if m.pendingAction == "delete" || m.pendingAction == "deleteAll" {
					return m, deleteSelected(m.directories, m.selected)
				}
			case "n", "N", "esc":
				m.showConfirmation = false
				m.pendingAction = ""
			}
			return m, nil
		}

		switch msg.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustViewport()
			}
		case "down", "j":
			if m.cursor < len(m.directories)-1 {
				m.cursor++
				m.adjustViewport()
			}
		case "enter", " ":
			if m.cursor < len(m.directories) {
				m.selected[m.cursor] = !m.selected[m.cursor]
				// Move cursor down after selecting
				if m.cursor < len(m.directories)-1 {
					m.cursor++
					m.adjustViewport()
				}
			}
			return m, nil
		case "d":
			m.showConfirmation = true
			m.pendingAction = "delete"
		case "D":
			for i := range m.directories {
				if m.directories[i].Safety == Safe {
					m.selected[i] = true
				}
			}
			m.showConfirmation = true
			m.pendingAction = "deleteAll"
		case "r":
			m.scanning = true
			return m, scanDirectories(m.scanPath)
		}

	case scanCompleteMsg:
		m.directories = msg.directories
		sort.Slice(m.directories, func(i, j int) bool {
			return m.directories[i].Safety < m.directories[j].Safety
		})
		m.scanning = false
		m.scanProgress = 1.0
		m.showSplash = false

	case scanProgressMsg:
		m.scanProgress = msg.progress
		m.scannedDirs = msg.scannedDirs

	case progressTickMsg:
		if m.scanning && m.scanProgress < 1.0 {
			m.scanProgress += 0.02
			if m.scanProgress > 1.0 {
				m.scanProgress = 1.0
			}
			m.scannedDirs += 50
			return m, tickProgress()
		}

	case deleteCompleteMsg:
		m.scanning = true
		m.selected = make(map[int]bool)
		m.cursor = 0
		return m, scanDirectories(m.scanPath)
	}

	return m, cmd
}

func (m *model) adjustViewport() {
	visibleRows := m.height - 18 // Match the visible rows calculation in renderMain
	if visibleRows < 3 {
		visibleRows = 3
	}

	if m.cursor < m.viewportStart {
		m.viewportStart = m.cursor
	} else if m.cursor >= m.viewportStart+visibleRows {
		m.viewportStart = m.cursor - visibleRows + 1
	}
}

func (m model) View() string {
	if m.showSplash {
		return m.renderSplash()
	}

	if m.scanning {
		return m.renderScanning()
	}

	if m.showConfirmation {
		return m.renderConfirmation()
	}

	return m.renderMain()
}

func (m model) renderSplash() string {
	artStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("45")).
		Bold(true)

	scanningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("6")).
		Bold(true)

	progressView := m.progress.ViewAs(m.scanProgress)
	statusText := fmt.Sprintf("Scanning %s", m.scanPath)

	content := artStyle.Render(splashArt) + "\n" + scanningStyle.Render(statusText) + "\n\n" + progressView

	outerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		Margin(1)

	return outerStyle.Render(content)
}

func (m model) renderScanning() string {
	scanningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("6")).
		Bold(true)

	progressView := m.progress.ViewAs(m.scanProgress)
	statusText := fmt.Sprintf("SCANNING %s\nDirectories scanned: %d", m.scanPath, m.scannedDirs)

	content := scanningStyle.Render(statusText) + "\n\n" + progressView + "\n\nPress 'q' to quit"

	outerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		Margin(1)

	return outerStyle.Render(content)
}

func (m model) renderMain() string {
	var b strings.Builder

	// Calculate available width
	contentWidth := m.width - 6 // Account for border and padding
	if contentWidth < 80 {
		contentWidth = 80
	}

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15"))

	// Column widths
	selectWidth := 8
	pathWidth := int(float64(contentWidth-selectWidth-12-12) * 0.65) // 65% of remaining (more for paths)
	descWidth := int(float64(contentWidth-selectWidth-12-12) * 0.15) // 15% of remaining (much narrower)
	safetyWidth := 12
	sizeWidth := 12

	header := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s",
		selectWidth, "Select",
		pathWidth, "Path",
		descWidth, "Description",
		safetyWidth, "Safety",
		sizeWidth, "Size")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Calculate visible rows based on available space
	// Account for: border (2), padding (2), header (1), blank line (1), status (2), blank line (1), controls (9)
	// Total overhead: ~18 lines
	visibleRows := m.height - 18
	if visibleRows < 3 {
		visibleRows = 3
	}

	end := m.viewportStart + visibleRows
	if end > len(m.directories) {
		end = len(m.directories)
	}

	// Rows
	for i := m.viewportStart; i < end; i++ {
		b.WriteString(m.renderRow(i, selectWidth, pathWidth, descWidth, safetyWidth, sizeWidth, contentWidth))
		b.WriteString("\n")
	}

	// Fill remaining space with empty lines to maintain consistent height
	rowsDisplayed := end - m.viewportStart
	if rowsDisplayed < visibleRows {
		for i := rowsDisplayed; i < visibleRows; i++ {
			b.WriteString(strings.Repeat(" ", contentWidth))
			b.WriteString("\n")
		}
	}

	// Status
	totalSelected := 0
	totalSize := int64(0)
	for idx := range m.directories {
		if m.selected[idx] {
			totalSelected++
			totalSize += m.directories[idx].Size
		}
	}

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	b.WriteString("\n")
	b.WriteString(statusStyle.Render(fmt.Sprintf("Selected: %d directories (%s)", totalSelected, formatSize(totalSize))))
	b.WriteString("\n\n")

	// Help - use multiple lines
	b.WriteString(statusStyle.Render("Controls:"))
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("\tk or up arrow: go up"))
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("\tj or down arrow: go down"))
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("\tspace/enter: toggle selection"))
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("\td: delete selected"))
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("\tD: auto-delete SAFE tagged folders"))
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("\tr: refresh scan"))
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("\tq: quit"))

	outerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		Width(m.width - 2)

	return outerStyle.Render(b.String())
}

func (m model) renderRow(idx, selectWidth, pathWidth, descWidth, safetyWidth, sizeWidth, totalWidth int) string {
	dir := m.directories[idx]

	checkbox := "[ ]"
	if m.selected[idx] {
		checkbox = "[X]"
	}

	path := convertToTildePath(dir.Path)

	// Extract the matched pattern (last directory component)
	matchedPattern := filepath.Base(dir.Path)

	// Truncate path if needed
	if len(path) > pathWidth {
		path = "..." + path[len(path)-(pathWidth-3):]
	}

	desc := dir.Description
	if len(desc) > descWidth {
		desc = desc[:descWidth-3] + "..."
	}

	safetyText := dir.Safety.String()
	size := formatSize(dir.Size)

	// Build row with plain text and proper spacing
	row := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s",
		selectWidth, checkbox,
		pathWidth, path,
		descWidth, desc,
		safetyWidth, safetyText,
		sizeWidth, size)

	// Pad to full width
	if len(row) < totalWidth {
		row = row + strings.Repeat(" ", totalWidth-len(row))
	}

	if idx == m.cursor {
		cursorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57"))
		return cursorStyle.Render(row)
	}

	// For non-cursor rows, apply color to safety column and highlight matched pattern in red
	safetyStyle := lipgloss.NewStyle().
		Foreground(dir.Safety.Color()).
		Bold(true)

	redStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)

	// Highlight the matched pattern in the path
	pathWithHighlight := path
	if strings.Contains(path, matchedPattern) {
		// Find the last occurrence of the pattern in the displayed path
		idx := strings.LastIndex(path, matchedPattern)
		if idx != -1 {
			before := path[:idx]
			pattern := path[idx : idx+len(matchedPattern)]
			after := path[idx+len(matchedPattern):]
			pathWithHighlight = before + redStyle.Render(pattern) + after
		}
	}

	// Build the row manually with styled safety and highlighted path
	prefix := fmt.Sprintf("%-*s ", selectWidth, checkbox)

	// Calculate spacing for path - we need to account for the actual display width
	pathSpacing := strings.Repeat(" ", pathWidth-len(path))

	styledSafety := safetyStyle.Render(fmt.Sprintf("%-*s", safetyWidth, safetyText))

	suffix := fmt.Sprintf(" %-*s", sizeWidth, size)

	// Calculate padding based on plain text length (without ANSI codes)
	plainRowLen := selectWidth + 1 + pathWidth + 1 + descWidth + 1 + safetyWidth + 1 + sizeWidth
	padding := ""
	if totalWidth > plainRowLen {
		padding = strings.Repeat(" ", totalWidth-plainRowLen)
	}

	return prefix + pathWithHighlight + pathSpacing + " " + fmt.Sprintf("%-*s ", descWidth, desc) + styledSafety + suffix + padding
}

func (m model) renderConfirmation() string {
	totalSelected := 0
	totalSize := int64(0)
	for idx := range m.directories {
		if m.selected[idx] {
			totalSelected++
			totalSize += m.directories[idx].Size
		}
	}

	dangerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Background(lipgloss.Color("52")).
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Padding(1, 2).
		Width(60).
		Align(lipgloss.Center)

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)

	var message string
	if m.pendingAction == "deleteAll" {
		safeCount := 0
		for idx := range m.directories {
			if m.selected[idx] && m.directories[idx].Safety == Safe {
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

	width := m.width
	height := m.height
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, confirmDialog, lipgloss.WithWhitespaceForeground(lipgloss.Color("240")))
}

type scanCompleteMsg struct {
	directories []DevDirectory
}

type scanProgressMsg struct {
	progress    float64
	scannedDirs int
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

		resolvedRoot, err := filepath.EvalSymlinks(rootPath)
		if err != nil {
			resolvedRoot = rootPath
		}

		err = filepath.Walk(resolvedRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if info.Mode()&os.ModeSymlink != 0 {
				realPath, err := filepath.EvalSymlinks(path)
				if err != nil {
					return nil
				}
				realInfo, err := os.Stat(realPath)
				if err != nil {
					return nil
				}
				info = realInfo
			}

			if !info.IsDir() {
				return nil
			}

			dirName := filepath.Base(path)

			if template, exists := commonDevDirs[dirName]; exists {
				size := calculateDirSize(path)
				foundDir := DevDirectory{
					Path:        path,
					Description: template.Description,
					Safety:      template.Safety,
					Size:        size,
				}
				foundDirs = append(foundDirs, foundDir)
			}

			return nil
		})

		if err != nil {
			return scanCompleteMsg{directories: []DevDirectory{}}
		}

		return scanCompleteMsg{directories: foundDirs}
	}
}

func deleteSelected(directories []DevDirectory, selected map[int]bool) tea.Cmd {
	return func() tea.Msg {
		for idx, dir := range directories {
			if selected[idx] {
				os.RemoveAll(dir.Path)
			}
		}
		return deleteCompleteMsg{}
	}
}

func calculateDirSize(path string) int64 {
	var size int64

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
		return -1
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

func convertToTildePath(absPath string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return absPath
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

const version = "v0.1.1"
const helpText = `DevClean: Code Cleanup, Simplified.

Usage:
  devclean [path]

Flags:

  --version    Show version information
  --help       Show this help message

If a path is not provided, it will scan the current directory.
`

func main() {
	versionFlag := flag.Bool("version", false, "Print version information")
	helpFlag := flag.Bool("help", false, "Print help information")

	flag.Parse()

	if *versionFlag {
		fmt.Println("DevClean", version)
		os.Exit(0)
	}

	if *helpFlag {
		fmt.Println(helpText)
		os.Exit(0)
	}

	scanPath := "."
	if flag.NArg() > 0 {
		scanPath = flag.Arg(0)
	}

	absPath, err := filepath.Abs(scanPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	prog := progress.New(progress.WithDefaultGradient())
	prog.Width = 50

	m := model{
		directories:      []DevDirectory{},
		cursor:           0,
		selected:         make(map[int]bool),
		scanning:         true,
		scanPath:         absPath,
		progress:         prog,
		scanProgress:     0.0,
		showConfirmation: false,
		pendingAction:    "",
		width:            80,
		height:           24,
		showSplash:       true,
		viewportStart:    0,
	}

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}
