package model

import (
	"sunset-watcher/internal/utils"
	"time"
)

// ==========================================
// 枚举类型定义
// ==========================================

// TwilightEventType 霞光事件类型
type TwilightEventType string

const (
	EventTypeSunrise TwilightEventType = "SUNRISE" // 朝霞
	EventTypeSunset  TwilightEventType = "SUNSET"  // 晚霞
)

// TwilightBurnLevel 霞光评级
type TwilightBurnLevel string

const (
	BurnLevelEpic     TwilightBurnLevel = "EPIC_BURN" // 极品火烧云
	BurnLevelGreat    TwilightBurnLevel = "GREAT"     // 壮美晚霞
	BurnLevelModerate TwilightBurnLevel = "MODERATE"  // 平淡/微霞
	BurnLevelPoor     TwilightBurnLevel = "POOR"      // 阴沉/无霞
)

// CloudCanvasType 主反射云层类型
type CloudCanvasType string

const (
	CanvasNone       CloudCanvasType = "NONE"            // 无有效反射
	CanvasHighCirrus CloudCanvasType = "HIGH_CIRRUS"     // 高卷云为主
	CanvasMidAlto    CloudCanvasType = "MID_ALTOCUMULUS" // 中高积云为主
	CanvasMixed      CloudCanvasType = "MIXED_HIGH_MID"  // 中高云混合
)

// ==========================================
// 1. 监测网格与点位表
// ==========================================

type GeoWatchPoint struct {
	ID        int64   `gorm:"column:id;primaryKey;autoIncrement;comment:观测点主键ID" json:"id"`
	Name      string  `gorm:"column:name;type:varchar(128);not null;comment:观测点或网格名称;uniqueIndex:uq_geo_watch_points_name" json:"name"`
	Latitude  float64 `gorm:"column:latitude;type:decimal(9,6);not null;check:chk_latitude,latitude BETWEEN -90.0 AND 90.0;comment:纬度(-90到90)" json:"latitude"`
	Longitude float64 `gorm:"column:longitude;type:decimal(9,6);not null;check:chk_longitude,longitude BETWEEN -180.0 AND 180.0;comment:经度(-180到180)" json:"longitude"`

	// 只读映射：MySQL 生成列，写入时自动排除，读取时解析为 types.Point 结构体
	Location utils.Point `gorm:"column:location;type:point;not null;->;-:migration;comment:WGS84空间几何点" json:"location"`

	ElevationM int       `gorm:"column:elevation_m;type:int;not null;default:0;comment:海拔高度(米)" json:"elevation_m"`
	Timezone   string    `gorm:"column:timezone;type:varchar(64);not null;default:'UTC';comment:IANA时区" json:"timezone"`
	IsActive   bool      `gorm:"column:is_active;type:tinyint(1);not null;default:1;index:idx_is_active;comment:是否纳入定时推算" json:"is_active"`
	CreatedAt  time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP;comment:创建时间" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP on update CURRENT_TIMESTAMP;comment:更新时间" json:"updated_at"`

	// 关联定义
	Snapshots     []WeatherForecastSnapshot `gorm:"foreignKey:PointID;constraint:OnDelete:CASCADE" json:"snapshots,omitempty"`
	Predictions   []TwilightPrediction      `gorm:"foreignKey:PointID;constraint:OnDelete:CASCADE" json:"predictions,omitempty"`
	Subscriptions []UserSubscription        `gorm:"foreignKey:PointID;constraint:OnDelete:CASCADE" json:"subscriptions,omitempty"`
}

func (GeoWatchPoint) TableName() string {
	return "geo_watch_points"
}

// ==========================================
// 2. 气象预报快照表
// ==========================================

type WeatherForecastSnapshot struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement;comment:快照ID" json:"id"`
	PointID      int64     `gorm:"column:point_id;type:bigint;not null;uniqueIndex:uq_point_forecast_time,priority:1;comment:关联监测点ID" json:"point_id"`
	ForecastTime time.Time `gorm:"column:forecast_time;type:datetime;not null;uniqueIndex:uq_point_forecast_time,priority:2;index:idx_forecast_time;comment:气象预报时刻(UTC)" json:"forecast_time"`
	FetchTime    time.Time `gorm:"column:fetch_time;type:datetime;not null;default:CURRENT_TIMESTAMP;comment:抓取入库时间" json:"fetch_time"`

	// 垂直云量分布（0-100%）
	CloudLow  int16 `gorm:"column:cloud_low;type:smallint;not null;check:chk_cloud_low,cloud_low BETWEEN 0 AND 100;comment:低云量百分比" json:"cloud_low"`
	CloudMid  int16 `gorm:"column:cloud_mid;type:smallint;not null;check:chk_cloud_mid,cloud_mid BETWEEN 0 AND 100;comment:中云量百分比" json:"cloud_mid"`
	CloudHigh int16 `gorm:"column:cloud_high;type:smallint;not null;check:chk_cloud_high,cloud_high BETWEEN 0 AND 100;comment:高云量百分比" json:"cloud_high"`

	// 大气与地表参数
	VisibilityM int      `gorm:"column:visibility_m;type:int;not null;comment:地表能见度(米)" json:"visibility_m"`
	HumidityPct int16    `gorm:"column:humidity_pct;type:smallint;not null;check:chk_humidity,humidity_pct BETWEEN 0 AND 100;comment:地表相对湿度" json:"humidity_pct"`
	AodIndex    *float64 `gorm:"column:aod_index;type:decimal(4,2);comment:气溶胶光学厚度" json:"aod_index,omitempty"`

	// 关联所属点位
	Point *GeoWatchPoint `gorm:"foreignKey:PointID" json:"point,omitempty"`
}

