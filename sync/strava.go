// Package sync 提供 Strava OAuth2 客户端，支持分页获取所有活动。
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const stravaTokenURL = "https://www.strava.com/oauth/token"
const stravaAPIBase = "https://www.strava.com/api/v3"

// StravaToken 是 Strava OAuth2 令牌响应。
type StravaToken struct {
	TokenType    string `json:"token_type"`
	AccessToken  string `json:"access_token"`
	ExpiresAt    int64  `json:"expires_at"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Athlete      struct {
		ID int64 `json:"id"`
	} `json:"athlete"`
}

// StravaActivity 对应 Strava API 返回的活动字段。
type StravaActivity struct {
	ID                 int64          `json:"id"`
	Name               string         `json:"name"`
	Type               string         `json:"type"`
	SportType          string         `json:"sport_type"`
	StartDate          time.Time      `json:"start_date"`
	StartDateLocal     time.Time      `json:"start_date_local"`
	Distance           float64        `json:"distance"`
	MovingTime         int64          `json:"moving_time"`
	ElapsedTime        int64          `json:"elapsed_time"`
	AverageSpeed       float64        `json:"average_speed"`
	AverageHeartrate   *float64       `json:"average_heartrate"`
	TotalElevationGain *float64       `json:"total_elevation_gain"`
	Map                StravaMap      `json:"map"`
	StartLatlng        []float64      `json:"start_latlng"`
	LocationCountry    string         `json:"location_country"`
}

// StravaMap 包含 Strava 活动地图数据。
type StravaMap struct {
	ID              string `json:"id"`
	SummaryPolyline string `json:"summary_polyline"`
	ResourceState   int    `json:"resource_state"`
}

// RefreshStravaToken 用 refresh_token 换取新的 access_token。
func RefreshStravaToken(clientID, clientSecret, refreshToken string) (*StravaToken, error) {
	params := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	resp, err := http.Post(stravaTokenURL, "application/x-www-form-urlencoded",
		strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("刷新 Strava token 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("刷新 token 失败，状态码 %d: %s", resp.StatusCode, string(body))
	}

	var token StravaToken
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("解析 token 响应失败: %w", err)
	}
	return &token, nil
}

// GetAllActivities 分页获取 afterTimestamp（Unix 时间戳）之后的所有活动。
// afterTimestamp 为 0 表示获取全部。
func GetAllActivities(ctx context.Context, accessToken string, afterTimestamp int64) ([]StravaActivity, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	var all []StravaActivity
	page := 1
	perPage := 100

	for {
		reqURL := fmt.Sprintf(
			"%s/athlete/activities?per_page=%d&page=%d",
			stravaAPIBase, perPage, page,
		)
		if afterTimestamp > 0 {
			reqURL += fmt.Sprintf("&after=%d", afterTimestamp)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("创建请求失败: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求 Strava API 失败（第 %d 页）: %w", page, err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Strava API 错误，状态码 %d: %s", resp.StatusCode, string(body))
		}

		var activities []StravaActivity
		if err := json.Unmarshal(body, &activities); err != nil {
			return nil, fmt.Errorf("解析活动列表失败（第 %d 页）: %w", page, err)
		}

		if len(activities) == 0 {
			break
		}

		all = append(all, activities...)

		if len(activities) < perPage {
			break // 已到最后一页
		}
		page++
	}

	return all, nil
}

// ToDBActivity 将 StravaActivity 转换为 db.Activity。
func (a *StravaActivity) ToDBActivity() interface{} {
	// 返回 interface{} 避免循环依赖；调用方自行转换
	return a
}
