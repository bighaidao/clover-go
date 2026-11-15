package ui

import (
	"github.com/bighaidao/clover-go/internal/app"
	"github.com/bighaidao/clover-go/internal/config"
	"github.com/bighaidao/clover-go/internal/domain"
	"github.com/egoist/mygo/ui"
)

// SettingsModal renders the tabbed preferences dialog backed by PrefsManager.
func SettingsModal(c *ui.Context, coord *app.Coordinator) {
	t := c.Theme()
	pal := Pal(t)
	prefs := coord.Prefs.Get()

	apply := func(mutate func(*config.Preferences)) {
		p := coord.Prefs.Get()
		mutate(&p)
		_ = coord.Prefs.Save(p)
		coord.Invalidate()
	}

	tab := coord.SettingsActiveTab
	if tab == "" {
		tab = "general"
	}

	backdrop := ui.Box(c).Fill().Background(ui.Hex("#000000").Alpha(0.45)).AlignItems(ui.Center).Justify(ui.Center)
	backdrop.Children(func() {
		dialog := ui.Column(c).Width(520).Radius(10).Background(t.Surface).Padding(16, 20).Gap(12)
		dialog.Shadow(0, 8, 24, 0, ui.Hex("#000000").Alpha(0.25))

		dialog.Children(func() {
			// Title Header
			ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, IcSettings).Size(16, 16).TextColor(t.Accent)
					ui.Text(c, "偏好设置").FontSize(14).FontWeight(600).TextColor(t.Text)
				})

				btnClose := ui.ButtonBase(c).Label("关闭").Padding(2).Radius(3).AlignItems(ui.Center)
				btnClose.Children(func() {
					ui.Icon(c, IcClose).Size(12, 12).TextColor(t.TextMuted)
				})
				if btnClose.Clicked() {
					coord.CloseSettings()
				}
			})

			ui.Box(c).Height(1).Background(pal.Line)

			// Tab Selector Row
			renderSettingsTabRow(c, coord, tab)

			ui.Box(c).Height(1).Background(pal.Line)

			// Tab Body
			ui.Column(c).MinHeight(220).Gap(10).Children(func() {
				switch tab {
				case "general":
					renderGeneralTab(c, coord, prefs, apply)
				case "appearance":
					renderAppearanceTab(c, coord, prefs, apply)
				case "dnd":
					renderDnDTab(c, coord, prefs, apply)
				case "shortcuts":
					renderShortcutsTab(c)
				case "about":
					renderAboutTab(c, coord)
				default:
					renderGeneralTab(c, coord, prefs, apply)
				}
			})

			ui.Box(c).Height(1).Background(pal.Line)

			// Footer Close
			ui.Row(c).Justify(ui.End).Children(func() {
				btnOk := ui.ButtonBase(c).Padding(5, 16).Radius(6).Background(t.Accent).AlignItems(ui.Center)
				btnOk.Children(func() {
					ui.Text(c, "完成").FontSize(12).FontWeight(600).TextColor(ui.Hex("#FFFFFF"))
				})
				if btnOk.Clicked() {
					coord.CloseSettings()
				}
			})
		})
	})
}

// AboutModal renders the standalone "关于 CloverGo" dialog.
func AboutModal(c *ui.Context, coord *app.Coordinator) {
	t := c.Theme()
	pal := Pal(t)

	backdrop := ui.Box(c).Fill().Background(ui.Hex("#000000").Alpha(0.45)).AlignItems(ui.Center).Justify(ui.Center)
	backdrop.Children(func() {
		dialog := ui.Column(c).Width(380).Radius(10).Background(t.Surface).Padding(20).Gap(14).AlignItems(ui.Center)
		dialog.Shadow(0, 8, 24, 0, ui.Hex("#000000").Alpha(0.25))

		dialog.Children(func() {
			ui.Icon(c, IcFolder).Size(48, 48).TextColor(t.Accent)
			ui.Text(c, "CloverGo").FontSize(18).FontWeight(700).TextColor(t.Text)
			ui.Text(c, "版本 1.1.0 (Pure Go + MyGo Metal GPU)").FontSize(11).TextColor(t.TextMuted)

			ui.Box(c).Height(1).FillWidth().Background(pal.Line)

			ui.Text(c, "极速 macOS 原生多标签与双栏文件管理器").FontSize(12).TextColor(t.Text)
			ui.Text(c, "全场景拖拽 · 零内存占用 · 120Hz 视网膜渲染").FontSize(11).TextColor(t.TextMuted)

			btnCheck := ui.ButtonBase(c).Padding(6, 14).Radius(6).Background(pal.Hover).AlignItems(ui.Center)
			btnCheck.Children(func() {
				ui.Text(c, "检查更新… (⌥⌘U)").FontSize(11).TextColor(t.Text)
			})
			if btnCheck.Clicked() {
				coord.Toast("当前已是最新版本 (v1.1.0)")
			}

			ui.Box(c).Height(1).FillWidth().Background(pal.Line)

			btnClose := ui.ButtonBase(c).Padding(6, 20).Radius(6).Background(t.Accent).AlignItems(ui.Center)
			btnClose.Children(func() {
				ui.Text(c, "确定").FontSize(12).FontWeight(600).TextColor(ui.Hex("#FFFFFF"))
			})
			if btnClose.Clicked() {
				coord.ShowAboutModal = false
			}
		})
	})
}

