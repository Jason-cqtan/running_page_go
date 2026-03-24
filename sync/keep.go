// Package sync 提供 Keep App 数据同步客户端。
// 功能：登录、获取运动 ID 列表、Base64+AES-CBC 解密 geo 数据、坐标转换、生成 GPX。
package sync

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const keepLoginURL = "https://api.gotokeep.com/v1.1/users/login"
const keepDataURL = "https://api.gotokeep.com/pd/v3/%s/detail?date=0&limit=100&lastId=%s"
const keepActivityURL = "https://api.gotokeep.com/pd/v3/stats/detail?dateUnit=all&type=%s&lastId=%s"

// KeepClient Keep App API 客户端。
type KeepClient struct {
	token      string
	httpClient *http.Client
}

// GeoPoint 表示一个地理坐标点（已转换为 WGS84）。
type GeoPoint struct {
	Lat       float64
	Lng       float64
	Timestamp int64 // 毫秒时间戳
}

// NewKeepClient 登录 Keep，返回已认证的客户端。
func NewKeepClient(phone, password string) (*KeepClient, error) {
	payload := map[string]string{
		"mobile":   phone,
		"password": password,
	}
	data, _ := json.Marshal(payload)

	resp, err := http.Post(keepLoginURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("Keep 登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取登录响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Keep 登录失败，状态码 %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析登录响应失败: %w", err)
	}

	// 提取 token
	token := ""
	if data, ok := result["data"].(map[string]interface{}); ok {
		if tokenData, ok := data["token"].(map[string]interface{}); ok {
			token, _ = tokenData["access_token"].(string)
		}
	}
	if token == "" {
		return nil, fmt.Errorf("登录响应中未找到 access_token")
	}

	return &KeepClient{
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// doGet 执行带鉴权的 GET 请求。
func (c *KeepClient) doGet(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 返回错误 %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// GetSportIDs 获取指定运动类型的所有运动 ID 列表。
// sportType: "running", "hiking", "cycling" 等。
func (c *KeepClient) GetSportIDs(sportType string) ([]string, error) {
	var ids []string
	lastID := ""

	for {
		url := fmt.Sprintf(keepActivityURL, sportType, lastID)
		body, err := c.doGet(url)
		if err != nil {
			return nil, fmt.Errorf("获取 %s 列表失败: %w", sportType, err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("解析列表响应失败: %w", err)
		}

		data, ok := result["data"].(map[string]interface{})
		if !ok {
			break
		}

		records, _ := data["records"].([]interface{})
		if len(records) == 0 {
			break
		}

		for _, rec := range records {
			if item, ok := rec.(map[string]interface{}); ok {
				if id, ok := item["id"].(string); ok {
					ids = append(ids, id)
					lastID = id
				}
			}
		}

		// 检查是否还有更多页
		if hasMore, ok := data["has_more"].(bool); !ok || !hasMore {
			break
		}
	}

	return ids, nil
}

// keepRunDetail Keep 运动详情响应结构（关键字段）。
type keepRunDetail struct {
	StartTime int64  `json:"startTime"`
	Duration  int64  `json:"duration"`
	Distance  float64 `json:"distance"`
	RunMap    struct {
		IsGeo   bool   `json:"isGeo"`
		MapData string `json:"mapData"`
	} `json:"runMap"`
}

// GetRunDetail 获取单次运动详情，解析 geo 数据并返回坐标点。
func (c *KeepClient) GetRunDetail(sportType, activityID string) (*keepRunDetail, []GeoPoint, error) {
	url := fmt.Sprintf(keepDataURL, sportType, activityID)
	body, err := c.doGet(url)
	if err != nil {
		return nil, nil, fmt.Errorf("获取运动详情失败: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, nil, fmt.Errorf("解析详情响应失败: %w", err)
	}

	dataRaw, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("响应中无 data 字段")
	}

	// 重新序列化并解析为强类型结构
	dataBytes, _ := json.Marshal(dataRaw)
	var detail keepRunDetail
	if err := json.Unmarshal(dataBytes, &detail); err != nil {
		return nil, nil, fmt.Errorf("解析运动详情结构失败: %w", err)
	}

	// 解码 geo 数据
	if detail.RunMap.MapData == "" {
		return &detail, nil, nil
	}

	rawPoints, err := DecodeRunmapData(detail.RunMap.MapData, detail.RunMap.IsGeo)
	if err != nil {
		return &detail, nil, fmt.Errorf("解码地图数据失败: %w", err)
	}

	// 转换坐标 GCJ02 → WGS84
	var geoPoints []GeoPoint
	for _, pt := range rawPoints {
		lat, _ := pt["latitude"].(float64)
		lng, _ := pt["longitude"].(float64)
		ts, _ := pt["timestamp"].(float64)
		wLat, wLng := GCJ02ToWGS84(lat, lng)
		geoPoints = append(geoPoints, GeoPoint{
			Lat:       wLat,
			Lng:       wLng,
			Timestamp: int64(ts),
		})
	}

	return &detail, geoPoints, nil
}

// DecodeRunmapData 解码 Keep geo 数据：Base64 → AES-CBC 解密 → zlib 解压 → JSON 解析。
// 若 isGeo 为 false，数据只是 Base64 编码的 JSON，直接解析即可。
func DecodeRunmapData(text string, isGeo bool) ([]map[string]interface{}, error) {
	if !isGeo {
		// 非 geo 数据，直接解析 JSON 数组
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			decoded = []byte(text)
		}
		var points []map[string]interface{}
		if err := json.Unmarshal(decoded, &points); err != nil {
			return nil, fmt.Errorf("解析非 geo 数据失败: %w", err)
		}
		return points, nil
	}

	// 1. Base64 解码
	cipherData, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("Base64 解码失败: %w", err)
	}

	// 2. AES-CBC 解密（Keep App 使用的公开固定密钥和 IV，非用户敏感凭证）
	key := []byte("6d@!fE24Hy+R7qP8")
	iv := []byte("0102030405060708")

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 AES cipher 失败: %w", err)
	}

	if len(cipherData)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("密文长度不是块大小的整数倍")
	}

	plainData := make([]byte, len(cipherData))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plainData, cipherData)

	// 去除 PKCS7 填充
	plainData = pkcs7Unpad(plainData)

	// 3. zlib 解压
	r, err := zlib.NewReader(bytes.NewReader(plainData))
	if err != nil {
		return nil, fmt.Errorf("创建 zlib reader 失败: %w", err)
	}
	defer r.Close()

	jsonData, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("zlib 解压失败: %w", err)
	}

	// 4. 解析 JSON
	var points []map[string]interface{}
	if err := json.Unmarshal(jsonData, &points); err != nil {
		return nil, fmt.Errorf("解析 geo JSON 失败: %w", err)
	}
	return points, nil
}

