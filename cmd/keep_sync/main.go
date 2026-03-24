// keep_sync 是 Keep App 数据同步 CLI 工具。
// 登录 Keep，获取运动记录，解密 geo 数据，坐标转换后生成 GPX 文件，并更新 SQLite 和 activities.json。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jason-cqtan/running_page_go/config"
	"github.com/Jason-cqtan/running_page_go/db"
	"github.com/Jason-cqtan/running_page_go/generator"
	syncp "github.com/Jason-cqtan/running_page_go/sync"
	"github.com/Jason-cqtan/running_page_go/utils"
)

func main() {
	phone := flag.String("phone", "", "Keep 手机号（必填）")
	password := flag.String("password", "", "Keep 密码（必填）")
	syncTypes := flag.String("sync-types", "running", "要同步的运动类型，逗号分隔（如：running,hiking,cycling）")
	withGPX := flag.Bool("with-gpx", true, "是否保存 GPX 文件到 GPX_OUT 目录")
	flag.Parse()

	if *phone == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "错误：--phone 和 --password 均为必填项")
		flag.Usage()
		os.Exit(1)
	}

	// 登录 Keep
	fmt.Println("正在登录 Keep...")
	client, err := syncp.NewKeepClient(*phone, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "登录失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("登录成功")

	// 打开数据库
	database, err := db.New(config.SQLFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// 获取已存在的活动 ID（避免重复同步）
	existingIDs, err := database.GetExistingIDs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取已有活动 ID 失败: %v\n", err)
		os.Exit(1)
	}

	types := strings.Split(*syncTypes, ",")
	totalSaved := 0

	for _, sportType := range types {
		sportType = strings.TrimSpace(sportType)
		if sportType == "" {
			continue
		}
		fmt.Printf("正在获取 %s 活动列表...\n", sportType)

		ids, err := client.GetSportIDs(sportType)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告：获取 %s 列表失败: %v\n", sportType, err)
			continue
		}
		fmt.Printf("共找到 %d 个 %s 活动\n", len(ids), sportType)

		for _, id := range ids {
			if existingIDs[id] {
				continue // 已存在，跳过
			}

			detail, geoPoints, err := client.GetRunDetail(sportType, id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "警告：获取活动 %s 详情失败: %v\n", id, err)
				continue
			}

			startTime := time.Unix(detail.StartTime/1000, 0).UTC()
			endTime := startTime.Add(time.Duration(detail.Duration) * time.Millisecond)
			startLocal, _ := utils.AdjustTime(startTime, config.BaseTimezone)

			// 构建 polyline
			polyline := ""
			startLatLng := ""
			if len(geoPoints) > 0 {
				pts := make([][2]float64, len(geoPoints))
				for i, p := range geoPoints {
					pts[i] = [2]float64{p.Lat, p.Lng}
				}
				polyline = utils.EncodePolyline(pts)
				startLatLng = fmt.Sprintf("[%f, %f]", geoPoints[0].Lat, geoPoints[0].Lng)
			}

			// 将运动类型首字母大写（如 "running" → "Running"）
		actType := strings.ToUpper(sportType[:1]) + sportType[1:]
			movingTime := detail.Duration / 1000

			var avgSpeed float64
			if movingTime > 0 {
				avgSpeed = detail.Distance / float64(movingTime)
			}

			dbAct := &db.Activity{
				ID:              id,
				Name:            fmt.Sprintf("%s %s", actType, startLocal.Format("2006-01-02")),
				Type:            actType,
				Subtype:         actType,
				StartDate:       startTime.Format(time.RFC3339),
				End:             endTime.Format(time.RFC3339),
				StartDateLocal:  startLocal.Format(time.RFC3339),
				EndLocal:        startLocal.Add(time.Duration(detail.Duration) * time.Millisecond).Format(time.RFC3339),
				Length:          detail.Distance,
				Distance:        detail.Distance,
				MovingTime:      movingTime,
				ElapsedTime:     movingTime,
				AverageSpeed:    avgSpeed,
				SummaryPolyline: polyline,
				StartLatLng:     startLatLng,
				LocationCountry: "",
			}

			if err := database.InsertOrUpdate(dbAct); err != nil {
				fmt.Fprintf(os.Stderr, "警告：写入活动 %s 失败: %v\n", id, err)
				continue
			}
			totalSaved++

			// 保存 GPX 文件
			if *withGPX && len(geoPoints) > 0 {
				if err := os.MkdirAll(config.GPXFolder, 0o755); err == nil {
					gpxContent := syncp.GenerateGPX(geoPoints, detail.StartTime, actType)
					gpxPath := filepath.Join(config.GPXFolder, id+".gpx")
					if err := os.WriteFile(gpxPath, []byte(gpxContent), 0o644); err != nil {
						fmt.Fprintf(os.Stderr, "警告：保存 GPX 文件失败: %v\n", err)
					} else {
						fmt.Printf("已保存 GPX: %s\n", gpxPath)
					}
				}
			}
			fmt.Printf("已同步: %s (%s)\n", dbAct.Name, id)
		}
	}

	fmt.Printf("共同步 %d 个新活动\n", totalSaved)

	// 生成 activities.json
	gen := generator.New(database)
	if err := gen.SaveActivitiesJSON(config.JSONFile); err != nil {
		fmt.Fprintf(os.Stderr, "生成 JSON 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Keep 同步完成！")
}
