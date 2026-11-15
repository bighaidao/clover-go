package ui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bighaidao/clover-go/internal/app"
	"github.com/bighaidao/clover-go/internal/config"
	"github.com/bighaidao/clover-go/internal/domain"
	"github.com/bighaidao/clover-go/internal/macos"
	"github.com/egoist/mygo/ui"
)

// ToolbarView renders navigation controls, breadcrumbs, search, and bookmark bar.
func ToolbarView(c *ui.Context, coord *app.Coordinator, onReload func()) {
	t := c.Theme()
	pal := Pal(t)
	activeTab := coord.Tabs.ActiveTab()
	if activeTab == nil {
		return
	}

	ui.Column(c).Background(t.Surface).Children(func() {
		// Main Navigation & Address Capsule Row
		ui.Row(c).Height(40).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			renderNavigationControls(c, coord, onReload)
			renderAddressCapsule(c, coord, activeTab, onReload)

			// Settings sits outside the capsule; refresh lives in the view
			// switcher group, next to the mode buttons it reloads for.
			btnSettings := IconButton(c, IcSettings, "偏好设置 (⌘,)", false)
			if btnSettings.Clicked() {
				coord.OpenSettings()
			}
		})

		// Optional Bookmark Bar
		if coord.Prefs.Get().ShowBookmark {
			ui.Box(c).Height(1).Background(pal.Line)
			renderBookmarkBar(c, coord, onReload)
		}

		ui.Box(c).Height(1).Background(pal.Line)
	})
}

func renderNavigationControls(c *ui.Context, coord *app.Coordinator, onReload func()) {
	tm := coord.Tabs
	idx := tm.ActiveIndex

	btnBack := IconButton(c, IcChevronLeft, "后退 (⌘[)", !tm.CanGoBack(idx))
	if btnBack.Clicked() {
		tm.GoBack(idx)
		if onReload != nil {
			onReload()
		}
	}

	btnFwd := IconButton(c, IcChevronRight, "前进 (⌘])", !tm.CanGoForward(idx))
	if btnFwd.Clicked() {
		tm.GoForward(idx)
		if onReload != nil {
			onReload()
		}
	}

	btnUp := IconButton(c, IcChevronUp, "上级目录 (⌘↑)", !tm.CanGoUp(idx))
	if btnUp.Clicked() {
		tm.GoUp(idx)
		if onReload != nil {
			onReload()
		}
	}
}

func renderAddressCapsule(c *ui.Context, coord *app.Coordinator, tab *app.TabState, onReload func()) {
	// Address bar always reflects and edits the ACTIVE pane (dual-pane aware).
	pane := tab.ActivePane()
	home, _ := os.UserHomeDir()
	crumbs := domain.SplitPathCrumbs(pane.Path, home)

	capsule := ui.Row(c).Grow(1).Height(30).Padding(0, 8).Radius(6).Background(Pal(c.Theme()).Inset).AlignItems(ui.Center).Gap(6)

	if coord.IsEditing {
		capsule.Children(func() {
			renderAddressEditingCapsule(c, coord, pane, onReload)
		})
	} else {
		capsule.Children(func() {
			renderAddressDisplayCapsule(c, coord, pane, crumbs, onReload)
		})
	}
}

func renderAddressEditingCapsule(c *ui.Context, coord *app.Coordinator, pane *app.PaneState, onReload func()) {
	t := c.Theme()
	txt := ui.TextInput(c, &pane.PathDraft).Grow(1).FontSize(12).SingleLine().
		TextColor(t.Text).CaretWidth(1.5).CaretColor(t.Accent)
	txt.OnTextKey(func(k ui.TextKey) bool {
		if k.Key == ui.KeyEscape {
			coord.IsEditing = false
			return true
		}
		return false
	})
	if txt.Submitted() {
		coord.IsEditing = false
		coord.Tabs.NavigatePane(coord.Tabs.ActiveIndex, pane, pane.PathDraft)
		if onReload != nil {
			onReload()
		}
	}
	btnCancel := ui.ButtonBase(c).Label("取消编辑 (Esc)").Padding(2).Radius(3).AlignItems(ui.Center)
	if btnCancel.Hovered() {
		btnCancel.Background(t.Text.Alpha(0.12))
	}
	btnCancel.Children(func() {
		ui.Icon(c, IcClose).Size(10, 10).TextColor(t.TextMuted)
	})
	if btnCancel.Clicked() {
		coord.IsEditing = false
	}
}

