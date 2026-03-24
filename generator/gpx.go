// Package generator 负责解析 GPX 文件并提取运动数据。
// 使用 github.com/tkrajina/gpxgo 库。
package generator

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tkrajina/gpxgo/gpx"

	"github.com/Jason-cqtan/running_page_go/db"
	"github.com/Jason-cqtan/running_page_go/utils"
)

// ParseGPX 解析指定路径的 GPX 文件，返回 Activity 记录。
// activityID 用作数据库主键。
func ParseGPX(path string, activityID string) (*db.Activity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 GPX 文件失败: %w", err)
	}

	g, err := gpx.ParseBytes(data)
	if err != nil {
		return nil, fmt.Errorf("解析 GPX 文件失败: %w", err)
	}

	// 收集所有轨迹点
	var allPoints []gpx.GPXPoint
	for _, track := range g.Tracks {
		for _, seg := range track.Segments {
			allPoints = append(allPoints, seg.Points...)
		}
	}

	if len(allPoints) == 0 {
		return nil, fmt.Errorf("GPX 文件无轨迹点: %s", path)
	}

	// 时间范围
	startTime := allPoints[0].Timestamp
	endTime := allPoints[len(allPoints)-1].Timestamp

	// 计算总距离（米）
	movingData := g.MovingData()
	distance := movingData.MovingDistance + movingData.StoppedDistance

	// 移动时间和总经过时间（秒）
	movingTime := int64(movingData.MovingTime)
	elapsedTime := int64(endTime.Sub(startTime).Seconds())
	if elapsedTime < 0 {
		elapsedTime = 0
	}

	// 平均速度（米/秒）
	var avgSpeed float64
	if movingTime > 0 {
		avgSpeed = distance / float64(movingTime)
	}

	// 海拔增益
	updownData := g.UphillDownhill()
	elevationGain := updownData.Uphill

	// 解析心率（从 TrackPointExtension）
	avgHR := extractAverageHeartrate(allPoints)

	// 编码 Polyline
	polylinePoints := make([][2]float64, 0, len(allPoints))
	for _, pt := range allPoints {
		if pt.Latitude != 0 || pt.Longitude != 0 {
			polylinePoints = append(polylinePoints, [2]float64{pt.Latitude, pt.Longitude})
		}
	}
	polyline := utils.EncodePolyline(polylinePoints)

	// 起始坐标
	startLatLng := ""
	if len(polylinePoints) > 0 {
		startLatLng = fmt.Sprintf("[%f, %f]", polylinePoints[0][0], polylinePoints[0][1])
	}

	// 本地时间（Asia/Shanghai）
	startLocal, _ := utils.AdjustTime(startTime, "Asia/Shanghai")
	endLocal, _ := utils.AdjustTime(endTime, "Asia/Shanghai")

	// 活动类型和名称（优先从 GPX track 字段获取）
	actType := "Run"
	actName := ""
	if len(g.Tracks) > 0 {
		if g.Tracks[0].Type != "" {
			actType = g.Tracks[0].Type
		}
		actName = g.Tracks[0].Name
	}
	if actName == "" {
		actName = fmt.Sprintf("%s %s", actType, startLocal.Format("2006-01-02"))
	}

	act := &db.Activity{
		ID:             activityID,
		Name:           actName,
		Type:           actType,
		Subtype:        actType,
		StartDate:      startTime.UTC().Format(time.RFC3339),
		End:            endTime.UTC().Format(time.RFC3339),
		StartDateLocal: startLocal.Format(time.RFC3339),
		EndLocal:       endLocal.Format(time.RFC3339),
		Length:         distance,
		Distance:       distance,
		MovingTime:     movingTime,
		ElapsedTime:    elapsedTime,
		AverageSpeed:   avgSpeed,
		SummaryPolyline: polyline,
		StartLatLng:    startLatLng,
		LocationCountry: "",
	}

	if elevationGain > 0 {
		eg := math.Round(elevationGain*10) / 10
		act.ElevationGain = &eg
	}
	if avgHR > 0 {
		act.AverageHeartrate = &avgHR
	}

	return act, nil
}

// extractAverageHeartrate 从轨迹点扩展数据中提取平均心率。
// GPX 扩展使用 Garmin TrackPointExtension 格式：<gpxtpx:hr>xxx</gpxtpx:hr>
func extractAverageHeartrate(points []gpx.GPXPoint) float64 {
	var sum float64
	count := 0
	for _, pt := range points {
		for _, ext := range pt.Extensions.Nodes {
			hr := findHRInExtension(ext)
			if hr > 0 {
				sum += hr
				count++
				break
			}
		}
	}
	if count == 0 {
		return 0
	}
	return math.Round(sum/float64(count)*10) / 10
}

// findHRInExtension 在 XML 节点树中递归查找心率值。
func findHRInExtension(node gpx.ExtensionNode) float64 {
	// 检查当前节点名称是否含 "hr"
	if strings.Contains(strings.ToLower(node.XMLName.Local), "hr") {
		if v, err := strconv.ParseFloat(strings.TrimSpace(node.Data), 64); err == nil {
			return v
		}
	}
	// 递归子节点
	for _, child := range node.Nodes {
		if v := findHRInExtension(child); v > 0 {
			return v
		}
	}
	return 0
}