// pkcs7Unpad 去除 PKCS7 填充。
func pkcs7Unpad(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	padLen := int(data[len(data)-1])
	if padLen > len(data) || padLen > aes.BlockSize {
		return data
	}
	return data[:len(data)-padLen]
}

// GCJ02ToWGS84 将火星坐标系（GCJ-02）转换为 WGS-84 坐标系。
// 算法参考：https://github.com/googollee/eviltransform
func GCJ02ToWGS84(lat, lng float64) (float64, float64) {
	if isOutOfChina(lat, lng) {
		return lat, lng
	}
	dLat, dLng := gcj02Delta(lat, lng)
	return lat - dLat, lng - dLng
}

// gcj02Delta 计算 GCJ02 偏移量。
func gcj02Delta(lat, lng float64) (float64, float64) {
	const a = 6378245.0
	const ee = 0.00669342162296594323

	dLat := transformLat(lng-105.0, lat-35.0)
	dLng := transformLng(lng-105.0, lat-35.0)

	radLat := lat / 180.0 * math.Pi
	magic := math.Sin(radLat)
	magic = 1 - ee*magic*magic
	sqrtMagic := math.Sqrt(magic)

	dLat = (dLat * 180.0) / ((a * (1 - ee)) / (magic * sqrtMagic) * math.Pi)
	dLng = (dLng * 180.0) / (a / sqrtMagic * math.Cos(radLat) * math.Pi)
	return dLat, dLng
}

func transformLat(x, y float64) float64 {
	ret := -100.0 + 2.0*x + 3.0*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(y*math.Pi) + 40.0*math.Sin(y/3.0*math.Pi)) * 2.0 / 3.0
	ret += (160.0*math.Sin(y/12.0*math.Pi) + 320*math.Sin(y*math.Pi/30.0)) * 2.0 / 3.0
	return ret
}

func transformLng(x, y float64) float64 {
	ret := 300.0 + x + 2.0*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(x*math.Pi) + 40.0*math.Sin(x/3.0*math.Pi)) * 2.0 / 3.0
	ret += (150.0*math.Sin(x/12.0*math.Pi) + 300.0*math.Sin(x/30.0*math.Pi)) * 2.0 / 3.0
	return ret
}

func isOutOfChina(lat, lng float64) bool {
	return lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271
}

// GenerateGPX 根据坐标点列表生成 GPX XML 字符串。
func GenerateGPX(points []GeoPoint, startTime int64, sportType string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<gpx version="1.1" creator="running_page_go"`)
	sb.WriteString(` xmlns="http://www.topografix.com/GPX/1/1"`)
	sb.WriteString(` xmlns:gpxtpx="http://www.garmin.com/xmlschemas/TrackPointExtension/v1">`)
	sb.WriteString("\n")

	// metadata
	t := time.Unix(startTime/1000, 0).UTC()
	sb.WriteString(fmt.Sprintf("  <metadata><time>%s</time></metadata>\n", t.Format(time.RFC3339)))

	sb.WriteString("  <trk>\n")
	sb.WriteString(fmt.Sprintf("    <name>%s %s</name>\n", sportType, t.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("    <type>%s</type>\n", sportType))
	sb.WriteString("    <trkseg>\n")

	for _, pt := range points {
		ts := time.Unix(pt.Timestamp/1000, 0).UTC()
		sb.WriteString(fmt.Sprintf(
			`      <trkpt lat="%f" lon="%f"><time>%s</time></trkpt>`+"\n",
			pt.Lat, pt.Lng, ts.Format(time.RFC3339),
		))
	}

	sb.WriteString("    </trkseg>\n")
	sb.WriteString("  </trk>\n")
	sb.WriteString("</gpx>\n")
	return sb.String()
}
