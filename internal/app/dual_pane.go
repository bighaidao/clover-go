package app

import (
	"strings"

	"github.com/bighaidao/clover-go/internal/domain"
	"github.com/bighaidao/clover-go/internal/fs"
	"github.com/bighaidao/clover-go/internal/macos"
	"github.com/bighaidao/clover-go/internal/search"
)

// ToggleDualPane toggles dual pane mode on the active tab and loads the secondary pane.
func (c *Coordinator) ToggleDualPane() {
	tab := c.Tabs.ActiveTab()
	if tab == nil {
		return
	}
	tab.ToggleDualPane()
	if tab.IsDual && tab.RightPane != nil {
		// Newly created right panes start with the preferred default ordering.
		tab.RightPane.Sort = c.Tabs.DefaultSort
		c.LoadPane(tab.RightPane, nil)
	}
	c.Invalidate()
}

// SwitchActivePane changes the active pane focus between left and right.
func (c *Coordinator) SwitchActivePane() {
	tab := c.Tabs.ActiveTab()
	if tab == nil || !tab.IsDual {
		return
	}
	tab.SwitchActivePane()
	c.Invalidate()
}

// SetActivePaneExplicit sets the active pane index directly (0 = Left, 1 = Right).
func (c *Coordinator) SetActivePaneExplicit(tab *TabState, index int) {
	if tab == nil || !tab.IsDual {
		return
	}
	if index != tab.ActiveIdx && (index == 0 || index == 1) {
		tab.ActiveIdx = index
		c.Invalidate()
	}
}

// LoadPane asynchronously fetches directory entries for a specific pane.
func (c *Coordinator) LoadPane(pane *PaneState, onDone func()) {
	if pane == nil {
		return
	}
	pane.IsSearchMode = false
	pane.LoadGen++
	gen := pane.LoadGen
	pane.IsLoading = true
	path := pane.Path
	showHidden := c.Prefs.Get().ShowHidden
	keepPlace := onDone == nil

	go func() {
		entries, _, err := fs.ReadDirPage(fs.ReadDirOptions{
			Path:       path,
			ShowHidden: showHidden,
		})

		var freeBytes uint64
		if _, free, vErr := fs.GetVolumeStats(path); vErr == nil {
			freeBytes = free
		}

		c.runOnMain(func() {
			if gen != pane.LoadGen {
				return
			}
			pane.IsLoading = false
			pane.VolumeFreeBytes = freeBytes
			if err != nil {
				pane.LoadErr = err.Error()
				pane.Entries = nil
				pane.DisplayEntries = nil
				if onDone != nil {
					onDone()
				}
				c.Invalidate()
				return
			}
			pane.LoadErr = ""

			domain.SortFiles(entries, pane.Sort)
			pane.Entries = entries
			if keepPlace {
				pane.TableState.HoldPlace()
			}
			c.applyPaneFilterInternal(pane, keepPlace)

			if onDone != nil {
				onDone()
			}
			c.Invalidate()
		})
	}()
}

// ApplyPaneFilter calculates DisplayEntries for a given pane based on its FilterQuery and the global FilterTag.
func (c *Coordinator) ApplyPaneFilter(pane *PaneState) {
	c.applyPaneFilterInternal(pane, true)
}

func (c *Coordinator) applyPaneFilterInternal(pane *PaneState, holdPlace bool) {
	if pane == nil {
		return
	}
	if holdPlace {
		pane.TableState.HoldPlace()
	}
	query := strings.TrimSpace(pane.FilterQuery)
	filtered := pane.Entries

	if query != "" {
		var qFiltered []domain.FileInfo
		for _, item := range filtered {
			if _, ok := search.ScoreItem(query, item.Name); ok {
				qFiltered = append(qFiltered, item)
			}
		}
		filtered = qFiltered
	}

	if c.FilterTag > 0 {
		var tFiltered []domain.FileInfo
		for _, item := range filtered {
			if macos.ColorIndexFromTags(item.ColorTags) == c.FilterTag {
				tFiltered = append(tFiltered, item)
			}
		}
		filtered = tFiltered
	}

	pane.DisplayEntries = filtered
}
