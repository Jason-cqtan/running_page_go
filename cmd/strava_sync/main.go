// strava_sync 是 Strava 数据同步 CLI 工具。
// 通过 OAuth2 refresh token 获取 access token，拉取活动列表并写入 SQLite 和 activities.json。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Jason-cqtan/running_page_go/config"
	"github.com/Jason-cqtan/running_page_go/db"
	"github.com/Jason-cqtan/running_page_go/generator"
	syncp "github.com/Jason-cqtan/running_page_go/sync"
)

func main() {
	clientID := flag.String("client-id", "", "Strava Client ID（必填）")
	clientSecret := flag.String("client-secret", "", "Strava Client Secret（必填）")
	refreshToken := flag.String("refresh-token", "", "Strava Refresh Token（必填）")
	onlyRun := flag.Bool("only-run", false, "只同步跑步活动")
	afterStr := flag.String("after", "", "只获取该时间之后的活动（格式：2006-01-02，可选）")
	flag.Parse()

	// 参数校验
	if *clientID == "" || *clientSecret == "" || *refreshToken == "" {
		fmt.Fprintln(os.Stderr, "错误：--client-id、--client-secret、--refresh-token 均为必填项")
		flag.Usage()
		os.Exit(1)
	}

	// 刷新 access token
	fmt.Println("正在刷新 Strava access token...")
	token, err := syncp.RefreshStravaToken(*clientID, *clientSecret, *refreshToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "刷新 token 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Token 有效期至: %s\n", time.Unix(token.ExpiresAt, 0).Format(time.RFC3339))

	// 解析 after 时间戳
	var afterTS int64
	if *afterStr != "" {
		t, err := time.Parse("2006-01-02", *afterStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "解析 --after 日期失败: %v\n", err)
			os.Exit(1)
		}
		afterTS = t.Unix()
	}

	// 获取活动列表
	fmt.Println("正在获取 Strava 活动列表...")
	ctx := context.Background()
	activities, err := syncp.GetAllActivities(ctx, token.AccessToken, afterTS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取活动列表失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("共获取 %d 个活动\n", len(activities))

	// 打开数据库
	database, err := db.New(config.SQLFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// 写入数据库
	saved := 0
	for _, act := range activities {
		// 若只同步跑步，跳过其他类型
		if *onlyRun && act.SportType != "Run" && act.Type != "Run" {
			continue
		}

		actType := act.SportType
		if actType == "" {
			actType = act.Type
		}

		startLatLng := ""
		if len(act.StartLatlng) >= 2 {
			startLatLng = fmt.Sprintf("[%f, %f]", act.StartLatlng[0], act.StartLatlng[1])
		}

		dbAct := &db.Activity{
			ID:              fmt.Sprintf("%d", act.ID),
			Name:            act.Name,
			Type:            actType,
			Subtype:         actType,
			StartDate:       act.StartDate.UTC().Format(time.RFC3339),
			End:             act.StartDate.Add(time.Duration(act.ElapsedTime) * time.Second).UTC().Format(time.RFC3339),
			StartDateLocal:  act.StartDateLocal.Format(time.RFC3339),
			EndLocal:        act.StartDateLocal.Add(time.Duration(act.ElapsedTime) * time.Second).Format(time.RFC3339),
			Length:          act.Distance,
			Distance:        act.Distance,
			MovingTime:      act.MovingTime,
			ElapsedTime:     act.ElapsedTime,
			AverageSpeed:    act.AverageSpeed,
			AverageHeartrate: act.AverageHeartrate,
			ElevationGain:   act.TotalElevationGain,
			SummaryPolyline:  act.Map.SummaryPolyline,
			StartLatLng:      startLatLng,
			LocationCountry:  act.LocationCountry,
		}

		if err := database.InsertOrUpdate(dbAct); err != nil {
			fmt.Fprintf(os.Stderr, "警告：写入活动 %d 失败: %v\n", act.ID, err)
			continue
		}
		saved++
	}
	fmt.Printf("已写入 %d 个活动到数据库\n", saved)

	// 生成 activities.json
	gen := generator.New(database)
	if err := gen.SaveActivitiesJSON(config.JSONFile); err != nil {
		fmt.Fprintf(os.Stderr, "生成 JSON 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Strava 同步完成！")
}