func (WeatherForecastSnapshot) TableName() string {
	return "weather_forecast_snapshots"
}

// ==========================================
// 3. 霞光预测结果主表
// ==========================================

type TwilightPrediction struct {
	ID         int64             `gorm:"column:id;primaryKey;autoIncrement;comment:预测结果ID" json:"id"`
	PointID    int64             `gorm:"column:point_id;type:bigint;not null;uniqueIndex:uq_point_event_date,priority:1;comment:关联监测点ID" json:"point_id"`
	EventType  TwilightEventType `gorm:"column:event_type;type:enum('SUNRISE','SUNSET');not null;uniqueIndex:uq_point_event_date,priority:2;comment:事件类型" json:"event_type"`
	TargetDate time.Time         `gorm:"column:target_date;type:date;not null;uniqueIndex:uq_point_event_date,priority:3;index:idx_target_date_score,priority:1;comment:预测目标日期" json:"target_date"`

	// 天文几何时刻
	PeakTime       time.Time `gorm:"column:peak_time;type:datetime;not null;index:idx_peak_time;comment:最佳观赏时刻(UTC)" json:"peak_time"`
	SunAzimuth     float64   `gorm:"column:sun_azimuth;type:decimal(5,2);not null;comment:落日/日出方位角(0-360)" json:"sun_azimuth"`
	SolarElevation float64   `gorm:"column:solar_elevation;type:decimal(4,2);not null;comment:峰值时太阳高度角" json:"solar_elevation"`

	// 综合评分与级别
	OverallScore int16             `gorm:"column:overall_score;type:smallint;not null;index:idx_target_date_score,priority:2,sort:desc;check:chk_overall_score,overall_score BETWEEN 0 AND 100;comment:综合评分(0-100)" json:"overall_score"`
	BurnLevel    TwilightBurnLevel `gorm:"column:burn_level;type:enum('EPIC_BURN','GREAT','MODERATE','POOR');not null;comment:霞光评级" json:"burn_level"`

	// 模型打分因子细分
	CanvasFactor     float64         `gorm:"column:canvas_factor;type:decimal(5,2);not null;comment:云层幕布反射分(0-100)" json:"canvas_factor"`
	PathTransparency float64         `gorm:"column:path_transparency;type:decimal(4,3);not null;comment:光路通透率(0-1)" json:"path_transparency"`
	LowCloudPenalty  float64         `gorm:"column:low_cloud_penalty;type:decimal(5,2);not null;comment:本地低云遮挡扣分(0-80)" json:"low_cloud_penalty"`
	AtmosphereFactor float64         `gorm:"column:atmosphere_factor;type:decimal(3,2);not null;comment:大气透明纯度修正因子" json:"atmosphere_factor"`
	PathBlocked      bool            `gorm:"column:path_blocked;type:tinyint(1);not null;default:0;comment:光路是否彻底阻断" json:"path_blocked"`
	CloudCanvasType  CloudCanvasType `gorm:"column:cloud_canvas_type;type:enum('NONE','HIGH_CIRRUS','MID_ALTOCUMULUS','MIXED_HIGH_MID');not null;default:'NONE';comment:主反射云层类别" json:"cloud_canvas_type"`

	SummaryText  *string   `gorm:"column:summary_text;type:varchar(255);comment:简短综述文本" json:"summary_text,omitempty"`
	CalculatedAt time.Time `gorm:"column:calculated_at;type:datetime;not null;default:CURRENT_TIMESTAMP;comment:模型推算时间" json:"calculated_at"`

	// 关联
	Point   *GeoWatchPoint    `gorm:"foreignKey:PointID" json:"point,omitempty"`
	Samples []LightPathSample `gorm:"foreignKey:PredictionID;constraint:OnDelete:CASCADE" json:"samples,omitempty"`
}

