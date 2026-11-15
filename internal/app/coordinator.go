package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bighaidao/clover-go/internal/config"
	"github.com/bighaidao/clover-go/internal/domain"
	"github.com/bighaidao/clover-go/internal/icon"
	"github.com/bighaidao/clover-go/internal/macos"
	"github.com/bighaidao/clover-go/internal/search"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// MainRunner defines the function signature for dispatching onto the UI thread.
type MainRunner func(func())

// ClipboardOp defines the clipboard action mode.
type ClipboardOp int

const (
	ClipNone ClipboardOp = iota
	ClipCopy
	ClipCut
)

// InternalClipboard captures clipboard operations.
type InternalClipboard struct {
	Op    ClipboardOp
	Paths []string
}

// Coordinator coordinates asynchronous I/O and bridges background workers to UI state.
type Coordinator struct {
	Tabs                *TabManager
	Prefs               *config.PrefsManager
	Workspaces          *config.WorkspaceManager
	Search              *search.Engine
	Icons               *icon.Pipeline
	ViewMode            config.ViewMode
	SearchText          string
	IsEditing           bool       // Whether path edit bar is open
	FocusFilter         bool       // one-shot flag: focus the filter box this frame (⌘F)
	RenamingPane        *PaneState // pane owning the inline rename (nil = none)
	RenamingIdx         int        // -1 if no item being renamed
	RenameDraft         string
	RenameSelectPending bool // 首帧自动选中文件名主干（不含扩展名）
	RenameFocusedOnce   bool // 标记重命名输入框已获得过焦点，用于失焦自动提交
	GridIconSize        int
	ShowPreview         bool
	CurrentPreview      *PreviewSnapshot
	CurrentVideoPlayer  *ui.VideoPlayer
	CurrentVideoPath    string
	CurrentEditorState  *editor.State
	CurrentEditorPath   string
	MarkdownViewMode    int // 0: 渲染预览 (Rendered), 1: 源码查看 (Source)
	Clipboard           InternalClipboard
	FilterTag           int // 0 means no tag filter, 1..7 filters by Finder color tag
	OnDetach            func(tabIdx int)
	WriteSysClipboard   func(paths ...string) error
	ReadSysClipboard    func() ([]string, error)
	// Batch Rename Modal State
	ShowBatchRenameModal bool
	BatchRenameItems     []domain.FileInfo
	BatchRenamePane      *PaneState // pane staged when the modal opened
	BatchRenameMode      domain.RenameMode
	BatchRenameBase      string
	BatchRenameFind      string
	BatchRenameReplace   string
	BatchRenamePrefix    string
	BatchRenameSuffix    string
	BatchRenameError     string // last execution error, rendered inside the modal

	// Settings & About Modal State
	ShowSettingsModal bool
	ShowAboutModal    bool
	SettingsActiveTab string // "general", "appearance", "dnd", "shortcuts", "about"

	// Layout State (mirrors config.Preferences, adjusted by ui.Split)
	SidebarWidth  float32
	DualPaneRatio float32
	PreviewWidth  float32

	// Table Column Widths (mirrors config.Preferences, adjusted by header resize handles)
	ColWidthName       float32
	ColWidthCreateTime float32
	ColWidthModTime    float32
	ColWidthSize       float32
	ColWidthKind       float32
	NameColManual      bool // 用户是否手动拖拽过“名称”列宽（默认 false 自适应撑满剩余空间）
	HideColCreateTime  bool
	HideColModTime     bool
	HideColSize        bool
	HideColKind        bool

	// Drag-resize state (memory-only during drag, flushed on DragEnded, reverted on PressCanceled)
	previewDragStart  float32
	isDraggingPreview bool
	colDragStart      map[string]float32

	// Toast notification
	ToastMessage string
	ToastPending bool // 触发 mygo 原生 c.Toast 浮动提示的一帧标志
	toastTimer   *time.Timer

	// Conflict Modal State
	ShowConflictModal     bool
	PendingConflict       *PendingConflict
	ConflictApplyToAll    bool
	ConflictDefaultChoice ConflictChoice

	// TransferBusy 标记一次完整转移会话（含冲突弹窗挂起与后台批次）进行中，
	// 仅在主线程读写，用于拒绝并发转移批次，防止两个批次同时写同一目标目录。
	TransferBusy bool

	// PDF & Watermark Modal State
	ShowPDFModal       bool
	PDFOptions         domain.PDFToolsOptions
	PDFTargetPane      *PaneState
	PDFTotalPages      int
	PDFError           string
	PDFIsProcessing    bool
	ShowWatermarkModal bool
	WatermarkOptions   domain.WatermarkOptions
	WatermarkPane      *PaneState
	WatermarkError     string
	WatermarkIsRunning bool

	// Finder Import Modal State
	ShowFinderImportModal bool
	FinderFavorites       []domain.FinderFavoriteItem
	FinderImportTarget    string // "bookmarks" or "favorites"
	FinderImportError     string // last import error, rendered inside the modal

	// Workspace Manager State
	ShowWorkspacesModal bool
	ActiveWorkspaceID   string
	WorkspaceDraftName  string

	watcherMu         sync.Mutex // guards watcher & activeWatchedPath against concurrent Close/restart
	watcher           *macos.DirWatcher
	activeWatchedPath string
	// secondaryWatcher 监听双栏模式下右栏的目录变动，主监听器只跟激活栏，
	// 否则在右栏浏览时左栏的 FSEvents 变动不会自动刷新。
	secondaryWatcher     *macos.DirWatcher
	secondaryWatchedPath string
	runOnMain            MainRunner
	invalidate           func()
	focusWindow          func()
}

