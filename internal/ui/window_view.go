package ui

import (
	"fmt"
	"path/filepath"

	"github.com/bighaidao/clover-go/internal/app"
	"github.com/bighaidao/clover-go/internal/config"
	"github.com/bighaidao/clover-go/internal/domain"
	"github.com/bighaidao/clover-go/internal/fs"
	"github.com/egoist/mygo/ui"
)

// AppState coordinates the root window presentation state.
type AppState struct {
	Coord *app.Coordinator
}

// NewAppState initializes the presentation state wrapper.
func NewAppState(coord *app.Coordinator) *AppState {
	return &AppState{Coord: coord}
}

// View is the 120Hz immediate mode root frame callback.
func (s *AppState) View(c *ui.Context) {
	// Bridge the coordinator's transient toast to mygo's native floating toast.
	if s.Coord.ToastPending {
		s.Coord.ToastPending = false
		if s.Coord.ToastMessage != "" {
			c.Toast(s.Coord.ToastMessage)
		}
	}
	t := c.Theme()
	pal := Pal(t)
	activeTab := s.Coord.Tabs.ActiveTab()
	if activeTab == nil {
		return
	}

	// Tab switches cancel transient edit states (inline rename, path edit);
	// otherwise a rename started in tab A would commit against tab B's row.
	selectTab := func(index int) {
		s.Coord.CancelRename()
		s.Coord.IsEditing = false
		s.Coord.Tabs.ActiveIndex = index
		s.Coord.LoadCurrentTab(nil)
	}

	ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		// 0. macOS Draggable TitleBar (Centered title, fully draggable window handle)
		TitleBarView(c, "fclover")

		// 1. Clover Multi-tab bar (Header)
		TabBarView(c, s.Coord, selectTab, func(index int) {
			s.Coord.CancelRename()
			s.Coord.Tabs.CloseTab(index)
			s.Coord.LoadCurrentTab(nil)
		}, func() {
			s.Coord.Tabs.NewTab(activeTab.ActivePane().Path)
			s.Coord.LoadCurrentTab(nil)
		})

		// 2. Toolbar & Navigation (Connected Page Color)
		// onReload 用于刷新按钮、⌘R、导航与面包屑：都是用户主动动作，
		// 必须真正重读目录（ReloadCurrentTab），而不是走 watcher 的静默重载。
		ToolbarView(c, s.Coord, func() {
			s.Coord.ReloadCurrentTab()
		})

		// 3. Main Workspace Split: Sidebar + Content Area + Preview Pane
		sSplit := ui.Split(c, &s.Coord.SidebarWidth, func() {
			SidebarView(c, s.Coord, func(newPath string) {
				activePane := activeTab.ActivePane()
				activePane.Navigate(newPath)
				s.Coord.LoadPane(activePane, nil)
			})
		}, func() {
			ui.Row(c).Grow(1).FillHeight().AlignItems(ui.Stretch).Children(func() {
				// Content Area
				renderContentArea(c, s.Coord, activeTab)

				// Optional Preview Pane (⇧⌘P)
				if s.Coord.ShowPreview {
					ui.Box(c).Width(1).FillHeight().Background(pal.Line)
					PreviewPaneView(c, s.Coord, activeTab)
				}
			})
		}).Fill()
		if sSplit.Changed() {
			if s.Coord.SidebarWidth < 120 {
				s.Coord.SidebarWidth = 120
			} else if s.Coord.SidebarWidth > 320 {
				s.Coord.SidebarWidth = 320
			}
			p := s.Coord.Prefs.Get()
			p.SidebarWidth = s.Coord.SidebarWidth
			_ = s.Coord.Prefs.Save(p)
		}

		// 4. Status Bar
		renderStatusBar(c, s.Coord, activeTab)

		// 5. Modals (Batch rename, settings, dialogs)
		RenderModals(c, s.Coord, activeTab)
	})

	// Process keyboard shortcuts (Space for QuickLook, Cmd+T, Cmd+W, etc.)
	s.handleShortcuts(c, activeTab)

	// Consume the one-shot ⌘F focus request at end of frame.
	s.Coord.FocusFilter = false
}

