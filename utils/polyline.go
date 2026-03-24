// Package utils 提供 Google Polyline 编解码实现（与 Python polyline 库兼容）。
package utils

import (
	"math"
	"strings"
)

// EncodePolyline 将坐标点序列编码为 Google Polyline 字符串。
// points 中每个元素为 [2]float64{lat, lng}。
func EncodePolyline(points [][2]float64) string {
	var sb strings.Builder
	prevLat, prevLng := 0, 0

	for _, p := range points {
		lat := int(math.Round(p[0] * 1e5))
		lng := int(math.Round(p[1] * 1e5))

		encodeValue(&sb, lat-prevLat)
		encodeValue(&sb, lng-prevLng)

		prevLat = lat
		prevLng = lng
	}
	return sb.String()
}

// encodeValue 编码单个整数值并写入 strings.Builder。
func encodeValue(sb *strings.Builder, value int) {
	// 左移 1 位，若为负数则取反
	v := value << 1
	if value < 0 {
		v = ^v
	}
	// 每 5 位一组，最高位标记是否还有后续块
	for v >= 0x20 {
		sb.WriteByte(byte((0x20|(v&0x1f)) + 63))
		v >>= 5
	}
	sb.WriteByte(byte(v + 63))
}

// DecodePolyline 将 Google Polyline 字符串解码为坐标点序列。
func DecodePolyline(encoded string) [][2]float64 {
	var points [][2]float64
	index := 0
	lat, lng := 0, 0

	for index < len(encoded) {
		// 解码纬度
		dlat, n := decodeValue(encoded, index)
		index += n
		lat += dlat

		// 解码经度
		dlng, n := decodeValue(encoded, index)
		index += n
		lng += dlng

		points = append(points, [2]float64{
			float64(lat) / 1e5,
			float64(lng) / 1e5,
		})
	}
	return points
}

// decodeValue 从 encoded[index:] 解码一个整数，返回（值, 消耗字节数）。
func decodeValue(encoded string, index int) (int, int) {
	result := 0
	shift := 0
	n := 0
	for index+n < len(encoded) {
		b := int(encoded[index+n]) - 63
		n++
		result |= (b & 0x1f) << shift
		shift += 5
		if b < 0x20 {
			break
		}
	}
	// 还原符号位
	if result&1 != 0 {
		result = ^result
	}
	result >>= 1
	return result, n
}
