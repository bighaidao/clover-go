package ui

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/bighaidao/clover-go/internal/app"
	"github.com/bighaidao/clover-go/internal/config"
	"github.com/bighaidao/clover-go/internal/domain"
	"github.com/bighaidao/clover-go/internal/macos"
	"github.com/egoist/mygo/ui"
)

type sidebarItem struct {
	Name string
	Path string
	Icon *ui.SVG
}

var (
	sidebarFoldsMu sync.Mutex
	sidebarFolds   = map[string]bool{}
)

func isSidebarFolded(section string) bool {
	sidebarFoldsMu.Lock()
	defer sidebarFoldsMu.Unlock()
	return sidebarFolds[section]
}

func toggleSidebarFold(section string) {
	sidebarFoldsMu.Lock()
	defer sidebarFoldsMu.Unlock()
	sidebarFolds[section] = !sidebarFolds[section]
}

// SidebarView renders the frosted Vibrancy sidebar with system favorites, disks, workspaces, and tags.
func SidebarView(c *ui.Context, coord *app.Coordinator, onNavigate func(string)) {
	t := c.Theme()
	pal := Pal(t)
	activeTab := coord.Tabs.ActiveTab()
	currentPath := ""
	if activeTab != nil {
		currentPath = activeTab.ActivePane().Path
	}

	ui.Scroll(c).FillHeight().FillWidth().Background(pal.Sidebar).Padding(12, 8).Children(func() {
		ui.Column(c).Gap(14).Children(func() {
			// 1. Common Places Group (常用位置)
			renderSidebarSection(c, coord, "常用位置", getCommonPlaces(), currentPath, onNavigate)

			// 2. User Bookmarks Group (收藏)
			renderBookmarksSection(c, coord, currentPath, onNavigate)

			// 3. Workspaces Group
			renderWorkspacesSection(c, coord)

			// 4. Color Tags Group
			renderColorTagsSection(c, coord)
		})
	})
}

func renderBookmarksSection(c *ui.Context, coord *app.Coordinator, currentPath string, onNavigate func(string)) {
	t := c.Theme()
	if coord.Workspaces == nil {
		return
	}
	wsData := coord.Workspaces.Get()
	items := wsData.Bookmarks

	secCol := ui.Column(c).Gap(2)
	handleBookmarksDrop(secCol, coord)

	folded := isSidebarFolded("收藏")
	arrow := "▾"
	if folded {
		arrow = "▸"
	}

	secCol.Children(func() {
		btnHeader := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Padding(2, 6).Radius(4).AlignItems(ui.Center).Sticky().Top(0).Background(Pal(t).Sidebar).ZIndex(1)
		btnHeader.Children(func() {
			ui.Text(c, "收藏 "+arrow).FontSize(11).FontWeight(600).TextColor(t.TextMuted.Alpha(0.7))
		})
		if btnHeader.Clicked() {
			toggleSidebarFold("收藏")
		}

		if folded {
			return
		}

		if len(items) == 0 {
			ui.Text(c, "拖拽文件夹至此收藏").FontSize(11).TextColor(t.TextMuted.Alpha(0.5)).Padding(2, 8)
			return
		}

		for _, item := range items {
			renderBookmarkItem(c, coord, item, currentPath, onNavigate)
		}
	})
}

func handleBookmarksDrop(secCol ui.Element, coord *app.Coordinator) {
	if payload, dropped := ui.Drop[app.FileDragPayload](secCol); dropped {
		for _, p := range payload.Paths {
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				_ = coord.AddBookmark(p, filepath.Base(p))
				coord.Toast("已添加到收藏: " + filepath.Base(p))
			}
		}
	} else if extFiles := secCol.DroppedFiles(); len(extFiles) > 0 {
		for _, p := range extFiles {
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				_ = coord.AddBookmark(p, filepath.Base(p))
				coord.Toast("已添加到收藏: " + filepath.Base(p))
			}
		}
	}
}

