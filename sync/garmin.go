// Package sync 提供 Garmin Connect API 客户端，支持并发下载活动文件。
package sync

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Garmin Connect API 域名
const (
	GarminDomainCOM = "connectapi.garmin.com"
	GarminDomainCN  = "connectapi.garmin.cn"
)

// garthToken 是 garth 格式 secret_string 的 JSON 结构。
type garthToken struct {
	OAuth2Token struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	} `json:"oauth2_token"`
}

// GarminClient Garmin Connect API 客户端。
type GarminClient struct {
	accessToken string
	domain      string
	httpClient  *http.Client
	limiter     *rate.Limiter
}

// NewGarminClient 解析 garth secret_string，创建 GarminClient。
// authDomain 可为 GarminDomainCOM 或 GarminDomainCN。
func NewGarminClient(secretString string, authDomain string) (*GarminClient, error) {
	var token garthToken
	if err := json.Unmarshal([]byte(secretString), &token); err != nil {
		return nil, fmt.Errorf("解析 garth secret_string 失败: %w", err)
	}
	if token.OAuth2Token.AccessToken == "" {
		return nil, fmt.Errorf("secret_string 中未找到 oauth2_token.access_token")
	}
	if authDomain == "" {
		authDomain = GarminDomainCOM
	}
	return &GarminClient{
		accessToken: token.OAuth2Token.AccessToken,
		domain:      authDomain,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		// 限流：每秒最多 5 个请求（突发上限同为 5），防止触发 Garmin API 限速
		limiter: rate.NewLimiter(rate.Every(200*time.Millisecond), 5),
	}, nil
}

// apiURL 构造完整 API URL。
func (c *GarminClient) apiURL(path string) string {
	return fmt.Sprintf("https://%s%s", c.domain, path)
}

// doRequest 执行带鉴权头的 GET 请求，返回响应体字节。
func (c *GarminClient) doRequest(ctx context.Context, url string) ([]byte, error) {
	// 限流等待
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("DI-Backend", "connectapi.garmin.com")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误状态 %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// activityListResponse Garmin 活动列表 API 响应结构。
type activityListResponse []struct {
	ActivityID int64 `json:"activityId"`
	ActivityType struct {
		TypeKey string `json:"typeKey"`
	} `json:"activityType"`
}

// GetAllActivityIDs 递归分页获取所有活动 ID。
// 若 isOnlyRun 为 true，则只返回 typeKey 中含 "running" 的活动。
func (c *GarminClient) GetAllActivityIDs(ctx context.Context, isOnlyRun bool) ([]string, error) {
	var allIDs []string
	start := 0
	limit := 100

	for {
		url := c.apiURL(fmt.Sprintf(
			"/activity-service/activity/search/activities?start=%d&limit=%d",
			start, limit,
		))

		body, err := c.doRequest(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("获取活动列表失败（start=%d）: %w", start, err)
		}

		var list activityListResponse
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, fmt.Errorf("解析活动列表失败: %w", err)
		}

		if len(list) == 0 {
			break // 已无更多数据
		}

		for _, item := range list {
			if isOnlyRun {
				typeKey := item.ActivityType.TypeKey
				if typeKey != "running" && typeKey != "track_running" && typeKey != "treadmill_running" {
					continue
				}
			}
			allIDs = append(allIDs, fmt.Sprintf("%d", item.ActivityID))
		}

		if len(list) < limit {
			break // 已到最后一页
		}
		start += limit
	}

	return allIDs, nil
}

// DownloadActivity 下载单个活动文件（gpx/tcx/fit），返回文件字节内容。
// FIT 文件以 ZIP 压缩包形式返回，此函数自动解压。
func (c *GarminClient) DownloadActivity(ctx context.Context, activityID, fileType string) ([]byte, error) {
	var url string
	switch fileType {
	case "gpx":
		url = c.apiURL(fmt.Sprintf("/download-service/export/gpx/activity/%s", activityID))
	case "tcx":
		url = c.apiURL(fmt.Sprintf("/download-service/export/tcx/activity/%s", activityID))
	case "fit":
		url = c.apiURL(fmt.Sprintf("/download-service/files/activity/%s", activityID))
	default:
		return nil, fmt.Errorf("不支持的文件类型: %s", fileType)
	}

	data, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	// FIT 文件以 ZIP 格式返回，需要解压
	if fileType == "fit" {
		data, err = unzipFirst(data)
		if err != nil {
			return nil, fmt.Errorf("解压 FIT ZIP 失败: %w", err)
		}
	}

	return data, nil
}

// unzipFirst 从 ZIP 字节中提取第一个文件的内容。
func unzipFirst(data []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(r.File) == 0 {
		return nil, fmt.Errorf("ZIP 文件为空")
	}
	f, err := r.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// DownloadNewActivities 并发（最多 MaxConcurrent 个）下载尚未下载的活动文件。
// downloadedIDs：已下载的活动 ID 集合（不重复下载）。
// 返回新下载的活动 ID 列表。
func DownloadNewActivities(
	ctx context.Context,
	client *GarminClient,
	allIDs []string,
	downloadedIDs map[string]bool,
	folder, fileType string,
	maxConcurrent int,
) ([]string, error) {
	// 过滤出未下载的 ID
	var toDownload []string
	for _, id := range allIDs {
		if !downloadedIDs[id] {
			toDownload = append(toDownload, id)
		}
	}

	if len(toDownload) == 0 {
		fmt.Println("没有新活动需要下载")
		return nil, nil
	}

	fmt.Printf("需要下载 %d 个新活动\n", len(toDownload))

	// 确保输出目录存在
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 使用 semaphore channel 控制并发数
	sem := make(chan struct{}, maxConcurrent)
	var mu sync.Mutex
	var downloaded []string
	var wg sync.WaitGroup
	errCh := make(chan error, len(toDownload))

	for _, id := range toDownload {
		wg.Add(1)
		id := id // 避免闭包捕获
		go func() {
			defer wg.Done()
			sem <- struct{}{}        // 获取 semaphore
			defer func() { <-sem }() // 释放 semaphore

			data, err := client.DownloadActivity(ctx, id, fileType)
			if err != nil {
				errCh <- fmt.Errorf("下载活动 %s 失败: %w", id, err)
				return
			}

			outPath := filepath.Join(folder, id+"."+fileType)
			if err := os.WriteFile(outPath, data, 0o644); err != nil {
				errCh <- fmt.Errorf("保存文件 %s 失败: %w", outPath, err)
				return
			}

			mu.Lock()
			downloaded = append(downloaded, id)
			mu.Unlock()
			fmt.Printf("已下载: %s.%s\n", id, fileType)
		}()
	}

	wg.Wait()
	close(errCh)

	// 收集错误（打印警告但不中断）
	for err := range errCh {
		fmt.Fprintf(os.Stderr, "警告: %v\n", err)
	}

	return downloaded, nil
}
