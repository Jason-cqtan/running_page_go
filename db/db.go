// Package db 封装 SQLite 数据库操作（使用 modernc.org/sqlite，纯 Go，无 CGO）。
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // 注册 sqlite 驱动
)

// Activity 表示一条运动活动记录，与 activities.json 字段对应。
type Activity struct {
	ID               string
	Name             string
	Type             string
	Subtype          string
	StartDate        string
	End              string
	StartDateLocal   string
	EndLocal         string
	Length           float64
	Distance         float64
	MovingTime       int64
	ElapsedTime      int64
	AverageSpeed     float64
	AverageHeartrate *float64 // 可空
	ElevationGain    *float64 // 可空
	SummaryPolyline  string
	StartLatLng      string
	LocationCountry  string
}

// DB 封装 SQLite 数据库连接。
type DB struct {
	conn *sql.DB
}

// New 打开（或创建）指定路径的 SQLite 数据库文件，返回 DB 实例。
func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	d := &DB{conn: conn}
	if err := d.CreateTable(); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

// Close 关闭数据库连接。
func (d *DB) Close() error {
	return d.conn.Close()
}

// CreateTable 创建 activities 表（若不存在）。
func (d *DB) CreateTable() error {
	_, err := d.conn.Exec(`
CREATE TABLE IF NOT EXISTS activities (
    id                TEXT PRIMARY KEY,
    name              TEXT,
    type              TEXT,
    subtype           TEXT,
    start_date        TEXT,
    end_date          TEXT,
    start_date_local  TEXT,
    end_local         TEXT,
    length            REAL,
    distance          REAL,
    moving_time       INTEGER,
    elapsed_time      INTEGER,
    average_speed     REAL,
    average_heartrate REAL,
    elevation_gain    REAL,
    summary_polyline  TEXT,
    start_latlng      TEXT,
    location_country  TEXT
)`)
	if err != nil {
		return fmt.Errorf("创建表失败: %w", err)
	}
	return nil
}

// InsertOrUpdate 插入或更新一条活动记录。
func (d *DB) InsertOrUpdate(a *Activity) error {
	_, err := d.conn.Exec(`
INSERT INTO activities (
    id, name, type, subtype, start_date, end_date, start_date_local, end_local,
    length, distance, moving_time, elapsed_time, average_speed,
    average_heartrate, elevation_gain, summary_polyline, start_latlng, location_country
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
    name=excluded.name,
    type=excluded.type,
    subtype=excluded.subtype,
    start_date=excluded.start_date,
    end_date=excluded.end_date,
    start_date_local=excluded.start_date_local,
    end_local=excluded.end_local,
    length=excluded.length,
    distance=excluded.distance,
    moving_time=excluded.moving_time,
    elapsed_time=excluded.elapsed_time,
    average_speed=excluded.average_speed,
    average_heartrate=excluded.average_heartrate,
    elevation_gain=excluded.elevation_gain,
    summary_polyline=excluded.summary_polyline,
    start_latlng=excluded.start_latlng,
    location_country=excluded.location_country`,
		a.ID, a.Name, a.Type, a.Subtype,
		a.StartDate, a.End, a.StartDateLocal, a.EndLocal,
		a.Length, a.Distance, a.MovingTime, a.ElapsedTime,
		a.AverageSpeed, a.AverageHeartrate, a.ElevationGain,
		a.SummaryPolyline, a.StartLatLng, a.LocationCountry,
	)
	if err != nil {
		return fmt.Errorf("写入活动 %s 失败: %w", a.ID, err)
	}
	return nil
}

// GetAll 查询所有活动，按开始时间降序返回。
func (d *DB) GetAll() ([]*Activity, error) {
	rows, err := d.conn.Query(`
SELECT id, name, type, subtype, start_date, end_date, start_date_local, end_local,
       length, distance, moving_time, elapsed_time, average_speed,
       average_heartrate, elevation_gain, summary_polyline, start_latlng, location_country
FROM activities
ORDER BY start_date DESC`)
	if err != nil {
		return nil, fmt.Errorf("查询活动失败: %w", err)
	}
	defer rows.Close()

	var activities []*Activity
	for rows.Next() {
		a := &Activity{}
		if err := rows.Scan(
			&a.ID, &a.Name, &a.Type, &a.Subtype,
			&a.StartDate, &a.End, &a.StartDateLocal, &a.EndLocal,
			&a.Length, &a.Distance, &a.MovingTime, &a.ElapsedTime,
			&a.AverageSpeed, &a.AverageHeartrate, &a.ElevationGain,
			&a.SummaryPolyline, &a.StartLatLng, &a.LocationCountry,
		); err != nil {
			return nil, fmt.Errorf("扫描行失败: %w", err)
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}

// GetExistingIDs 返回数据库中已存在的所有活动 ID 集合。
func (d *DB) GetExistingIDs() (map[string]bool, error) {
	rows, err := d.conn.Query("SELECT id FROM activities")
	if err != nil {
		return nil, fmt.Errorf("查询 ID 失败: %w", err)
	}
	defer rows.Close()

	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("扫描 ID 失败: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}
