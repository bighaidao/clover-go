package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bighaidao/clover-go/internal/app"
	"github.com/bighaidao/clover-go/internal/domain"
	"github.com/bighaidao/clover-go/internal/fs"
	"github.com/bighaidao/clover-go/internal/macos"
	"github.com/egoist/mygo/ui"
)

// ShowFileContextMenu renders the comprehensive right-click context menu for
// a file item; the sections live in the context*Actions helpers below.
func ShowFileContextMenu(m *ui.Menu, c *ui.Context, coord *app.Coordinator, pane *app.PaneState, item domain.FileInfo, i int) {
	m.MaxWidth(360)
	if fs.IsArchiveVirtualPath(pane.Path) {
		showArchiveItemContextMenu(m, coord, pane, item)
		return
	}
	if activeTab := coord.Tabs.ActiveTab(); activeTab != nil && activeTab.IsDual {
		if m.Item("复制到对侧分栏").Chosen() {
			coord.CopySelectedToOppositePane(activeTab)
		}
		if m.Item("移动到对侧分栏").Chosen() {
			coord.MoveSelectedToOppositePane(activeTab)
		}
		m.Separator()
	}
	contextOpenActions(m, c, coord, pane, item)
	m.Separator()
	contextSystemActions(m, coord, item)
	m.Separator()
	contextClipboardActions(m, coord, pane, item)
	m.Separator()
	contextRenameActions(m, coord, pane, i)
	m.Separator()
	contextTagTrashActions(m, c, coord, pane, item)
}

// showArchiveItemContextMenu provides a safe read-only context menu inside virtual archives.
func showArchiveItemContextMenu(m *ui.Menu, coord *app.Coordinator, pane *app.PaneState, item domain.FileInfo) {
	if m.Item("快速查看").Trailing("空格").Chosen() {
		if !item.IsDir {
			if tmpPath, err := fs.ExtractArchiveFileToTemp(item.Path); err == nil {
				macos.ToggleQuickLook(tmpPath)
			}
		}
	}
	if m.Item("显示/隐藏预览栏").Trailing("⌘I").Chosen() {
		coord.TogglePreview()
		if coord.ShowPreview {
			coord.LoadPreview(item, nil)
		}
	}
	if m.Item("打开").Chosen() {
		openItem(coord, pane, item)
	}
	m.Separator()
	if m.Item("提取选中项到同级目录...").Chosen() {
		coord.ExtractVirtualSelected(pane)
	}
	if m.Item("复制虚拟路径").Chosen() {
		_ = macos.CopyPath(item.Path, "posix")
	}
	m.Separator()
	if m.Item("在访达中显示压缩包").Chosen() {
		archivePath, _, _ := fs.ParseArchiveVirtualPath(pane.Path)
		revealInFinderOrSelf(coord, archivePath)
	}
}

// contextOpenActions renders Quick Look / preview toggle / open / new tab / package-content entries.
func contextOpenActions(m *ui.Menu, c *ui.Context, coord *app.Coordinator, pane *app.PaneState, item domain.FileInfo) {
	// 1. 快速查看 (Native macOS QLPreviewPanel)
	if m.Item("快速查看").Trailing("空格").Chosen() {
		macos.ToggleQuickLook(item.Path)
	}

	// 2. 预览栏显隐切换 (Inspector Pane)
	if m.Item("显示/隐藏预览栏").Trailing("⌘I").Chosen() {
		coord.TogglePreview()
		if coord.ShowPreview {
			coord.LoadPreview(item, nil)
		}
	}

	// 3. 压缩包就地免解压浏览
	if fs.IsArchiveFile(item.Path) && !fs.IsArchiveVirtualPath(item.Path) {
		if m.Item("浏览压缩包内容 (就地免解压)").Chosen() {
			vPath := fs.FormatArchiveVirtualPath(item.Path, "")
			coord.Tabs.NavigatePane(coord.Tabs.ActiveIndex, pane, vPath)
			coord.LoadCurrentTab(nil)
		}
	}

	// 4. 打开
	if m.Item("打开").Chosen() {
		openItem(coord, pane, item)
	}

	// 5. PDF 瑞士军刀微工具 / 图片水印
	contextToolActions(m, coord, pane, item)

	// 6. 在新标签页中打开
	targetTabPath := item.Path
	if !item.IsDir || item.IsAppBundle {
		targetTabPath = filepath.Dir(item.Path)
	}
	if m.Item("在新标签页中打开").Chosen() {
		coord.Tabs.NewTab(targetTabPath)
		coord.LoadCurrentTab(nil)
	}

	if item.IsAppBundle {
		if m.Item("显示包内容").Chosen() {
			coord.Tabs.NavigatePane(coord.Tabs.ActiveIndex, pane, item.Path)
			coord.LoadCurrentTab(nil)
		}
	}
}

// contextToolActions mounts the PDF micro-tool and image watermark entries,
// gated by the actual PDF/image count of the current selection.
func contextToolActions(m *ui.Menu, coord *app.Coordinator, pane *app.PaneState, item domain.FileInfo) {
	if strings.EqualFold(filepath.Ext(item.Name), ".pdf") {
		pdfCount := countSelectionPDFs(coord, pane)
		if pdfCount > 1 {
			label := fmt.Sprintf("合并所选 PDF 文件 (%d个)...", pdfCount)
			if m.Item(label).Chosen() {
				coord.OpenPDFTools(pane, domain.PDFModeMerge)
			}
		} else {
			if m.Item("PDF 瑞士军刀 (提取/拆分/转图)...").Chosen() {
				coord.OpenPDFTools(pane, domain.PDFModeExtract)
			}
		}
	}

	if !item.IsDir && macos.IsImageFile(item.Path) {
		if m.Item("添加文字水印...").Chosen() {
			coord.OpenWatermark(pane)
		}
	}
}