func renderAddressDisplayCapsule(c *ui.Context, coord *app.Coordinator, pane *app.PaneState, crumbs []domain.PathCrumb, onReload func()) {
	// Clickable breadcrumbs for instant level navigation
	BreadcrumbBar(c, crumbs, coord, func(target string) {
		coord.Tabs.Navigate(coord.Tabs.ActiveIndex, target)
		if onReload != nil {
			onReload()
		}
	})

	// Flexible spacer: clicking empty middle area switches to manual path edit
	spacer := ui.Box(c).Grow(1).Height(24)
	if spacer.Clicked() {
		coord.BeginPathEdit(pane)
	}

	// Embedded quick switcher (List / Grid / Preview pane) + Refresh
	renderViewSwitcher(c, coord, onReload)

	// Embedded New Folder button
	btnNewFolder := IconButton(c, IcNewFolder, "新建文件夹 (⇧⌘N)", false)
	if btnNewFolder.Clicked() {
		_, _ = coord.CreateNewFolder(pane, "未命名文件夹")
	}

	// Embedded Search box
	renderSearchBox(c, coord, pane)
}

func renderViewSwitcher(c *ui.Context, coord *app.Coordinator, onReload func()) {
	isList := coord.ViewMode == config.ViewList
	btnList := IconButton(c, IcList, "列表视图", false)
	if isList {
		btnList.Background(c.Theme().Text.Alpha(0.12))
	}
	if btnList.Clicked() {
		coord.ViewMode = config.ViewList
		p := coord.Prefs.Get()
		p.DefaultView = config.ViewList
		_ = coord.Prefs.Save(p)
		coord.Invalidate()
	}

	btnGrid := IconButton(c, IcGrid, "网格视图", false)
	if !isList {
		btnGrid.Background(c.Theme().Text.Alpha(0.12))
	}
	if btnGrid.Clicked() {
		coord.ViewMode = config.ViewGrid
		p := coord.Prefs.Get()
		p.DefaultView = config.ViewGrid
		_ = coord.Prefs.Save(p)
		coord.Invalidate()
	}

	activeTab := coord.Tabs.ActiveTab()
	isDual := activeTab != nil && activeTab.IsDual
	btnDual := IconButton(c, IcSplit, "双栏对等分屏 (⌥⌘D)", false)
	if isDual {
		btnDual.Background(c.Theme().Text.Alpha(0.12))
	}
	if btnDual.Clicked() {
		coord.ToggleDualPane()
	}

	btnPrev := IconButton(c, IcEye, "切换预览栏 (⇧⌘P)", false)
	if coord.ShowPreview {
		btnPrev.Background(c.Theme().Text.Alpha(0.12))
	}
	if btnPrev.Clicked() {
		coord.TogglePreview()
	}

	// 手动刷新：与 ⌘R 同一入口，强制重读当前目录（不受搜索模式影响）。
	btnReload := IconButton(c, IcRefresh, "刷新当前目录 (⌘R)", false)
	if btnReload.Clicked() && onReload != nil {
		onReload()
	}
}

func renderSearchBox(c *ui.Context, coord *app.Coordinator, pane *app.PaneState) {
	placeholder := "搜索当前目录 (⌘F)"
	if pane.IsSearchMode {
		placeholder = "搜索中 · 回车过滤"
	}
	// mygo v0.7.4 原生搜索框：自带放大镜、一键清除 × 与 Esc 清空。
	sf := ui.SearchField(c, &pane.FilterQuery).
		Width(155).Height(24).FontSize(11).Placeholder(placeholder).Label("搜索").
		Shrink(1).MinWidth(90)
	if coord.FocusFilter {
		// One-shot focus request from ⌘F; the root frame clears the flag
		// at end of frame so the focus is not stolen back every frame.
		sf.AutoFocus()
	}
	if sf.Changed() {
		if pane.IsSearchMode && strings.TrimSpace(pane.FilterQuery) == "" {
			// Clearing the query while in search mode exits and reloads
			// the real directory listing.
			coord.ExitSearch(pane)
		} else {
			coord.ApplyPaneFilter(pane)
		}
	}
	if sf.Submitted() {
		// Enter in the filter box triggers recursive search (Finder-like
		// "search this Mac/folder" behavior), finally wiring the search
		// engine to the UI.
		coord.StartSearch(pane)
	}
}