func renderBookmarkItem(c *ui.Context, coord *app.Coordinator, it config.BookmarkItem, currentPath string, onNavigate func(string)) {
	t := c.Theme()
	isSelected := domain.NormalizePath(currentPath) == domain.NormalizePath(it.Path)

	btn := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Padding(5, 8).Radius(6).Gap(8).AlignItems(ui.Center).Tooltip(it.Name + "\n" + it.Path)
	isOver := BindFileDropTarget(c, btn, it.Path, coord)

	txtCol := t.Text
	if isOver {
		btn.Border(1.5, t.Accent).Background(Pal(t).Hover)
		txtCol = t.Accent
	} else if isSelected {
		btn.Background(t.Selection)
		txtCol = t.Accent
	} else if btn.Hovered() {
		btn.Background(Pal(t).Hover)
	}

	btn.Children(func() {
		ui.Icon(c, IcFolder).Size(14, 14).TextColor(txtCol).Shrink(0)
		ui.Text(c, it.Name).FontSize(12).TextColor(txtCol).FontWeight(func() int {
			if isSelected {
				return 500
			}
			return 400
		}()).SingleLine().Grow(1).Shrink(1).MinWidth(0)
	})

	btn.ContextMenu(func(m *ui.Menu) {
		if m.Item("在新标签页中打开").Chosen() {
			coord.Tabs.NewTab(it.Path)
			coord.LoadCurrentTab(nil)
		}
		if m.Item("在访达中显示").Chosen() {
			revealInFinderOrSelf(coord, it.Path)
		}
		if m.Item("复制路径").Chosen() {
			_ = macos.CopyPath(it.Path, "posix")
		}
		m.Separator()
		if m.Item("从收藏中移除").Chosen() {
			_ = coord.RemoveBookmark(it.Path)
		}
	})

	if btn.Clicked() && onNavigate != nil {
		onNavigate(it.Path)
	}
}

func renderSidebarSection(c *ui.Context, coord *app.Coordinator, header string, items []sidebarItem, currentPath string, onNavigate func(string)) {
	t := c.Theme()
	folded := isSidebarFolded(header)
	arrow := "▾"
	if folded {
		arrow = "▸"
	}

	ui.Column(c).Gap(2).Children(func() {
		btnHeader := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Padding(2, 6).Radius(4).AlignItems(ui.Center).Sticky().Top(0).Background(Pal(t).Sidebar).ZIndex(1)
		btnHeader.Children(func() {
			ui.Text(c, header+" "+arrow).FontSize(11).FontWeight(600).TextColor(t.TextMuted.Alpha(0.7))
		})
		if btnHeader.Clicked() {
			toggleSidebarFold(header)
		}

		if folded {
			return
		}

		for _, item := range items {
			renderSidebarItem(c, coord, item, currentPath, onNavigate)
		}
	})
}

func renderSidebarItem(c *ui.Context, coord *app.Coordinator, it sidebarItem, currentPath string, onNavigate func(string)) {
	t := c.Theme()
	isSelected := domain.NormalizePath(currentPath) == domain.NormalizePath(it.Path)

	btn := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Padding(5, 8).Radius(6).Gap(8).AlignItems(ui.Center).Tooltip(it.Name + "\n" + it.Path)
	isOver := BindFileDropTarget(c, btn, it.Path, coord)

	txtCol := t.Text
	if isOver {
		btn.Border(1.5, t.Accent).Background(Pal(t).Hover)
		txtCol = t.Accent
	} else if isSelected {
		btn.Background(t.Selection)
		txtCol = t.Accent
	} else if btn.Hovered() {
		btn.Background(Pal(t).Hover)
	}

	btn.Children(func() {
		if it.Icon != nil {
			ui.Icon(c, it.Icon).Size(14, 14).TextColor(txtCol).Shrink(0)
		}
		ui.Text(c, it.Name).FontSize(12).TextColor(txtCol).FontWeight(func() int {
			if isSelected {
				return 500
			}
			return 400
		}()).SingleLine().Grow(1).Shrink(1).MinWidth(0)
	})

	btn.ContextMenu(func(m *ui.Menu) {
		if m.Item("在新标签页中打开").Chosen() {
			coord.Tabs.NewTab(it.Path)
			coord.LoadCurrentTab(nil)
		}
	})

	if btn.Clicked() && onNavigate != nil {
		onNavigate(it.Path)
	}
}

