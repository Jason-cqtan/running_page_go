// Package utils 提供时区处理和文件目录工具函数。
package utils

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AdjustTime 将时间 t 转换到指定时区，返回该时区的本地时间。
func AdjustTime(t time.Time, tzName string) (time.Time, error) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return t, err
	}
	return t.In(loc), nil
}

// AdjustTimeToUTC 先将 t 解释为 tzName 时区的本地时间，再转换为 UTC 返回。
func AdjustTimeToUTC(t time.Time, tzName string) (time.Time, error) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return t, err
	}
	// 使用 time.Date 重新构造，以避免已带有时区信息的 t 被二次转换
	local := time.Date(
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
		loc,
	)
	return local.UTC(), nil
}

// GetDownloadedIDs 扫描 folder 目录，返回所有文件（去扩展名）的 ID 集合。
// 例如目录中存在 "12345678.gpx"，则返回 {"12345678": true}。
func GetDownloadedIDs(folder string) map[string]bool {
	ids := make(map[string]bool)
	entries, err := os.ReadDir(folder)
	if err != nil {
		return ids
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 去掉扩展名作为 ID
		ext := filepath.Ext(name)
		id := strings.TrimSuffix(name, ext)
		if id != "" && id != ".gitkeep" {
			ids[id] = true
		}
	}
	return ids
}
