package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ViewMode defines the default view presentation.
type ViewMode string

const (
	ViewList ViewMode = "list"
	ViewGrid ViewMode = "grid"
)

// Preferences stores user preferences.
type Preferences struct {
	Appearance     string   `json:"appearance"` // "system", "light", "dark"
	AccentColor    string   `json:"accent_color"`
	Vibrancy       bool     `json:"vibrancy"`
	DefaultView    ViewMode `json:"default_view"`
	ShowHidden     bool     `json:"show_hidden"`
	ShowBookmark   bool     `json:"show_bookmark"`
	ShowPreview    bool     `json:"show_preview"`
	GridIconSize   int      `json:"grid_icon_size"`
	SidebarWidth   float32  `json:"sidebar_width"`   // default: 155
	DualPaneRatio  float32  `json:"dual_pane_ratio"` // default: 50 (%)
	PreviewWidth   float32  `json:"preview_width"`   // default: 250
	DragAction     string   `json:"drag_action"`     // "move" or "copy"
	ConflictAction string   `json:"conflict_action"` // "ask", "rename", "overwrite"
	DoubleClickNav bool     `json:"double_click_nav"`
	RestoreSession bool     `json:"restore_session"`
	// TakeoverFinder makes CloverGo the system-wide handler for "Open Folder"
	// and "Show in Finder" requests on macOS. Defaults to off so that merely
	// installing CloverGo never changes the user's system defaults.
	TakeoverFinder     bool    `json:"takeover_finder"`
	LaunchAtLogin      bool    `json:"launch_at_login"`
	SortField          string  `json:"sort_field"`            // "create_time", "mod_time", "name", "size", "kind"
	SortDirection      string  `json:"sort_direction"`        // "asc" or "desc"
	ColWidthName       float32 `json:"col_width_name"`        // default: 220
	ColWidthCreateTime float32 `json:"col_width_create_time"` // default: 115
	ColWidthModTime    float32 `json:"col_width_mod_time"`    // default: 115
	ColWidthSize       float32 `json:"col_width_size"`        // default: 70
	ColWidthKind       float32 `json:"col_width_kind"`        // default: 90
	// Metadata column visibility: the zero value (false) shows every column,
	// so existing prefs.json files keep their full layout.
	HideColCreateTime bool `json:"hide_col_create_time,omitempty"`
	HideColModTime    bool `json:"hide_col_mod_time,omitempty"`
	HideColSize       bool `json:"hide_col_size,omitempty"`
	HideColKind       bool `json:"hide_col_kind,omitempty"`
}

// DefaultPreferences returns standard baseline preferences.
func DefaultPreferences() Preferences {
	return Preferences{
		Appearance:         "system",
		AccentColor:        "#0A84FF",
		Vibrancy:           true,
		DefaultView:        ViewList,
		ShowHidden:         false,
		ShowBookmark:       true,
		ShowPreview:        true,
		GridIconSize:       88,
		SidebarWidth:       155,
		DualPaneRatio:      50,
		PreviewWidth:       250,
		DragAction:         "move",
		ConflictAction:     "ask",
		DoubleClickNav:     true,
		RestoreSession:     true,
		TakeoverFinder:     false,
		SortField:          "create_time",
		SortDirection:      "desc",
		ColWidthName:       220,
		ColWidthCreateTime: 115,
		ColWidthModTime:    115,
		ColWidthSize:       70,
		ColWidthKind:       90,
	}
}

// PrefsManager coordinates atomic thread-safe reading and writing of preferences.
type PrefsManager struct {
	mu       sync.RWMutex
	filePath string
	current  Preferences
}

// NewPrefsManager loads or initializes preferences from path.
func NewPrefsManager(path string) (*PrefsManager, error) {
	mgr := &PrefsManager{
		filePath: path,
		current:  DefaultPreferences(),
	}

	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &mgr.current)
		if mgr.current.SidebarWidth <= 0 {
			mgr.current.SidebarWidth = 155
		}
		if mgr.current.DualPaneRatio <= 0 {
			mgr.current.DualPaneRatio = 50
		}
		if mgr.current.PreviewWidth <= 0 {
			mgr.current.PreviewWidth = 250
		}
		if mgr.current.DragAction == "" {
			mgr.current.DragAction = "move"
		}
		if mgr.current.ConflictAction == "" {
			mgr.current.ConflictAction = "ask"
		}
		mgr.current.SortField = normalizeSortField(mgr.current.SortField)
		mgr.current.SortDirection = normalizeSortDirection(mgr.current.SortDirection)
		if mgr.current.ColWidthName < 80 || mgr.current.ColWidthName > 1200 {
			mgr.current.ColWidthName = 220
		}
		if mgr.current.ColWidthCreateTime < 50 || mgr.current.ColWidthCreateTime > 500 {
			mgr.current.ColWidthCreateTime = 115
		}
		// 存量脏数据修复：旧版名称列分隔线把 -dx 传给创建时间列，反向拖拽会
		// 把该列异常撑大（远超默认 115），加载时收敛回默认值。
		if mgr.current.ColWidthCreateTime > 180 {
			mgr.current.ColWidthCreateTime = 115
		}
		if mgr.current.ColWidthModTime < 50 || mgr.current.ColWidthModTime > 500 {
			mgr.current.ColWidthModTime = 115
		}
		if mgr.current.ColWidthSize < 40 || mgr.current.ColWidthSize > 300 {
			mgr.current.ColWidthSize = 70
		}
		if mgr.current.ColWidthKind < 50 || mgr.current.ColWidthKind > 400 {
			mgr.current.ColWidthKind = 90
		}
	} else if os.IsNotExist(err) {
		_ = mgr.Save(mgr.current)
	}

	return mgr, nil
}

// Get returns a thread-safe snapshot of current preferences.
func (m *PrefsManager) Get() Preferences {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// validSortFields/Directions mirror domain.SortField/SortDirection values;
// config does not import domain to keep the dependency direction stable.
var validSortFields = map[string]bool{
	"create_time": true,
	"mod_time":    true,
	"name":        true,
	"size":        true,
	"kind":        true,
}

var validSortDirections = map[string]bool{
	"asc":  true,
	"desc": true,
}

// normalizeSortField maps empty or unknown stored values to the default.
func normalizeSortField(v string) string {
	if validSortFields[v] {
		return v
	}
	return "create_time"
}

// normalizeSortDirection maps empty or unknown stored values to the default.
func normalizeSortDirection(v string) string {
	if validSortDirections[v] {
		return v
	}
	return "desc"
}

// Save atomically writes preferences to disk via temp-file + rename.
func (m *PrefsManager) Save(p Preferences) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal preferences: %w", err)
	}

	dir := filepath.Dir(m.filePath)
	tmpFile, err := os.CreateTemp(dir, "prefs-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp prefs file: %w", err)
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp prefs: %w", err)
	}
	_ = tmpFile.Close()

	if err := os.Rename(tmpName, m.filePath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("atomic rename prefs: %w", err)
	}

	m.current = p
	return nil
}