func renderWorkspacesSection(c *ui.Context, coord *app.Coordinator) {
	t := c.Theme()
	wsData := coord.Workspaces.Get()
	if len(wsData.Workspaces) == 0 {
		return
	}

	ui.Column(c).Gap(2).Children(func() {
		ui.Text(c, "常用工作区").FontSize(11).FontWeight(600).TextColor(t.TextMuted.Alpha(0.7)).Padding(2, 6).Sticky().Top(0).Background(Pal(t).Sidebar).ZIndex(1)

		for _, w := range wsData.Workspaces {
			ws := w
			btn := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Padding(5, 8).Radius(6).Gap(8).AlignItems(ui.Center).Tooltip(ws.Name)
			if btn.Hovered() {
				btn.Background(Pal(t).Hover)
			}
			btn.Children(func() {
				ui.Icon(c, IcFolder).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
				ui.Text(c, ws.Name).FontSize(12).TextColor(t.Text).SingleLine().Grow(1).Shrink(1).MinWidth(0)
			})
			if btn.Clicked() {
				coord.RestoreWorkspace(ws)
			}
		}
	})
}

func renderColorTagsSection(c *ui.Context, coord *app.Coordinator) {
	t := c.Theme()
	tags := []struct {
		Idx   int
		Name  string
		Color ui.Color
	}{
		{macos.TagRed, "红色", TintRed.C(t)},
		{macos.TagOrange, "橙色", TintOrange.C(t)},
		{macos.TagYellow, "黄色", ui.Hex("#FFCC00")},
		{macos.TagGreen, "绿色", TintGreen.C(t)},
		{macos.TagBlue, "蓝色", TintBlue.C(t)},
		{macos.TagPurple, "紫色", TintPurple.C(t)},
		{macos.TagGray, "灰色", TintGray.C(t)},
	}

	ui.Column(c).Gap(2).Children(func() {
		ui.Text(c, "标签筛选").FontSize(11).FontWeight(600).TextColor(t.TextMuted.Alpha(0.7)).Padding(2, 6).Sticky().Top(0).Background(Pal(t).Sidebar).ZIndex(1)

		for _, tg := range tags {
			tagCopy := tg
			isSelected := coord.FilterTag == tagCopy.Idx
			btn := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Padding(4, 8).Radius(6).Gap(8).AlignItems(ui.Center).Tooltip(tagCopy.Name)
			if isSelected {
				btn.Background(t.Selection)
			} else if btn.Hovered() {
				btn.Background(Pal(t).Hover)
			}
			btn.Children(func() {
				ColorDot(c, tagCopy.Color, 8)
				txtCol := t.Text
				if isSelected {
					txtCol = t.Accent
				}
				ui.Text(c, tagCopy.Name).FontSize(12).TextColor(txtCol).SingleLine().Grow(1).Shrink(1).MinWidth(0)
			})
			if btn.Clicked() {
				if coord.FilterTag == tagCopy.Idx {
					coord.FilterTag = 0
				} else {
					coord.FilterTag = tagCopy.Idx
				}
				activeTab := coord.Tabs.ActiveTab()
				coord.ApplyFilter(activeTab)
				coord.Invalidate()
			}
		}
	})
}

func getCommonPlaces() []sidebarItem {
	home, _ := os.UserHomeDir()
	return []sidebarItem{
		{Name: "个人", Path: home, Icon: IcHome},
		{Name: "桌面", Path: filepath.Join(home, "Desktop"), Icon: IcDesktop},
		{Name: "文稿", Path: filepath.Join(home, "Documents"), Icon: IcDocument},
		{Name: "下载", Path: filepath.Join(home, "Downloads"), Icon: IcDownload},
		{Name: "应用程序", Path: "/Applications", Icon: IcApp},
	}
}