func (TwilightPrediction) TableName() string {
	return "twilight_predictions"
}

// ==========================================
// 4. 光路步进采样明细表
// ==========================================

type LightPathSample struct {
	ID           int64   `gorm:"column:id;primaryKey;autoIncrement;comment:采样明细ID" json:"id"`
	PredictionID int64   `gorm:"column:prediction_id;type:bigint;not null;index:idx_pred_step,priority:1;comment:关联预测记录ID" json:"prediction_id"`
	StepOrder    int16   `gorm:"column:step_order;type:smallint;not null;index:idx_pred_step,priority:2;comment:步进序号(1/2/3...)" json:"step_order"`
	DistanceKM   int     `gorm:"column:distance_km;type:int;not null;comment:沿光路距观测点距离(km)" json:"distance_km"`
	Latitude     float64 `gorm:"column:latitude;type:decimal(9,6);not null;comment:采样点纬度" json:"latitude"`
	Longitude    float64 `gorm:"column:longitude;type:decimal(9,6);not null;comment:采样点经度" json:"longitude"`
	CloudLow     int16   `gorm:"column:cloud_low;type:smallint;not null;check:chk_step_cloud,cloud_low BETWEEN 0 AND 100;comment:采样点实测低云量" json:"cloud_low"`
	IsBlocking   bool    `gorm:"column:is_blocking;type:tinyint(1);not null;default:0;comment:该点是否产生低空遮光截断" json:"is_blocking"`

	Prediction *TwilightPrediction `gorm:"foreignKey:PredictionID" json:"prediction,omitempty"`
}

func (LightPathSample) TableName() string {
	return "light_path_samples"
}

// ==========================================
// 5. 用户关注与预警订阅表
// ==========================================

type UserSubscription struct {
	ID                int64             `gorm:"column:id;primaryKey;autoIncrement;comment:订阅ID" json:"id"`
	UserID            string            `gorm:"column:user_id;type:varchar(64);not null;comment:用户标识/设备Token" json:"user_id"`
	PointID           int64             `gorm:"column:point_id;type:bigint;not null;index:idx_point_notify,priority:1;comment:关联监测点ID" json:"point_id"`
	EventType         TwilightEventType `gorm:"column:event_type;type:enum('SUNRISE','SUNSET');not null;default:'SUNSET';comment:关注事件" json:"event_type"`
	MinScoreThreshold int16             `gorm:"column:min_score_threshold;type:smallint;not null;default:75;check:chk_threshold,min_score_threshold BETWEEN 0 AND 100;comment:触发提醒最低分阈值" json:"min_score_threshold"`
	NotifyChannel     string            `gorm:"column:notify_channel;type:varchar(32);not null;default:'WECHAT_BOT';comment:通知渠道(WECHAT_BOT/FEISHU/APNS)" json:"notify_channel"`
	WebhookURL        *string           `gorm:"column:webhook_url;type:text;comment:Webhook推送地址" json:"webhook_url,omitempty"`
	IsEnabled         bool              `gorm:"column:is_enabled;type:tinyint(1);not null;default:1;index:idx_point_notify,priority:2;comment:是否启用提醒" json:"is_enabled"`
	CreatedAt         time.Time         `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP;comment:创建时间" json:"created_at"`

	Point *GeoWatchPoint `gorm:"foreignKey:PointID" json:"point,omitempty"`
}

func (UserSubscription) TableName() string {
	return "user_subscriptions"
}