func renderContentArea(c *ui.Context, coord *app.Coordinator, tab *app.TabState) {
	if !tab.IsDual || tab.RightPane == nil {
		renderSinglePane(c, coord, tab, &tab.PaneState, true, 0)
		return
	}

	dpSplit := ui.SplitPercent(c, &coord.DualPaneRatio, func() {
		renderSinglePane(c, coord, tab, &tab.PaneState, tab.ActiveIdx == 0, 0)
	}, func() {
		renderSinglePane(c, coord, tab, tab.RightPane, tab.ActiveIdx == 1, 1)
	}).Fill()
	if dpSplit.Changed() {
		p := coord.Prefs.Get()
		p.DualPaneRatio = coord.DualPaneRatio
		_ = coord.Prefs.Save(p)
	}
}

func renderSinglePane(c *ui.Context, coord *app.Coordinator, tab *app.TabState, pane *app.PaneState, isActive bool, paneIdx int) {
	t := c.Theme()
	pal := Pal(t)

	paneCol := ui.Column(c).Grow(1).FillHeight().Background(t.Background)
	BindFileDropTarget(c, paneCol, pane.Path, coord)

	if tab.IsDual {
		if isActive {
			paneCol.Border(1.5, t.Accent).Radius(4)
		} else {
			paneCol.Border(1, pal.Line.Alpha(0.6)).Radius(4)
		}
	}
	paneCol.Padding(6, 8)

	paneCol.Children(func() {
		if tab.IsDual {
			ui.Row(c).Height(24).Padding(0, 4).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
				title := filepath.Base(pane.Path)
				if pane.Path == "/" {
					title = "Macintosh HD"
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					txtCol := t.TextMuted
					if isActive {
						txtCol = t.Accent
					}
					ui.Icon(c, IcFolder).Size(12, 12).TextColor(txtCol).Shrink(0)
					ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(txtCol).SingleLine().Shrink(1).MinWidth(0).Tooltip(pane.Path)
				})

				btnClosePane := ui.ButtonBase(c).Padding(2).Radius(3).AlignItems(ui.Center).Shrink(0)
				btnClosePane.Children(func() {
					ui.Icon(c, IcClose).Size(10, 10).TextColor(t.TextMuted)
				})
				if btnClosePane.Clicked() {
					coord.ToggleDualPane()
				}
			})
			ui.Box(c).Height(1).Background(pal.Line.Alpha(0.6))
		}

		if fs.IsArchiveVirtualPath(pane.Path) {
			renderArchiveBanner(c, coord, pane)
		}

		if pane.IsLoading && len(pane.DisplayEntries) == 0 {
			EmptyCard(c, IcFolder, "正在加载文件夹...", "", nil)
			return
		}

		if pane.LoadErr != "" {
			actionBtn := func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					btnReveal := ui.ButtonBase(c).Padding(5, 12).Radius(6).Background(t.Accent).AlignItems(ui.Center)
					btnReveal.Children(func() {
						ui.Text(c, "在访达中打开").FontSize(11).TextColor(ui.Hex("#FFFFFF"))
					})
					if btnReveal.Clicked() {
						revealDirectoryOrSelf(coord, pane.Path)
					}

					btnRetry := ui.ButtonBase(c).Padding(5, 12).Radius(6).AlignItems(ui.Center)
					if btnRetry.Hovered() {
						btnRetry.Background(t.Text.Alpha(0.08))
					}
					btnRetry.Children(func() {
						ui.Text(c, "重试").FontSize(11).TextColor(t.Text)
					})
					if btnRetry.Clicked() {
						coord.LoadPane(pane, nil)
					}
				})
			}
			EmptyCard(c, IcFolder, "无权限访问此文件夹", pane.LoadErr, actionBtn)
			return
		}

		if len(pane.DisplayEntries) == 0 {
			msg := "此文件夹为空"
			sub := "可将文件拖拽至此，或按 ⇧⌘N 新建文件夹"
			if pane.FilterQuery != "" {
				msg = fmt.Sprintf("未找到匹配 “%s” 的项目", pane.FilterQuery)
				sub = "请检查搜索关键词或清除过滤条件"
			}
			EmptyCard(c, IcFolder, msg, sub, nil)
			return
		}

		if coord.ViewMode == config.ViewGrid {
			IconGridViewForPane(c, coord, pane)
		} else {
			FileListViewForPane(c, coord, pane)
		}
	})

	if paneCol.RowDoubleClicked() && coord.Prefs.Get().DoubleClickNav {
		idx := coord.Tabs.ActiveIndex
		if coord.Tabs.CanGoUp(idx) {
			coord.Tabs.GoUp(idx)
			coord.LoadCurrentTab(nil)
		}
	}

	if tab.IsDual && !isActive && paneCol.Clicked() {
		coord.SetActivePaneExplicit(tab, paneIdx)
	}
}