// countSelectionPDFs counts how many of the pane's selected items are PDFs,
// so the merge menu label matches what OpenPDFTools will actually process.
func countSelectionPDFs(coord *app.Coordinator, pane *app.PaneState) int {
	count := 0
	for _, it := range coord.SelectedItems(pane) {
		if strings.EqualFold(filepath.Ext(it.Name), ".pdf") {
			count++
		}
	}
	return count
}

// contextSystemActions renders bookmark, Finder-reveal, path-copy, terminal
// and editor entries.
func contextSystemActions(m *ui.Menu, coord *app.Coordinator, item domain.FileInfo) {
	// 添加到/移出收藏 (书签)
	if item.IsDir && !item.IsAppBundle {
		if coord.IsBookmarked(item.Path) {
			if m.Item("从收藏中移除").Chosen() {
				_ = coord.RemoveBookmark(item.Path)
			}
		} else {
			if m.Item("添加到收藏 / 书签").Chosen() {
				_ = coord.AddBookmark(item.Path, item.Name)
			}
		}
	}

	if m.Item("在访达中显示").Chosen() {
		revealInFinderOrSelf(coord, item.Path)
	}

	if m.Item("复制绝对路径").Trailing("⌥⌘C").Chosen() {
		_ = macos.CopyPath(item.Path, "posix")
	}

	if m.Item("在终端中打开").Chosen() {
		_ = macos.OpenInTerminal(item.Path)
	}
	if m.Item("在编辑器中打开").Chosen() {
		_ = macos.OpenInEditor(item.Path)
	}
}

// contextClipboardActions renders cut/copy plus zip compress/extract entries.
func contextClipboardActions(m *ui.Menu, coord *app.Coordinator, pane *app.PaneState, item domain.FileInfo) {
	if m.Item("剪切").Trailing("⌘X").Chosen() {
		coord.CutSelected(pane)
	}
	if m.Item("复制").Trailing("⌘C").Chosen() {
		coord.CopySelected(pane)
	}

	if strings.EqualFold(filepath.Ext(item.Name), ".zip") {
		if m.Item("解压到当前文件夹").Chosen() {
			coord.ExtractSelected(pane)
		}
	} else {
		if m.Item("压缩为 Zip 归档").Chosen() {
			coord.CompressSelected(pane)
		}
	}
}

// contextRenameActions renders rename, batch rename and duplicate entries.
func contextRenameActions(m *ui.Menu, coord *app.Coordinator, pane *app.PaneState, i int) {
	if m.Item("重命名").Chosen() {
		coord.StartRename(pane, i)
	}
	if len(coord.SelectedItems(pane)) > 1 {
		if m.Item("批量重命名").Trailing("⇧⌘R").Chosen() {
			coord.OpenBatchRename(pane)
		}
	}
	if m.Item("创建副本").Chosen() {
		coord.DuplicateSelected(pane)
	}
}

// ShowEmptyAreaContextMenu renders the context menu for background blank area.
func ShowEmptyAreaContextMenu(m *ui.Menu, coord *app.Coordinator, pane *app.PaneState) {
	m.MaxWidth(320)
	currentPath := pane.Path
	if fs.IsArchiveVirtualPath(currentPath) {
		showArchiveEmptyAreaMenu(m, coord, pane)
		return
	}

	if m.Item("新建文件夹").Trailing("⇧⌘N").Chosen() {
		_, _ = coord.CreateNewFolder(pane, "未命名文件夹")
	}

	if m.Item("全选").Trailing("⌘A").Chosen() {
		coord.SelectAll(pane)
	}
	if m.Item("刷新").Trailing("⌘R").Chosen() {
		coord.LoadPane(pane, nil)
	}

	if coord.IsBookmarked(currentPath) {
		if m.Item("从收藏中移除当前文件夹").Chosen() {
			_ = coord.RemoveBookmark(currentPath)
		}
	} else {
		title := filepath.Base(currentPath)
		if m.Item("将当前文件夹添加到收藏").Chosen() {
			_ = coord.AddBookmark(currentPath, title)
		}
	}

	if len(coord.Clipboard.Paths) > 0 {
		if m.Item("粘贴项目").Trailing("⌘V").Chosen() {
			coord.Paste(pane)
		}
	}

	m.Separator()

	if m.Item("复制当前目录绝对路径").Chosen() {
		_ = macos.CopyPath(currentPath, "posix")
	}

	if m.Item("在此处打开终端").Chosen() {
		_ = macos.OpenInTerminal(currentPath)
	}

	if m.Item("从 Finder 导入常用位置...").Chosen() {
		coord.OpenFinderImport()
	}
}

// showArchiveEmptyAreaMenu provides blank area options when inside a virtual archive.
func showArchiveEmptyAreaMenu(m *ui.Menu, coord *app.Coordinator, pane *app.PaneState) {
	if m.Item("全部解压到当前目录所在文件夹").Chosen() {
		coord.ExtractEntireArchive(pane)
	}
	if m.Item("退出压缩包").Chosen() {
		pane.GoUp()
		coord.LoadCurrentTab(nil)
	}
	m.Separator()
	if m.Item("复制当前虚拟路径").Chosen() {
		_ = macos.CopyPath(pane.Path, "posix")
	}
	if m.Item("刷新").Trailing("⌘R").Chosen() {
		coord.LoadCurrentTab(nil)
	}
}
