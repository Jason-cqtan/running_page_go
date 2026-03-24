// Package generator 负责解析 TCX 文件并提取运动数据。
// 使用标准库 encoding/xml 手动解析。
package generator

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/Jason-cqtan/running_page_go/db"
	"github.com/Jason-cqtan/running_page_go/utils"
)

// ----- TCX XML 结构定义 -----

type tcxTrainingCenterDatabase struct {
	XMLName    xml.Name      `xml:"TrainingCenterDatabase"`
	Activities []tcxActivity `xml:"Activities>Activity"`
}

type tcxActivity struct {
	Sport string    `xml:"Sport,attr"`
	ID    string    `xml:"Id"`
	Laps  []tcxLap  `xml:"Lap"`
}

type tcxLap struct {
	StartTime    string         `xml:"StartTime,attr"`
	TotalTime    float64        `xml:"TotalTimeSeconds"`
	Distance     float64        `xml:"DistanceMeters"`
	MaxSpeed     float64        `xml:"MaximumSpeed"`
	Calories     int            `xml:"Calories"`
	AvgHeartRate *tcxHeartRate  `xml:"AverageHeartRateBpm"`
	MaxHeartRate *tcxHeartRate  `xml:"MaximumHeartRateBpm"`
	Intensity    string         `xml:"Intensity"`
	TrigMethod   string         `xml:"TriggerMethod"`
	Tracks       []tcxTrack     `xml:"Track"`
}

type tcxHeartRate struct {
	Value float64 `xml:"Value"`
}

type tcxTrack struct {
	Points []tcxTrackpoint `xml:"Trackpoint"`
}

type tcxTrackpoint struct {
	Time      string          `xml:"Time"`
	Position  *tcxPosition    `xml:"Position"`
	Altitude  float64         `xml:"AltitudeMeters"`
	Distance  float64         `xml:"DistanceMeters"`
	HeartRate *tcxHeartRate   `xml:"HeartRateBpm"`
	Speed     float64         `xml:"Extensions>TPX>Speed"`
}

type tcxPosition struct {
	Latitude  float64 `xml:"LatitudeDegrees"`
	Longitude float64 `xml:"LongitudeDegrees"`
}

// ParseTCX 解析指定路径的 TCX 文件，返回 Activity 记录。
func ParseTCX(path string, activityID string) (*db.Activity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 TCX 文件失败: %w", err)
	}

	var tcd tcxTrainingCenterDatabase
	if err := xml.Unmarshal(data, &tcd); err != nil {
		return nil, fmt.Errorf("解析 TCX XML 失败: %w", err)
	}

	if len(tcd.Activities) == 0 {
		return nil, fmt.Errorf("TCX 文件无活动数据: %s", path)
	}

	act := tcd.Activities[0]

	// 汇总所有 Lap 数据
	var totalDistance, totalTime float64
	var hrSum float64
	var hrCount int
	var allPoints []tcxTrackpoint
	var startTime, endTime time.Time

	for _, lap := range act.Laps {
		totalDistance += lap.Distance
		totalTime += lap.TotalTime

		// 收集轨迹点
		for _, track := range lap.Tracks {
			for _, pt := range track.Points {
				allPoints = append(allPoints, pt)
				// 解析心率
				if pt.HeartRate != nil && pt.HeartRate.Value > 0 {
					hrSum += pt.HeartRate.Value
					hrCount++
				}
			}
		}
	}

	if len(allPoints) == 0 {
		return nil, fmt.Errorf("TCX 文件无轨迹点: %s", path)
	}

	// 解析起止时间
	if t, err := time.Parse(time.RFC3339, allPoints[0].Time); err == nil {
		startTime = t
	}
	if t, err := time.Parse(time.RFC3339, allPoints[len(allPoints)-1].Time); err == nil {
		endTime = t
	}

	elapsedTime := int64(endTime.Sub(startTime).Seconds())
	if elapsedTime < 0 {
		elapsedTime = 0
	}
	movingTime := int64(totalTime)

	// 平均速度（米/秒）
	var avgSpeed float64
	if movingTime > 0 {
		avgSpeed = totalDistance / float64(movingTime)
	}

	// 编码 Polyline
	polylinePoints := make([][2]float64, 0, len(allPoints))
	for _, pt := range allPoints {
		if pt.Position != nil {
			polylinePoints = append(polylinePoints, [2]float64{
				pt.Position.Latitude,
				pt.Position.Longitude,
			})
		}
	}
	polyline := utils.EncodePolyline(polylinePoints)

	startLatLng := ""
	if len(polylinePoints) > 0 {
		startLatLng = fmt.Sprintf("[%f, %f]", polylinePoints[0][0], polylinePoints[0][1])
	}

	// 本地时间
	startLocal, _ := utils.AdjustTime(startTime, "Asia/Shanghai")
	endLocal, _ := utils.AdjustTime(endTime, "Asia/Shanghai")

	// 活动类型
	actType := act.Sport
	if actType == "" {
		actType = "Run"
	}

	actName := fmt.Sprintf("%s %s", actType, startLocal.Format("2006-01-02"))

	result := &db.Activity{
		ID:              activityID,
		Name:            actName,
		Type:            actType,
		Subtype:         actType,
		StartDate:       startTime.UTC().Format(time.RFC3339),
		End:             endTime.UTC().Format(time.RFC3339),
		StartDateLocal:  startLocal.Format(time.RFC3339),
		EndLocal:        endLocal.Format(time.RFC3339),
		Length:          totalDistance,
		Distance:        totalDistance,
		MovingTime:      movingTime,
		ElapsedTime:     elapsedTime,
		AverageSpeed:    avgSpeed,
		SummaryPolyline: polyline,
		StartLatLng:     startLatLng,
		LocationCountry: "",
	}

	if hrCount > 0 {
		avgHR := math.Round(hrSum/float64(hrCount)*10) / 10
		result.AverageHeartrate = &avgHR
	}

	return result, nil
}