func renderSettingsTabRow(c *ui.Context, coord *app.Coordinator, activeTab string) {
	t := c.Theme()
	tabs := []struct {
		ID    string
		Label string
	}{
		{"general", "通用"},
		{"appearance", "外观"},
		{"dnd", "拖拽与分栏"},
		{"shortcuts", "快捷键"},
		{"about", "关于"},
	}

	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		for _, item := range tabs {
			it := item
			isActive := activeTab == it.ID
			btn := ui.ButtonBase(c).Padding(4, 12).Radius(5).AlignItems(ui.Center)
			if isActive {
				btn.Background(t.Accent)
			} else if btn.Hovered() {
				btn.Background(Pal(t).Hover)
			}
			btn.Children(func() {
				txtCol := t.Text
				if isActive {
					txtCol = ui.Hex("#FFFFFF")
				}
				ui.Text(c, it.Label).FontSize(11).FontWeight(func() int {
					if isActive {
						return 600
					}
					return 400
				}()).TextColor(txtCol)
			})
			if btn.Clicked() {
				coord.SettingsActiveTab = it.ID
			}
		}
	})
}

func renderGeneralTab(c *ui.Context, coord *app.Coordinator, prefs config.Preferences, apply func(func(*config.Preferences))) {
	renderCheckRow(c, "启动时自动恢复上次会话标签页", prefs.RestoreSession, func(v bool) {
		apply(func(p *config.Preferences) { p.RestoreSession = v })
	})
	renderCheckRow(c, "双击窗格空白区域返回上一级文件夹", prefs.DoubleClickNav, func(v bool) {
		apply(func(p *config.Preferences) { p.DoubleClickNav = v })
	})
	renderCheckRow(c, "显示隐藏文件 (⇧⌘.)", prefs.ShowHidden, func(v bool) {
		apply(func(p *config.Preferences) { p.ShowHidden = v })
		coord.LoadCurrentTab(nil)
	})
	renderCheckRow(c, "显示书签栏", prefs.ShowBookmark, func(v bool) {
		apply(func(p *config.Preferences) { p.ShowBookmark = v })
	})
	renderCheckRow(c, "接管 Finder(需授予自动化权限)", prefs.TakeoverFinder, func(v bool) {
		apply(func(p *config.Preferences) { p.TakeoverFinder = v })
		coord.SetFinderTakeover(v)
	})
	renderCheckRow(c, "开机时自动启动 CloverGo", prefs.LaunchAtLogin, func(v bool) {
		apply(func(p *config.Preferences) { p.LaunchAtLogin = v })
		coord.SetLaunchAtLogin(v)
	})

	// 默认排序：持久化到偏好，并立即重排当前与未来所有标签页与窗格。
	renderSegmentedRow(c, "默认排序",
		[]string{"create_time", "mod_time", "name", "size", "kind"},
		[]string{"创建时间", "修改时间", "名称", "大小", "类型"},
		prefs.SortField, func(v string) {
			apply(func(p *config.Preferences) { p.SortField = v })
			applyDefaultSort(coord, coord.Prefs.Get())
		})
	renderSegmentedRow(c, "排序方向",
		[]string{"desc", "asc"},
		[]string{"最新在前", "最旧在前"},
		prefs.SortDirection, func(v string) {
			apply(func(p *config.Preferences) { p.SortDirection = v })
			applyDefaultSort(coord, coord.Prefs.Get())
		})
}

// applyDefaultSort refreshes manager-side default ordering and immediately
// updates and resorts all active and background panes.
func applyDefaultSort(coord *app.Coordinator, p config.Preferences) {
	coord.SetDefaultSortAndResort(domain.SortField(p.SortField), domain.SortDirection(p.SortDirection))
}

func renderAppearanceTab(c *ui.Context, coord *app.Coordinator, prefs config.Preferences, apply func(func(*config.Preferences))) {
	renderSegmentedRow(c, "外观主题", []string{"system", "light", "dark"}, []string{"跟随系统", "浅色", "深色"}, prefs.Appearance, func(v string) {
		apply(func(p *config.Preferences) { p.Appearance = v })
		ApplyAppearance(v)
	})
	renderCheckRow(c, "原生毛玻璃背景 (Vibrancy，重启生效)", prefs.Vibrancy, func(v bool) {
		apply(func(p *config.Preferences) { p.Vibrancy = v })
	})
	renderSegmentedRow(c, "默认浏览视图", []string{"list", "grid"}, []string{"列表", "网格"}, string(prefs.DefaultView), func(v string) {
		apply(func(p *config.Preferences) { p.DefaultView = config.ViewMode(v) })
	})
	renderIconSizeRow(c, coord, prefs, apply)
}

