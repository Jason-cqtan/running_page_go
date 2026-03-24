// Package generator 负责扫描数据目录，将 GPX/TCX/FIT 文件写入 SQLite，
// 并生成 activities.json 文件（与原 Python 版格式兼容）。
package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jason-cqtan/running_page_go/db"
)

// Generator 持有数据库连接，提供数据同步和 JSON 导出功能。
type Generator struct {
	DB *db.DB
}

// New 创建 Generator 实例。
func New(database *db.DB) *Generator {
	return &Generator{DB: database}
}

// SyncFromDataDir 扫描 dir 目录，解析所有 suffix 扩展名的文件（如 ".gpx"），
// 将结果写入 SQLite。titleDict 可选，用于覆盖活动名称（key 为文件名不含扩展名）。
func (g *Generator) SyncFromDataDir(dir, suffix string, titleDict map[string]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取目录失败 %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), suffix) {
			continue
		}

		// 使用文件名（去扩展名）作为活动 ID
		activityID := strings.TrimSuffix(name, filepath.Ext(name))
		fullPath := filepath.Join(dir, name)

		var act *db.Activity
		var parseErr error

		switch strings.ToLower(suffix) {
		case ".gpx":
			act, parseErr = ParseGPX(fullPath, activityID)
		case ".tcx":
			act, parseErr = ParseTCX(fullPath, activityID)
		default:
			// FIT 文件暂不支持直接解析，跳过
			continue
		}

		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "警告：解析文件 %s 失败: %v\n", name, parseErr)
			continue
		}

		// 应用自定义标题
		if titleDict != nil {
			if title, ok := titleDict[activityID]; ok {
				act.Name = title
			}
		}

		if err := g.DB.InsertOrUpdate(act); err != nil {
			return fmt.Errorf("写入活动 %s 失败: %w", activityID, err)
		}
		fmt.Printf("已同步: %s (%s)\n", act.Name, activityID)
	}
	return nil
}

// activityJSON 是 activities.json 中每个元素的 JSON 格式（与原 Python 版兼容）。
type activityJSON struct {
	RunID            string          `json:"run_id"`
	Name             string          `json:"name"`
	Type             string          `json:"type"`
	StartDate        string          `json:"start_date"`
	End              string          `json:"end_date"`
	StartDateLocal   string          `json:"start_date_local"`
	Distance         float64         `json:"distance"`
	MovingTime       int64           `json:"moving_time"`
	ElapsedTime      int64           `json:"elapsed_time"`
	AverageSpeed     float64         `json:"average_speed"`
	AverageHeartrate *float64        `json:"average_heartrate"`
	ElevationGain    *float64        `json:"elevation_gain"`
	Map              activityMapJSON `json:"map"`
	StartLatLng      string          `json:"start_latlng"`
	LocationCountry  string          `json:"location_country"`
}

type activityMapJSON struct {
	SummaryPolyline string `json:"summary_polyline"`
}

// SaveActivitiesJSON 从 SQLite 读取全部活动，序列化后写入 path 路径的 JSON 文件。
func (g *Generator) SaveActivitiesJSON(path string) error {
	activities, err := g.DB.GetAll()
	if err != nil {
		return fmt.Errorf("读取活动失败: %w", err)
	}

	result := make([]activityJSON, 0, len(activities))
	for _, a := range activities {
		result = append(result, activityJSON{
			RunID:          a.ID,
			Name:           a.Name,
			Type:           a.Type,
			StartDate:      a.StartDate,
			End:            a.End,
			StartDateLocal: a.StartDateLocal,
			Distance:       a.Distance,
			MovingTime:     a.MovingTime,
			ElapsedTime:    a.ElapsedTime,
			AverageSpeed:   a.AverageSpeed,
			AverageHeartrate: a.AverageHeartrate,
			ElevationGain:  a.ElevationGain,
			Map: activityMapJSON{
				SummaryPolyline: a.SummaryPolyline,
			},
			StartLatLng:     a.StartLatLng,
			LocationCountry: a.LocationCountry,
		})
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %w", err)
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入 JSON 文件失败: %w", err)
	}

	fmt.Printf("已生成 activities.json，共 %d 条记录\n", len(result))
	return nil
}
