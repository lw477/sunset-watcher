package utils

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// Point 表示 WGS 84 (SRID 4326) 经纬度点

type Point struct {
	Lng float64 `json:"lng"`
	Lat float64 `json:"lat"`
}

// GormDataType 1. 指定 GORM 映射的数据库底层类型
func (p Point) GormDataType() string {
	return "geometry"
}

// GormDBDataType 2. 指定具体的字段参数（用于 AutoMigrate）
func (p Point) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "geometry(Point, 4326)"
	case "mysql":
		return "POINT"
	}
	return "geometry"
}

// GormValue writes coordinates through PostGIS so PostgreSQL does not need to
// infer how to encode a Go struct as a geometry parameter.
func (p Point) GormValue(_ context.Context, _ *gorm.DB) clause.Expr {
	return clause.Expr{
		SQL:  "ST_SetSRID(ST_MakePoint(?, ?), 4326)",
		Vars: []interface{}{p.Lng, p.Lat},
	}
}

// Scan 3. 实现 sql.Scanner: 从数据库读出时解析
func (p *Point) Scan(val interface{}) error {
	if val == nil {
		*p = Point{}
		return nil
	}

	var data []byte
	switch v := val.(type) {
	case []byte:
		data = v
	case string:
		b, err := hex.DecodeString(v)
		if err != nil {
			return fmt.Errorf("failed to decode hex EWKB: %w", err)
		}
		data = b
	default:
		return errors.New("unsupported data format for PostGIS Point")
	}
	return p.decodeEWKB(data)
}

// 内部函数：解析 PostGIS EWKB 二进制字节流
func (p *Point) decodeEWKB(data []byte) error {
	if len(data) < 21 {
		return errors.New("invalid EWKB data length")
	}
	// 读取字节序（0 为 Big-Endian, 1 为 Little-Endian）
	var byteOrder binary.ByteOrder
	if data[0] == 0 {
		byteOrder = binary.BigEndian
	} else if data[0] == 1 {
		byteOrder = binary.LittleEndian
	} else {
		return errors.New("invalid EWKB byte order")
	}

	// 偏移 1 字节后为类型标识（WKB/EWKB Type）
	geomType := byteOrder.Uint32(data[1:5])
	if geomType&0x0fffffff != 1 {
		return fmt.Errorf("unsupported EWKB geometry type: %d", geomType&0x0fffffff)
	}
	offset := 5

	// 如果包含 SRID 标志（EWKB 标志位 0x20000000）
	if geomType&0x20000000 != 0 {
		offset += 4
	}

	if len(data) < offset+16 {
		return errors.New("EWKB coordinate payload insufficient")
	}
	// 读取 X (Lng) 和 Y (Lat)，各占 8 字节 (float64)
	p.Lng = math.Float64frombits(byteOrder.Uint64(data[offset : offset+8]))
	p.Lat = math.Float64frombits(byteOrder.Uint64(data[offset+8 : offset+16]))
	return nil
}