func renderDnDTab(c *ui.Context, coord *app.Coordinator, prefs config.Preferences, apply func(func(*config.Preferences))) {
	t := c.Theme()
	renderSegmentedRow(c, "跨栏拖拽默认操作", []string{"move", "copy"}, []string{"移动 (Move)", "复制 (Copy)"}, prefs.DragAction, func(v string) {
		apply(func(p *config.Preferences) { p.DragAction = v })
	})
	renderSegmentedRow(c, "同名文件冲突策略", []string{"ask", "rename", "overwrite"}, []string{"弹窗询问", "保留两者", "覆盖替换"}, prefs.ConflictAction, func(v string) {
		apply(func(p *config.Preferences) { p.ConflictAction = v })
	})

	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "分栏布局重置").FontSize(12).TextColor(t.Text).Width(180)
		btnReset := ui.ButtonBase(c).Padding(4, 12).Radius(5).Background(Pal(t).Hover).AlignItems(ui.Center)
		btnReset.Children(func() {
			ui.Text(c, "重置分栏比例与各列宽度").FontSize(11).TextColor(t.Text)
		})
		if btnReset.Clicked() {
			coord.SidebarWidth = 155
			coord.DualPaneRatio = 50
			coord.PreviewWidth = 250
			coord.ColWidthName = 220
			coord.ColWidthCreateTime = 115
			coord.ColWidthModTime = 115
			coord.ColWidthSize = 70
			coord.ColWidthKind = 90
			coord.SaveColWidths()
			apply(func(p *config.Preferences) {
				p.SidebarWidth = 155
				p.DualPaneRatio = 50
				p.PreviewWidth = 250
				p.ColWidthName = 220
				p.ColWidthCreateTime = 115
				p.ColWidthModTime = 115
				p.ColWidthSize = 70
				p.ColWidthKind = 90
			})
			coord.Toast("已重置布局与列宽为默认尺寸")
		}
	})
}

func renderShortcutsTab(c *ui.Context) {
	t := c.Theme()
	pal := Pal(t)
	shortcuts := []struct {
		Key  string
		Desc string
	}{
		{"⌘T / ⌘W", "新建标签页 / 关闭当前标签页"},
		{"⌘O / ⌘↓", "打开选中的项目 (文件夹/文件/压缩包)"},
		{"⌘↑", "返回上一层文件夹"},
		{"⌘1 / ⌘2", "切换为列表视图 / 图标网格视图"},
		{"⌘B", "开启/关闭双栏对齐模式"},
		{"⌘A / ⌘D", "全选所有项目 / 创建所选项副本"},
		{"⌘I / ⇧⌘P", "显示简介 / 开启超级预览面板"},
		{"⌥⌘C", "复制选中项目完整绝对路径"},
		{"Space", "macOS 原生快速查看 (Quick Look)"},
		{"⇧⌘N", "新建文件夹"},
		{"⇧⌘.", "显示/隐藏以点开头的隐藏文件"},
	}

	ui.Scroll(c).MaxHeight(200).Children(func() {
		ui.Column(c).Gap(4).Children(func() {
			for _, sc := range shortcuts {
				s := sc
				ui.Row(c).Height(22).Padding(0, 6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, s.Key).FontSize(11).FontWeight(600).TextColor(t.Accent).Width(130)
					ui.Text(c, s.Desc).FontSize(11).TextColor(t.TextMuted)
				})
				ui.Box(c).Height(1).Background(pal.Line.Alpha(0.4))
			}
		})
	})
}

func renderAboutTab(c *ui.Context, coord *app.Coordinator) {
	t := c.Theme()
	ui.Column(c).Gap(8).AlignItems(ui.Center).Padding(10).Children(func() {
		ui.Icon(c, IcFolder).Size(36, 36).TextColor(t.Accent)
		ui.Text(c, "CloverGo").FontSize(16).FontWeight(700).TextColor(t.Text)
		ui.Text(c, "版本 1.1.0 (Pure Go + MyGo Metal GPU)").FontSize(11).TextColor(t.TextMuted)
		ui.Text(c, "极速原生双栏文件管理器 · 全场景跨栏拖拽转移").FontSize(11).TextColor(t.Text)

		btnCheck := ui.ButtonBase(c).Padding(5, 14).Radius(6).Background(Pal(t).Hover).AlignItems(ui.Center)
		btnCheck.Children(func() {
			ui.Text(c, "检查更新… (⌥⌘U)").FontSize(11).TextColor(t.Text)
		})
		if btnCheck.Clicked() {
			coord.Toast("当前已是最新版本 (v1.1.0)")
		}
	})
}