func renderBookmarkBar(c *ui.Context, coord *app.Coordinator, onReload func()) {
	t := c.Theme()
	bData := coord.Workspaces.Get()

	bar := ui.Row(c).Height(28).Padding(0, 12).Gap(6).AlignItems(ui.Center)
	bar.ContextMenu(func(m *ui.Menu) {
		activeTab := coord.Tabs.ActiveTab()
		if activeTab != nil {
			currPath := activeTab.ActivePane().Path
			if !coord.IsBookmarked(currPath) {
				if m.Item("添加当前文件夹为书签").Chosen() {
					_ = coord.AddBookmark(currPath, filepath.Base(currPath))
				}
			}
		}
		m.Separator()
		if m.Item("从 Finder 导入常用位置...").Chosen() {
			coord.OpenFinderImport()
		}
	})

	bar.Children(func() {
		ui.Icon(c, IcBookmark).Size(11, 11).TextColor(t.TextMuted).Shrink(0)
		ui.ScrollHorizontal(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Scrollbars(false).Children(func() {
			if len(bData.Bookmarks) == 0 {
				ui.Text(c, "书签栏：在文件夹上右键可添加至此").FontSize(11).TextColor(t.TextMuted.Alpha(0.6))
			} else {
				for _, b := range bData.Bookmarks {
					renderBookmarkItemButton(c, coord, b, onReload)
				}
			}
		})
		renderWorkspaceDropdown(c, coord)
	})
}

func renderBookmarkItemButton(c *ui.Context, coord *app.Coordinator, bm config.BookmarkItem, onReload func()) {
	t := c.Theme()
	hasChildren := len(bm.Children) > 0
	btn := ui.ButtonBase(c).Padding(2, 6).Radius(4).AlignItems(ui.Center).Justify(ui.Start).Shrink(0).Tooltip(bm.Name + "\n" + bm.Path)
	if btn.Hovered() {
		btn.Background(t.Text.Alpha(0.08))
	}
	btn.Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, IcFolder).Size(11, 11).TextColor(t.TextMuted).Shrink(0)
			name := bm.Name
			if hasChildren {
				name += " ▾"
			}
			ui.Text(c, name).FontSize(11).TextColor(t.Text).SingleLine().MaxWidth(135)
		})
	})

	if hasChildren {
		btn.PopoverMenuButton(func(m *ui.Menu) {
			renderBookmarkSubmenuItems(m, bm.Children, coord, onReload)
		}, ui.PopoverMenuButtonOptions{At: ui.AnchorBottomStart, Self: ui.AnchorTopStart})
	} else {
		btn.ContextMenu(func(m *ui.Menu) {
			if m.Item("在新标签页中打开").Chosen() {
				coord.Tabs.NewTab(bm.Path)
				if onReload != nil {
					onReload()
				}
			}
			if m.Item("在访达中显示").Chosen() {
				revealInFinderOrSelf(coord, bm.Path)
			}
			if m.Item("复制绝对路径").Chosen() {
				_ = macos.CopyPath(bm.Path, "posix")
			}
			m.Separator()
			if m.Item("移除书签").Chosen() {
				_ = coord.RemoveBookmark(bm.Path)
			}
		})
		if btn.Clicked() {
			coord.Tabs.Navigate(coord.Tabs.ActiveIndex, bm.Path)
			if onReload != nil {
				onReload()
			}
		}
	}
}

func renderWorkspaceDropdown(c *ui.Context, coord *app.Coordinator) {
	t := c.Theme()
	wsLabel := "工作区 ▾"
	if coord.ActiveWorkspaceID != "" {
		for _, ws := range coord.Workspaces.Get().Workspaces {
			if ws.ID == coord.ActiveWorkspaceID {
				wsLabel = ws.Name + " ▾"
				break
			}
		}
	}

	btn := ui.ButtonBase(c).Padding(2, 6).Radius(4).AlignItems(ui.Center).Shrink(0)
	if btn.Hovered() {
		btn.Background(t.Text.Alpha(0.08))
	}
	btn.Children(func() {
		ui.Text(c, wsLabel).FontSize(11).TextColor(t.TextMuted).SingleLine().MaxWidth(120)
	})
	btn.PopoverMenuButton(func(m *ui.Menu) {
		workspaces := coord.Workspaces.Get().Workspaces
		for _, w := range workspaces {
			ws := w
			if m.Item(ws.Name).Radio(ws.ID == coord.ActiveWorkspaceID).Chosen() {
				coord.RestoreWorkspace(ws)
			}
		}
		if len(workspaces) > 0 {
			m.Separator()
		}
		if m.Item("另存当前布局为工作区...").Chosen() {
			coord.ShowWorkspacesModal = true
			coord.WorkspaceDraftName = ""
		}
		if m.Item("管理工作区...").Chosen() {
			coord.ShowWorkspacesModal = true
		}
		m.Separator()
		if m.Item("从 Finder 导入常用位置...").Chosen() {
			coord.OpenFinderImport()
		}
	}, ui.PopoverMenuButtonOptions{At: ui.AnchorBottomStart, Self: ui.AnchorTopStart})
}

func renderBookmarkSubmenuItems(m *ui.Menu, items []config.BookmarkItem, coord *app.Coordinator, onReload func()) {
	for _, it := range items {
		item := it
		if len(item.Children) > 0 {
			m.Submenu(item.Name, func(sub *ui.Menu) {
				renderBookmarkSubmenuItems(sub, item.Children, coord, onReload)
			})
		} else {
			if m.Item(item.Name).Chosen() {
				coord.Tabs.Navigate(coord.Tabs.ActiveIndex, item.Path)
				if onReload != nil {
					onReload()
				}
			}
		}
	}
}
