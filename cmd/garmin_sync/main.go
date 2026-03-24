// garmin_sync 是 Garmin Connect 数据同步 CLI 工具。
// 从 Garmin Connect 拉取活动列表并下载 GPX/TCX/FIT 文件，然后更新 SQLite 数据库和 activities.json。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Jason-cqtan/running_page_go/config"
	"github.com/Jason-cqtan/running_page_go/db"
	"github.com/Jason-cqtan/running_page_go/generator"
	syncp "github.com/Jason-cqtan/running_page_go/sync"
	"github.com/Jason-cqtan/running_page_go/utils"
)

func main() {
	secret := flag.String("secret", "", "garth secret string（JSON 格式，必填）")
	cn := flag.Bool("cn", false, "使用 Garmin 中国服务器（connectapi.garmin.cn）")
	onlyRun := flag.Bool("only-run", false, "只同步跑步活动")
	fileType := flag.String("type", "gpx", "下载文件格式：gpx/tcx/fit")
	flag.Parse()

	if *secret == "" {
		fmt.Fprintln(os.Stderr, "错误：请通过 --secret 提供 garth secret string")
		flag.Usage()
		os.Exit(1)
	}

	// 确定 Garmin 域名
	authDomain := syncp.GarminDomainCOM
	if *cn {
		authDomain = syncp.GarminDomainCN
	}

	// 确定输出目录
	folder, ok := config.FolderDict[*fileType]
	if !ok {
		fmt.Fprintf(os.Stderr, "错误：不支持的文件类型 %q，请使用 gpx/tcx/fit\n", *fileType)
		os.Exit(1)
	}

	// 获取已下载的活动 ID
	downloadedIDs := utils.GetDownloadedIDs(folder)
	fmt.Printf("已下载 %d 个活动文件\n", len(downloadedIDs))

	// 创建 Garmin 客户端
	client, err := syncp.NewGarminClient(*secret, authDomain)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建 Garmin 客户端失败: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// 获取所有活动 ID
	fmt.Println("正在获取活动列表...")
	allIDs, err := client.GetAllActivityIDs(ctx, *onlyRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取活动列表失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("共找到 %d 个活动\n", len(allIDs))

	// 并发下载新活动
	_, err = syncp.DownloadNewActivities(ctx, client, allIDs, downloadedIDs, folder, *fileType, config.MaxConcurrentDownloads)
	if err != nil {
		fmt.Fprintf(os.Stderr, "下载活动失败: %v\n", err)
		os.Exit(1)
	}

	// 打开数据库
	database, err := db.New(config.SQLFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// 扫描文件目录，同步到数据库
	gen := generator.New(database)
	suffix := "." + *fileType
	if err := gen.SyncFromDataDir(folder, suffix, nil); err != nil {
		fmt.Fprintf(os.Stderr, "同步数据失败: %v\n", err)
		os.Exit(1)
	}

	// 生成 activities.json
	if err := gen.SaveActivitiesJSON(config.JSONFile); err != nil {
		fmt.Fprintf(os.Stderr, "生成 JSON 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Garmin 同步完成！")
}