// NewCoordinator initializes the application coordinator.
func NewCoordinator(initialPath string, prefs *config.PrefsManager, ws *config.WorkspaceManager, icons *icon.Pipeline, runner MainRunner) *Coordinator {
	p := prefs.Get()
	gridSize := p.GridIconSize
	if gridSize < 64 || gridSize > 128 {
		gridSize = 88
	}

	sbWidth := p.SidebarWidth
	if sbWidth < 120 || sbWidth > 300 {
		sbWidth = 155
	}
	dpRatio := p.DualPaneRatio
	if dpRatio < 10 || dpRatio > 90 {
		dpRatio = 50
	}
	pvWidth := p.PreviewWidth
	if pvWidth < 180 || pvWidth > 500 {
		pvWidth = 250
	}

	colName := p.ColWidthName
	if colName < 80 || colName > 1200 {
		colName = 220
	}
	colCreate := p.ColWidthCreateTime
	if colCreate < 50 || colCreate > 500 {
		colCreate = 115
	}
	colMod := p.ColWidthModTime
	if colMod < 50 || colMod > 500 {
		colMod = 115
	}
	colSize := p.ColWidthSize
	if colSize < 40 || colSize > 300 {
		colSize = 70
	}
	colKind := p.ColWidthKind
	if colKind < 50 || colKind > 400 {
		colKind = 90
	}

	// SortOptionsFromPrefs maps persisted preference strings (validated at load
	// time) onto domain sort options; it is the single conversion point between
	// config and domain sorting types.
	sortOptionsFromPrefs := func(p config.Preferences) domain.SortOptions {
		return domain.SortOptions{
			Field:     domain.SortField(p.SortField),
			Direction: domain.SortDirection(p.SortDirection),
		}
	}

	c := &Coordinator{
		Tabs:               NewTabManager(initialPath),
		Prefs:              prefs,
		Workspaces:         ws,
		Search:             search.NewEngine(),
		Icons:              icons,
		ViewMode:           p.DefaultView,
		RenamingIdx:        -1,
		GridIconSize:       gridSize,
		ShowPreview:        p.ShowPreview,
		SidebarWidth:       sbWidth,
		DualPaneRatio:      dpRatio,
		PreviewWidth:       pvWidth,
		ColWidthName:       colName,
		ColWidthCreateTime: colCreate,
		ColWidthModTime:    colMod,
		ColWidthSize:       colSize,
		ColWidthKind:       colKind,
		HideColCreateTime:  p.HideColCreateTime,
		HideColModTime:     p.HideColModTime,
		HideColSize:        p.HideColSize,
		HideColKind:        p.HideColKind,
		SettingsActiveTab:  "general",
		runOnMain:          runner,
		colDragStart:       make(map[string]float32),
	}
	// Newly created panes (tabs, dual-pane right panes) start with the
	// user's preferred default ordering.
	c.Tabs.DefaultSort = sortOptionsFromPrefs(p)
	if len(c.Tabs.Tabs) > 0 {
		c.Tabs.Tabs[0].PaneState.Sort = c.Tabs.DefaultSort
		if c.Tabs.Tabs[0].RightPane != nil {
			c.Tabs.Tabs[0].RightPane.Sort = c.Tabs.DefaultSort
		}
	}
	return c
}

