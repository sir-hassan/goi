package ui

import (
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
)

type Table struct {
	values       map[string]string
	width        int
	cachedView   string
	cachedHeight int
	dirty        bool
	progressBar  progress.Model
}

var (
	keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	valueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	cellStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderRight(true).
		BorderBottom(true).
		Padding(0, 1)
)

func NewTable() *Table {
	return &Table{
		values: make(map[string]string),
		dirty:  true,
		progressBar: progress.New(
			progress.WithWidth(18),
			progress.WithDefaultBlend(),
			progress.WithFillCharacters(progress.DefaultFullCharFullBlock, ' '),
		),
	}
}

func (t *Table) SetWidth(width int) {
	if t.width == width {
		return
	}
	t.width = width
	t.dirty = true
}

func (t *Table) SetKeyValue(key, value string) {
	if current, ok := t.values[key]; ok && current == value {
		return
	}

	t.values[key] = value
	t.dirty = true
}

func (t *Table) Clear() {
	if len(t.values) == 0 {
		return
	}

	clear(t.values)
	t.dirty = true
}

func (t *Table) View() string {
	t.render()
	return t.cachedView
}

func (t *Table) Height() int {
	t.render()
	return t.cachedHeight
}

func (t *Table) render() {
	if !t.dirty {
		return
	}

	t.cachedView = t.renderStringMapTable(t.values, t.width)
	t.cachedHeight = lipgloss.Height(t.cachedView)
	t.dirty = false
}

func (t *Table) renderStringMapTable(values map[string]string, availableWidth int) string {
	keys := make([]string, 0, len(values))
	keyWidth := 0
	valueWidth := 0
	renderedValues := make(map[string]string, len(values))

	for key, value := range values {
		keys = append(keys, key)
		keyWidth = max(keyWidth, lipgloss.Width(key))
		renderedValues[key] = t.renderTableValue(value)
		valueWidth = max(valueWidth, lipgloss.Width(renderedValues[key]))
	}

	sort.Strings(keys)

	keyColumn := lipgloss.NewStyle().Width(keyWidth)
	rowGap := 2
	columnGap := 4
	cellContentWidth := keyWidth + rowGap + valueWidth
	cellWidth := cellContentWidth + cellStyle.GetHorizontalFrameSize()
	columnWidth := cellWidth
	columnCount := 1

	if availableWidth >= columnWidth*3+columnGap*2 {
		columnCount = 3
	} else if availableWidth >= columnWidth*2+columnGap {
		columnCount = 2
	}

	rowsPerColumn := (len(keys) + columnCount - 1) / columnCount
	columns := make([]string, 0, columnCount)

	for col := 0; col < columnCount; col++ {
		start := col * rowsPerColumn
		if start >= len(keys) {
			break
		}

		end := min(start+rowsPerColumn, len(keys))
		rows := make([]string, 0, end-start)

		for _, key := range keys[start:end] {
			content := lipgloss.JoinHorizontal(
				lipgloss.Top,
				keyStyle.Render(keyColumn.Render(key)),
				strings.Repeat(" ", rowGap),
				valueStyle.Render(renderedValues[key]),
			)
			rows = append(rows, cellStyle.Width(cellWidth).Render(content))
		}

		columns = append(columns, lipgloss.NewStyle().Width(columnWidth).Render(strings.Join(rows, "\n")))
	}

	if len(columns) == 0 {
		return ""
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, columns...)
}

func (t *Table) renderTableValue(value string) string {
	percent, ok := parsePercentValue(value)
	if !ok {
		return value
	}

	return t.progressBar.ViewAs(float64(percent) / 100)
}

func parsePercentValue(value string) (int, bool) {
	if !strings.HasPrefix(value, "%") {
		return 0, false
	}

	percent, err := strconv.Atoi(strings.TrimPrefix(value, "%"))
	if err != nil || percent < 0 || percent > 100 {
		return 0, false
	}

	return percent, true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