func renderStatusBar(c *ui.Context, coord *app.Coordinator, tab *app.TabState) {
	t := c.Theme()
	pal := Pal(t)
	pane := tab.ActivePane()

	ui.Row(c).Height(24).Background(pal.Sidebar).Padding(0, 12).AlignItems(ui.Center).Children(func() {
		if coord.ToastMessage != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Icon(c, IcInfo).Size(12, 12).TextColor(t.Accent).Shrink(0)
				ui.Text(c, coord.ToastMessage).FontSize(11).FontWeight(600).TextColor(t.Accent).SingleLine().Shrink(1).MinWidth(0)
			})
			ui.Box(c).Width(12).Height(1)
		}

		var statusText string
		if tab.IsDual && tab.RightPane != nil {
			leftTotal := len(tab.Left().DisplayEntries)
			rightTotal := len(tab.Right().DisplayEntries)
			leftTag := "左栏"
			rightTag := "右栏"
			if tab.ActiveIdx == 0 {
				leftTag = "左栏 (激活)"
			} else {
				rightTag = "右栏 (激活)"
			}
			statusText = fmt.Sprintf("%s: %d 项 · %s: %d 项", leftTag, leftTotal, rightTag, rightTotal)
			if pane.Chosen.Len() > 0 {
				statusText += fmt.Sprintf(" (已选 %d 项)", pane.Chosen.Len())
			}
		} else {
			total := len(pane.DisplayEntries)
			statusText = fmt.Sprintf("%d 个项目", total)
			if pane.Chosen.Len() > 0 {
				statusText = fmt.Sprintf("%d 个项目 (已选中 %d 项)", total, pane.Chosen.Len())
			} else if pane.SelectedIdx >= 0 && pane.SelectedIdx < total {
				sel := pane.DisplayEntries[pane.SelectedIdx]
				statusText = fmt.Sprintf("%d 个项目 (已选中: %s)", total, sel.Name)
			}
		}

		// Read cached volume stats with zero sync I/O in render frame
		if pane.VolumeFreeBytes > 0 {
			statusText += fmt.Sprintf(" · 可用空间 %s", domain.FormatSize(int64(pane.VolumeFreeBytes)))
		}

		ui.Text(c, statusText).FontSize(11).TextColor(t.TextMuted).Grow(1).SingleLine().Shrink(1).MinWidth(0).Tooltip(statusText)

		renderStatusControls(c, coord)
	})
}

func renderStatusControls(c *ui.Context, coord *app.Coordinator) {
	t := c.Theme()
	if coord.ViewMode == config.ViewGrid {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
			ui.Text(c, fmt.Sprintf("%dpx", coord.GridIconSize)).FontSize(10).TextColor(t.TextMuted)

			btnMinus := IconButton(c, IcMinus, "缩小图标", coord.GridIconSize <= 64)
			if btnMinus.Clicked() && coord.GridIconSize > 64 {
				coord.GridIconSize -= 16
				p := coord.Prefs.Get()
				p.GridIconSize = coord.GridIconSize
				_ = coord.Prefs.Save(p)
			}

			btnPlus := IconButton(c, IcPlus, "放大图标", coord.GridIconSize >= 128)
			if btnPlus.Clicked() && coord.GridIconSize < 128 {
				coord.GridIconSize += 16
				p := coord.Prefs.Get()
				p.GridIconSize = coord.GridIconSize
				_ = coord.Prefs.Save(p)
			}
		})
	}
}
