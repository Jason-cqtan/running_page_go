// Package config 定义项目中所有文件路径常量和全局配置。
package config

import (
	"os"
	"path/filepath"
)

// 项目根目录（以可执行文件所在目录为基准）
var rootDir string

func init() {
	exe, err := os.Executable()
	if err != nil {
		// 回退到当前工作目录
		rootDir, _ = os.Getwd()
	} else {
		// 解析软链接，获取真实路径
		real, err := filepath.EvalSymlinks(exe)
		if err != nil {
			real = exe
		}
		rootDir = filepath.Dir(real)
	}
}

// RootDir 返回项目根目录路径。
func RootDir() string {
	return rootDir
}

// 文件路径常量（相对项目根目录）
var (
	GPXFolder  = filepath.Join(rootDir, "GPX_OUT")
	TCXFolder  = filepath.Join(rootDir, "TCX_OUT")
	FITFolder  = filepath.Join(rootDir, "FIT_OUT")
	SQLFile    = filepath.Join(rootDir, "data.db")
	JSONFile   = filepath.Join(rootDir, "activities", "activities.json")
)

// FolderDict 将文件类型映射到对应的输出目录。
var FolderDict = map[string]string{
	"gpx": GPXFolder,
	"tcx": TCXFolder,
	"fit": FITFolder,
}

// BaseTimezone 默认时区。
const BaseTimezone = "Asia/Shanghai"

// MaxConcurrentDownloads 最大并发下载数。
const MaxConcurrentDownloads = 10