// SetInvalidate registers the UI redraw trigger callback.
func (c *Coordinator) SetInvalidate(fn func()) {
	c.invalidate = fn
}

// SetFocusWindow registers a callback to bring the main window to the foreground.
func (c *Coordinator) SetFocusWindow(fn func()) {
	c.focusWindow = fn
}

// FocusWindow brings the window to front on the main thread.
func (c *Coordinator) FocusWindow() {
	if c.focusWindow != nil {
		c.runOnMain(c.focusWindow)
	}
}

// RevealPath opens a directory or reveals and selects a specific file in CloverGo.
// When forceReveal is true, even a directory or .app bundle is selected inside its parent folder
// (matching "Show in Finder" / open -R semantics). When forceReveal is false, a plain directory
// is opened directly as a tab, while a regular file or .app bundle is revealed in its parent directory.
func (c *Coordinator) RevealPath(rawPath string, forceReveal bool) {
	norm := domain.NormalizePath(rawPath)
	if norm == "" {
		return
	}

	fi, err := os.Stat(norm)
	isAppBundle := strings.HasSuffix(strings.ToLower(norm), ".app")

	// Determine whether to open norm as a directory or select norm inside its parent directory.
	openAsDir := false
	if err == nil && fi.IsDir() && !isAppBundle && !forceReveal {
		openAsDir = true
	}

	if openAsDir {
		// Switch to existing tab if already open, otherwise create a new tab
		switched := false
		for i, t := range c.Tabs.Tabs {
			if t.ActivePane().Path == norm {
				c.Tabs.ActiveIndex = i
				switched = true
				break
			}
		}
		if !switched {
			c.Tabs.NewTab(norm)
		}
		c.FocusWindow()
		c.LoadCurrentTab(func() {
			c.Invalidate()
		})
		return
	}

	parentDir := filepath.Dir(norm)
	targetFile := norm

	// If target is hidden (starts with '.') and ShowHidden is off, temporarily enable or ensure it can be seen
	if domain.IsHiddenName(filepath.Base(targetFile)) && c.Prefs != nil && !c.Prefs.Get().ShowHidden {
		p := c.Prefs.Get()
		p.ShowHidden = true
		_ = c.Prefs.Save(p)
	}

	// Reuse existing tab if it is already at parentDir, otherwise open a new tab
	var targetTab *TabState
	for i, t := range c.Tabs.Tabs {
		if t.ActivePane().Path == parentDir {
			c.Tabs.ActiveIndex = i
			targetTab = t
			break
		}
	}
	if targetTab == nil {
		targetTab = c.Tabs.NewTab(parentDir)
	}

	pane := targetTab.ActivePane()
	// Clear any active filter that might hide the revealed file
	pane.FilterQuery = ""
	pane.IsSearchMode = false
	c.FilterTag = 0

	c.FocusWindow()

	selectTarget := func() {
		idx := -1
		for i, entry := range pane.DisplayEntries {
			if entry.Path == targetFile || strings.EqualFold(entry.Path, targetFile) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		actualPath := pane.DisplayEntries[idx].Path
		pane.Chosen.Clear()
		pane.Chosen.Add(actualPath)
		pane.SelectedIdx = idx
		pane.TableState.ScrollTo(idx, ui.Center)
		pane.GridState.ScrollTo(idx, ui.Center)
		c.LoadPreview(pane.DisplayEntries[idx], nil)
		c.Invalidate()
	}

	c.LoadCurrentTab(selectTarget)
}

// SetFinderTakeover toggles Finder takeover and shows a toast notification.
func (c *Coordinator) SetFinderTakeover(enable bool) {
	go func() {
		err := macos.SetFinderTakeover(enable)
		if enable {
			macos.StartFinderWatch()
			_ = macos.InstallWatchLaunchAgent()
		} else {
			macos.StopFinderWatch()
			macos.UninstallWatchLaunchAgent()
		}
		c.runOnMain(func() {
			if err != nil {
				c.Toast("设置 Finder 接管失败: " + err.Error())
				return
			}
			if enable {
				c.Toast("已开启 Finder 接管（部分已运行的三方应用重启后生效）")
			} else {
				c.Toast("已关闭接管，恢复系统默认 Finder")
			}
		})
	}()
}

// SetLaunchAtLogin registers or removes CloverGo as a login item.
func (c *Coordinator) SetLaunchAtLogin(enable bool) {
	go func() {
		err := macos.SetLaunchAtLogin(enable)
		c.runOnMain(func() {
			if err != nil {
				c.Toast("设置开机自启失败: " + err.Error())
				return
			}
			if enable {
				c.Toast("已开启开机自启")
			} else {
				c.Toast("已关闭开机自启")
			}
		})
	}()
}

// Invalidate safely dispatches a window redraw request onto the main thread.
func (c *Coordinator) Invalidate() {
	if c.invalidate != nil {
		c.runOnMain(c.invalidate)
	}
}

// SetDefaultSortAndResort updates the preferred default sort and immediately
// applies it to all currently open panes/tabs, re-sorting entries in place so the
// UI updates seamlessly without requiring an app restart.
func (c *Coordinator) SetDefaultSortAndResort(field domain.SortField, dir domain.SortDirection) {
	opt := domain.SortOptions{
		Field:     field,
		Direction: dir,
	}
	c.Tabs.DefaultSort = opt

	for _, tab := range c.Tabs.Tabs {
		if tab == nil {
			continue
		}
		tab.Left().Sort = opt
		domain.SortFiles(tab.Left().Entries, opt)
		c.ApplyPaneFilter(tab.Left())
		if tab.Right() != nil {
			tab.Right().Sort = opt
			domain.SortFiles(tab.Right().Entries, opt)
			c.ApplyPaneFilter(tab.Right())
		}
	}
	c.Invalidate()
}

// UpdatePreviewWidthInMemory updates PreviewWidth in memory during drag without disk write.
func (c *Coordinator) UpdatePreviewWidthInMemory(dx float32) {
	if !c.isDraggingPreview {
		c.previewDragStart = c.PreviewWidth
		c.isDraggingPreview = true
	}
	newW := c.PreviewWidth - dx
	if newW < 180 {
		newW = 180
	} else if newW > 500 {
		newW = 500
	}
	c.PreviewWidth = newW
	c.Invalidate()
}

// EndPreviewDrag persists the final preview width to preferences.
func (c *Coordinator) EndPreviewDrag() {
	if !c.isDraggingPreview {
		return
	}
	c.isDraggingPreview = false
	if c.Prefs != nil {
		p := c.Prefs.Get()
		p.PreviewWidth = c.PreviewWidth
		_ = c.Prefs.Save(p)
	}
}

// CancelPreviewDrag restores the preview width before dragging started.
func (c *Coordinator) CancelPreviewDrag() {
	if c.isDraggingPreview {
		c.PreviewWidth = c.previewDragStart
		c.isDraggingPreview = false
		c.Invalidate()
	}
}

func (c *Coordinator) getColWidth(col string) float32 {
	switch col {
	case "name":
		return c.ColWidthName
	case "create_time":
		return c.ColWidthCreateTime
	case "mod_time":
		return c.ColWidthModTime
	case "size":
		return c.ColWidthSize
	case "kind":
		return c.ColWidthKind
	default:
		return 100
	}
}

func (c *Coordinator) setColWidth(col string, w float32) {
	switch col {
	case "name":
		c.ColWidthName = w
	case "create_time":
		c.ColWidthCreateTime = w
	case "mod_time":
		c.ColWidthModTime = w
	case "size":
		c.ColWidthSize = w
	case "kind":
		c.ColWidthKind = w
	}
}

func (c *Coordinator) applyColDelta(col string, dx float32) {
	switch col {
	case "name":
		w := c.ColWidthName + dx
		if w < 80 {
			w = 80
		} else if w > 1200 {
			w = 1200
		}
		c.ColWidthName = w
		c.NameColManual = true
	case "create_time":
		w := c.ColWidthCreateTime + dx
		if w < 50 {
			w = 50
		} else if w > 500 {
			w = 500
		}
		c.ColWidthCreateTime = w
	case "mod_time":
		w := c.ColWidthModTime + dx
		if w < 50 {
			w = 50
		} else if w > 500 {
			w = 500
		}
		c.ColWidthModTime = w
	case "size":
		w := c.ColWidthSize + dx
		if w < 40 {
			w = 40
		} else if w > 300 {
			w = 300
		}
		c.ColWidthSize = w
	case "kind":
		w := c.ColWidthKind + dx
		if w < 50 {
			w = 50
		} else if w > 400 {
			w = 400
		}
		c.ColWidthKind = w
	}
}

// DragColWidth adjusts column width in memory during dragging without saving to prefs.
func (c *Coordinator) DragColWidth(col string, dx float32) {
	if c.colDragStart == nil {
		c.colDragStart = make(map[string]float32)
	}
	if _, ok := c.colDragStart[col]; !ok {
		c.colDragStart[col] = c.getColWidth(col)
	}
	c.applyColDelta(col, dx)
	c.Invalidate()
}

// EndColDrag commits column widths to preferences upon drag release.
func (c *Coordinator) EndColDrag() {
	if len(c.colDragStart) > 0 {
		c.colDragStart = make(map[string]float32)
		c.SaveColWidths()
	}
}

// CancelColDrag rolls back the column width to the start of the drag.
func (c *Coordinator) CancelColDrag(col string) {
	if c.colDragStart != nil {
		if start, ok := c.colDragStart[col]; ok {
			c.setColWidth(col, start)
			delete(c.colDragStart, col)
			c.Invalidate()
		}
	}
}

// UpdateColWidth adjusts the width of a specified column by dx DIPs (clamped within safe bounds)
// and persists the updated width.
func (c *Coordinator) UpdateColWidth(col string, dx float32) {
	c.applyColDelta(col, dx)
	c.SaveColWidths()
	c.Invalidate()
}

// ResetColWidth resets the specified column width back to its standard default.
func (c *Coordinator) ResetColWidth(col string) {
	switch col {
	case "name":
		c.ColWidthName = 220
		c.NameColManual = false
	case "create_time":
		c.ColWidthCreateTime = 115
	case "mod_time":
		c.ColWidthModTime = 115
	case "size":
		c.ColWidthSize = 70
	case "kind":
		c.ColWidthKind = 90
	}
	c.SaveColWidths()
	c.Invalidate()
}

// ToggleColumnVisible toggles the visibility of a metadata column and persists it.
func (c *Coordinator) ToggleColumnVisible(col string) {
	switch col {
	case "create_time":
		c.HideColCreateTime = !c.HideColCreateTime
	case "mod_time":
		c.HideColModTime = !c.HideColModTime
	case "size":
		c.HideColSize = !c.HideColSize
	case "kind":
		c.HideColKind = !c.HideColKind
	}
	c.SaveColWidths()
	c.Invalidate()
}

// ResetAllColumns restores all column widths and visibility to defaults.
func (c *Coordinator) ResetAllColumns() {
	c.ColWidthName = 220
	c.NameColManual = false
	c.ColWidthCreateTime = 115
	c.ColWidthModTime = 115
	c.ColWidthSize = 70
	c.ColWidthKind = 90
	c.HideColCreateTime = false
	c.HideColModTime = false
	c.HideColSize = false
	c.HideColKind = false
	c.SaveColWidths()
	c.Invalidate()
}

// SaveColWidths writes the current in-memory column widths to user preferences.
func (c *Coordinator) SaveColWidths() {
	p := c.Prefs.Get()
	p.ColWidthName = c.ColWidthName
	p.ColWidthCreateTime = c.ColWidthCreateTime
	p.ColWidthModTime = c.ColWidthModTime
	p.ColWidthSize = c.ColWidthSize
	p.ColWidthKind = c.ColWidthKind
	p.HideColCreateTime = c.HideColCreateTime
	p.HideColModTime = c.HideColModTime
	p.HideColSize = c.HideColSize
	p.HideColKind = c.HideColKind
	_ = c.Prefs.Save(p)
}

// Close gracefully releases background resources including file watchers.
func (c *Coordinator) Close() {
	c.watcherMu.Lock()
	defer c.watcherMu.Unlock()
	if c.watcher != nil {
		c.watcher.Stop()
		c.watcher = nil
		c.activeWatchedPath = ""
	}
	if c.secondaryWatcher != nil {
		c.secondaryWatcher.Stop()
		c.secondaryWatcher = nil
		c.secondaryWatchedPath = ""
	}
}

func (c *Coordinator) restartWatcher(path string) {
	c.watcherMu.Lock()
	defer c.watcherMu.Unlock()

	norm := domain.NormalizePath(path)
	if c.watcher != nil && c.activeWatchedPath == norm {
		return
	}
	if c.watcher != nil {
		c.watcher.Stop()
		c.watcher = nil
	}
	c.activeWatchedPath = norm
	c.watcher = macos.WatchDirectory(norm, 200*time.Millisecond, func() {
		c.runOnMain(func() {
			c.LoadCurrentTab(nil)
		})
	})
}

// restartSecondaryWatcher watches the other pane's directory in dual-pane
// mode, or stops watching when path is empty or is the pane the primary
// watcher already follows.
func (c *Coordinator) restartSecondaryWatcher(path string) {
	c.watcherMu.Lock()
	defer c.watcherMu.Unlock()

	stop := func() {
		if c.secondaryWatcher != nil {
			c.secondaryWatcher.Stop()
			c.secondaryWatcher = nil
		}
		c.secondaryWatchedPath = ""
	}

	if path == "" {
		stop()
		return
	}
	norm := domain.NormalizePath(path)
	if norm == c.activeWatchedPath {
		stop()
		return
	}
	if c.secondaryWatcher != nil && c.secondaryWatchedPath == norm {
		return
	}
	stop()
	c.secondaryWatchedPath = norm
	c.secondaryWatcher = macos.WatchDirectory(norm, 200*time.Millisecond, func() {
		c.runOnMain(func() {
			c.LoadCurrentTab(nil)
		})
	})
}

// TogglePreview flips the preview pane visibility and persists it to preferences.
func (c *Coordinator) TogglePreview() {
	c.ShowPreview = !c.ShowPreview
	if !c.ShowPreview {
		c.ClosePreviewResources()
	}
	if c.Prefs != nil {
		p := c.Prefs.Get()
		p.ShowPreview = c.ShowPreview
		_ = c.Prefs.Save(p)
	}
	c.Invalidate()
}

// ClosePreviewResources cleanly releases heavy preview resources such as looping video decoders.
func (c *Coordinator) ClosePreviewResources() {
	if c.CurrentVideoPlayer != nil {
		c.CurrentVideoPlayer.Close()
		c.CurrentVideoPlayer = nil
		c.CurrentVideoPath = ""
	}
	c.CurrentEditorState = nil
	c.CurrentEditorPath = ""
}

// SelectedItems 与其余条目级文件操作已收敛至 file_ops.go（pane-aware）；
// PreviewSnapshot/LoadPreview 在 preview.go。

// LoadCurrentTab asynchronously fetches directory entries for the active tab (both panes if dual).
func (c *Coordinator) LoadCurrentTab(onDone func()) {
	tab := c.Tabs.ActiveTab()
	if tab == nil {
		return
	}

	// Silent reload (FSEvents watcher) must not clobber an active search
	// result view in EITHER pane; explicit user actions (onDone != nil) reload.
	if onDone == nil && (tab.IsSearchMode || (tab.RightPane != nil && tab.RightPane.IsSearchMode)) {
		return
	}

	c.restartWatcher(tab.ActivePane().Path)

	if tab.IsDual && tab.RightPane != nil {
		// 双栏：主监听跟激活栏，副监听跟另一栏（同路径时自动去重）
		if tab.ActivePane() == &tab.PaneState {
			c.restartSecondaryWatcher(tab.RightPane.Path)
		} else {
			c.restartSecondaryWatcher(tab.PaneState.Path)
		}
		c.LoadPane(&tab.PaneState, nil)
		c.LoadPane(tab.RightPane, onDone)
		return
	}

	c.restartSecondaryWatcher("")
	c.LoadPane(&tab.PaneState, onDone)
}

// ReloadCurrentTab 强制重读当前标签页的目录：刷新按钮与 ⌘R 的入口。
// 与 watcher 触发的静默重载不同，它总是重新读取磁盘，即使 pane 正处于搜索
// 模式（随后回到真实目录列表），避免手动刷新被无声忽略。
func (c *Coordinator) ReloadCurrentTab() {
	// LoadCurrentTab 以 onDone 是否为 nil 区分“用户主动”与“静默重载”：
	// 传一个非 nil 的回调即表示必须重载。
	c.LoadCurrentTab(func() {})
}

// ApplyFilter computes DisplayEntries according to current FilterQuery and FilterTag.
func (c *Coordinator) ApplyFilter(tab *TabState) {
	if tab == nil {
		return
	}
	c.ApplyPaneFilter(&tab.PaneState)
	if tab.IsDual && tab.RightPane != nil {
		c.ApplyPaneFilter(tab.RightPane)
	}
}

// CopySelected, CutSelected, Paste, batch rename and the remaining item-level
// operations live in file_ops.go; they are all pane-aware.

// SortByColumn was removed: column sorting now happens per-pane in the list
// headers; rename helpers live pane-aware in file_ops.go.

// ToggleView switches between List and Grid view modes (⌘1 app menu).
func (c *Coordinator) ToggleView() {
	if c.ViewMode == config.ViewList {
		c.ViewMode = config.ViewGrid
	} else {
		c.ViewMode = config.ViewList
	}
}

// SearchCurrentTab performs a recursive search across the pane's directory.
func (c *Coordinator) SearchCurrentTab(ctx context.Context, pane *PaneState, query string, onDone func([]domain.FileInfo)) {
	if pane == nil {
		return
	}
	path := pane.Path
	go func() {
		results, _ := c.Search.Search(ctx, path, query, 100)
		c.runOnMain(func() {
			if onDone != nil {
				onDone(results)
			}
		})
	}()
}

// StartSearch switches the given pane into recursive search mode: the query
// results replace the pane listing until the user clears the filter or
// navigates elsewhere (which resets IsSearchMode in LoadCurrentTab).
func (c *Coordinator) StartSearch(pane *PaneState) {
	if pane == nil {
		return
	}
	query := strings.TrimSpace(pane.FilterQuery)
	if query == "" {
		// Clearing the query exits search mode and reloads the real directory.
		pane.IsSearchMode = false
		c.LoadPane(pane, nil)
		return
	}
	pane.IsSearchMode = true
	pane.IsLoading = true
	pane.LoadGen++
	pane.SelectedIdx = -1
	pane.Chosen.Clear()

	gen := pane.LoadGen
	c.SearchCurrentTab(context.Background(), pane, query, func(results []domain.FileInfo) {
		if gen != pane.LoadGen {
			return // stale search response, discard
		}
		pane.IsLoading = false
		pane.Entries = results
		c.ApplyPaneFilter(pane)
		c.Invalidate()
	})
}

// OpenSettings shows the preferences modal.
func (c *Coordinator) OpenSettings() {
	c.ShowSettingsModal = true
	c.Invalidate()
}

// CloseSettings hides the preferences modal.
func (c *Coordinator) CloseSettings() {
	c.ShowSettingsModal = false
	c.Invalidate()
}

// BeginPathEdit enters path-edit mode for the given pane with a fresh draft
// seeded from its real path; discarding the draft (Esc) leaves Path untouched.
func (c *Coordinator) BeginPathEdit(pane *PaneState) {
	if pane == nil {
		return
	}
	pane.PathDraft = pane.Path
	c.IsEditing = true
}

// ExitSearch leaves search mode on the given pane and reloads its real
// directory listing directly: LoadCurrentTab would early-return while any
// pane is still in search mode.
func (c *Coordinator) ExitSearch(pane *PaneState) {
	if pane == nil {
		return
	}
	pane.IsSearchMode = false
	c.LoadPane(pane, nil)
}
