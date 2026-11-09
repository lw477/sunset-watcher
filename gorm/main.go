package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"sunset-watcher/internal/model"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

func main() {
	dsn, err := loadMySQLDSN()
	if err != nil {
		log.Fatalf("读取数据库配置失败: %v", err)
	}

	db, err := InitMySQL(dsn)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}

	log.Println("所有表结构创建与迁移完成，数据库连接就绪！")

	// 导入mock数据
	if err := mockData(db); err != nil {
		log.Fatalf("测试数据导入失败：%v", err)
	}
	log.Println("测试数据导入成功！")
}

func loadMySQLDSN() (string, error) {
	config := viper.New()
	config.SetConfigFile("password/password.yaml")
	if err := config.ReadInConfig(); err != nil {
		return "", fmt.Errorf("read password/password.yaml: %w", err)
	}

	user := strings.TrimSpace(config.GetString("mysql.user"))
	password := config.GetString("mysql.password")
	if user == "" {
		return "", fmt.Errorf("mysql.user is missing in password/password.yaml")
	}
	if password == "" {
		return "", fmt.Errorf("mysql.password is missing in password/password.yaml")
	}

	dsnConfig := mysqldriver.NewConfig()
	dsnConfig.User = user
	dsnConfig.Passwd = password
	dsnConfig.Net = "tcp"
	dsnConfig.Addr = "127.0.0.1:3306"
	dsnConfig.DBName = "gis"
	dsnConfig.ParseTime = true
	dsnConfig.Loc = time.Local
	dsnConfig.Params = map[string]string{"charset": "utf8mb4"}

	return dsnConfig.FormatDSN(), nil
}

// InitMySQL 初始化 MySQL 连接池并执行迁移
func InitMySQL(dsb string) (*gorm.DB, error) {
	db, err := gorm.Open(gormmysql.Open(dsb), &gorm.Config{Logger: logger.Default.LogMode(logger.Info)})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 执行自动建表
	if err := AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}
	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&model.GeoWatchPoint{},
		&model.WeatherForecastSnapshot{},
		&model.TwilightPrediction{},
		&model.LightPathSample{},
		&model.UserSubscription{},
	)
	if err != nil {
		return err
	}
	return nil
}

func setupSpatialPatch(db *gorm.DB) error {
	columnDefinition := `POINT GENERATED ALWAYS AS (
        ST_PointFromText(
            CONCAT('POINT(', longitude, ' ', latitude, ')'),
            4326,
            'axis-order=long-lat'
        )
    ) STORED NOT NULL SRID 4326`

	var columnReady int64
	if err := db.Raw(`
		SELECT COUNT(1)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'geo_watch_points'
		  AND COLUMN_NAME = 'location'
		  AND DATA_TYPE = 'point'
		  AND IS_NULLABLE = 'NO'
		  AND SRS_ID = 4326
		  AND GENERATION_EXPRESSION <> ''
		  AND LOWER(GENERATION_EXPRESSION) LIKE '%axis-order=long-lat%'
	`).Scan(&columnReady).Error; err != nil {
		return fmt.Errorf("check spatial column: %w", err)
	}
	var columnExists int64
	if err := db.Raw(`
		SELECT COUNT(1)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'geo_watch_points'
		  AND COLUMN_NAME = 'location'
	`).Scan(&columnExists).Error; err != nil {
		return fmt.Errorf("check whether spatial column exists: %w", err)
	}

	var indexCount int64
	checkIndex := func() error {
		return db.Raw(`
        SELECT COUNT(1)
        FROM information_schema.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'geo_watch_points'
          AND INDEX_NAME = 'idx_spatial_location'
		`).Scan(&indexCount).Error
	}
	if err := checkIndex(); err != nil {
		return fmt.Errorf("check spatial index: %w", err)
	}

	if columnReady == 0 {
		indexExisted := indexCount > 0
		if indexExisted {
			if err := db.Exec(`ALTER TABLE geo_watch_points DROP INDEX idx_spatial_location`).Error; err != nil {
				return fmt.Errorf("drop spatial index before changing SRID: %w", err)
			}
		}

		alterSQL := "ALTER TABLE geo_watch_points ADD COLUMN location " + columnDefinition
		if columnExists > 0 {
			alterSQL = "ALTER TABLE geo_watch_points MODIFY COLUMN location " + columnDefinition
		}
		if err := db.Exec(alterSQL).Error; err != nil {
			if indexExisted {
				restoreErr := db.Exec(`ALTER TABLE geo_watch_points ADD SPATIAL INDEX idx_spatial_location (location)`).Error
				if restoreErr != nil {
					return fmt.Errorf("apply spatial column patch: %w; restoring spatial index also failed: %v", err, restoreErr)
				}
			}
			return fmt.Errorf("apply spatial column patch: %w", err)
		}
	}

	if err := checkIndex(); err != nil {
		return fmt.Errorf("check spatial index after column patch: %w", err)
	}
	if indexCount == 0 {
		if err := db.Exec(`
            ALTER TABLE geo_watch_points
            ADD SPATIAL INDEX idx_spatial_location (location)
	        `).Error; err != nil {
			return fmt.Errorf("create spatial index: %w", err)
		}
	}

	return nil
}

func mockData(db *gorm.DB) error {
	watchPointData, err := os.ReadFile("./configs/watch_points.json")
	if err != nil {
		fmt.Printf("读取文件失败：%v\n", err)
		return err
	}

	var watchPoint []model.GeoWatchPoint // 注意这里是数组
	err = json.Unmarshal(watchPointData, &watchPoint)
	if err != nil {
		fmt.Printf("解析 JSON 失败: %v\n", err)
		return err
	}

	err = setupSpatialPatch(db)
	if err != nil {
		return fmt.Errorf("setup spatial schema: %w", err)
	}

	result := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"latitude",
			"longitude",
			"elevation_m",
			"timezone",
			"is_active",
		}),
	}).Create(&watchPoint)

	if result.Error != nil {
		return fmt.Errorf("upsert watch points: %w", result.Error)
	}
	return nil
}
