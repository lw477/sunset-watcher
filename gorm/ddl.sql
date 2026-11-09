-- 创建数据库（如果尚未创建）
CREATE DATABASE IF NOT EXISTS `twilight_forecast` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE `twilight_forecast`;

-- ---------------------------------------------------------------------
-- 1. 监测网格与点位表 (geo_watch_points)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `geo_watch_points` (
                                                  `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
                                                  `name` VARCHAR(128) NOT NULL COMMENT '观测点/网格名称',
    `latitude` DECIMAL(9, 6) NOT NULL COMMENT '纬度 (-90 到 90)',
    `longitude` DECIMAL(9, 6) NOT NULL COMMENT '经度 (-180 到 180)',

    -- 空间几何生成列：强制非空且绑定 WGS84 (SRID 4326) 空间参考系
    `location` POINT GENERATED ALWAYS AS (
                                                                ST_PointFromText(CONCAT('POINT(', `longitude`, ' ', `latitude`, ')'), 4326, 'axis-order=long-lat')
    ) STORED NOT NULL SRID 4326 COMMENT '空间地理坐标点',

    `elevation_m` INT NOT NULL DEFAULT 0 COMMENT '海拔高度(米)',
    `timezone` VARCHAR(64) NOT NULL DEFAULT 'UTC' COMMENT 'IANA 时区',
    `is_active` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否纳入定时计算',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    -- 空间索引与数值边界检查
    SPATIAL INDEX `idx_spatial_location` (`location`),
    INDEX `idx_is_active` (`is_active`),
    CONSTRAINT `chk_latitude` CHECK (`latitude` BETWEEN -90.0 AND 90.0),
    CONSTRAINT `chk_longitude` CHECK (`longitude` BETWEEN -180.0 AND 180.0)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='监测点位基础信息表';

-- ---------------------------------------------------------------------
-- 2. 气象预报快照表 (weather_forecast_snapshots)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `weather_forecast_snapshots` (
                                                            `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
                                                            `point_id` BIGINT NOT NULL COMMENT '关联观测点ID',
                                                            `forecast_time` DATETIME NOT NULL COMMENT '预报目标时刻(UTC)',
                                                            `fetch_time` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '数据抓取入库时间',

    -- 云量参数 (0-100%)
                                                            `cloud_low` SMALLINT NOT NULL COMMENT '低云覆盖率 (<2000m)',
                                                            `cloud_mid` SMALLINT NOT NULL COMMENT '中云覆盖率 (2000-6000m)',
                                                            `cloud_high` SMALLINT NOT NULL COMMENT '高云覆盖率 (>6000m)',

    -- 地表与大气参数
                                                            `visibility_m` INT NOT NULL COMMENT '地表能见度(米)',
                                                            `humidity_pct` SMALLINT NOT NULL COMMENT '相对湿度百分比 (0-100)',
                                                            `aod_index` DECIMAL(4, 2) DEFAULT NULL COMMENT '气溶胶光学厚度 (AOD)',

    -- 联合唯一索引：同一点位在同一时刻的预报唯一，便于拉取时覆盖更新
    UNIQUE KEY `uq_point_forecast_time` (`point_id`, `forecast_time`),
    INDEX `idx_forecast_time` (`forecast_time`),
    CONSTRAINT `fk_snapshots_point_id` FOREIGN KEY (`point_id`) REFERENCES `geo_watch_points` (`id`) ON DELETE CASCADE,
    CONSTRAINT `chk_cloud_low` CHECK (`cloud_low` BETWEEN 0 AND 100),
    CONSTRAINT `chk_cloud_mid` CHECK (`cloud_mid` BETWEEN 0 AND 100),
    CONSTRAINT `chk_cloud_high` CHECK (`cloud_high` BETWEEN 0 AND 100),
    CONSTRAINT `chk_humidity` CHECK (`humidity_pct` BETWEEN 0 AND 100)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='垂直分层气象数据快照表';

-- ---------------------------------------------------------------------
-- 3. 霞光预测结果表 (twilight_predictions)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `twilight_predictions` (
                                                      `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
                                                      `point_id` BIGINT NOT NULL COMMENT '关联监测点ID',
                                                      `event_type` ENUM('SUNRISE', 'SUNSET') NOT NULL COMMENT '事件类型：朝霞 / 晚霞',
    `target_date` DATE NOT NULL COMMENT '预测目标日期',

    -- 天文时间与几何参数
    `peak_time` DATETIME NOT NULL COMMENT '预计最佳观赏时刻(UTC)',
    `sun_azimuth` DECIMAL(5, 2) NOT NULL COMMENT '太阳方位角 (0.00 - 360.00)',
    `solar_elevation` DECIMAL(4, 2) NOT NULL COMMENT '此时刻太阳高度角',

    -- 综合评分与评级
    `overall_score` SMALLINT NOT NULL COMMENT '综合得分 (0 - 100)',
    `burn_level` ENUM('EPIC_BURN', 'GREAT', 'MODERATE', 'POOR') NOT NULL COMMENT '评级',

    -- 物理中间打分因子（排查计算结果与前端详情展开）
    `canvas_factor` DECIMAL(5, 2) NOT NULL COMMENT '幕布反射分值 (0 - 100)',
    `path_transparency` DECIMAL(4, 3) NOT NULL COMMENT '光路通透系数 (0.000 - 1.000)',
    `low_cloud_penalty` DECIMAL(5, 2) NOT NULL COMMENT '本地低云遮挡扣分 (0 - 80)',
    `atmosphere_factor` DECIMAL(3, 2) NOT NULL COMMENT '大气透明度系数 (0.50 - 1.10)',
    `path_blocked` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '落日/日出方向光路是否彻底阻断',
    `cloud_canvas_type` ENUM('NONE', 'HIGH_CIRRUS', 'MID_ALTOCUMULUS', 'MIXED_HIGH_MID') NOT NULL DEFAULT 'NONE' COMMENT '主反射云层类型',

    `summary_text` VARCHAR(255) DEFAULT NULL COMMENT '简评（如：中云饱满且视线通透）',
    `calculated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '计算生成时间',

    -- 同一点位每天的朝霞或晚霞仅保留一条综合记录
    UNIQUE KEY `uq_point_event_date` (`point_id`, `event_type`, `target_date`),
    INDEX `idx_target_date_score` (`target_date`, `overall_score` DESC),
    INDEX `idx_peak_time` (`peak_time`),
    CONSTRAINT `fk_predictions_point_id` FOREIGN KEY (`point_id`) REFERENCES `geo_watch_points` (`id`) ON DELETE CASCADE,
    CONSTRAINT `chk_overall_score` CHECK (`overall_score` BETWEEN 0 AND 100)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='霞光预测评估结果主表';

-- ---------------------------------------------------------------------
-- 4. 光路步进采样明细表 (light_path_samples)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `light_path_samples` (
                                                    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
                                                    `prediction_id` BIGINT NOT NULL COMMENT '关联预测记录ID',
                                                    `step_order` SMALLINT NOT NULL COMMENT '步进序号 (1, 2, 3...)',
                                                    `distance_km` INT NOT NULL COMMENT '离起点距离(km)，如 50, 100, 180, 260',
                                                    `latitude` DECIMAL(9, 6) NOT NULL COMMENT '采样点纬度',
    `longitude` DECIMAL(9, 6) NOT NULL COMMENT '采样点经度',
    `cloud_low` SMALLINT NOT NULL COMMENT '采样点实测低云量 (0-100)',
    `is_blocking` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '该采样点是否发生遮光截断',

    INDEX `idx_pred_step` (`prediction_id`, `step_order`),
    CONSTRAINT `fk_samples_prediction_id` FOREIGN KEY (`prediction_id`) REFERENCES `twilight_predictions` (`id`) ON DELETE CASCADE,
    CONSTRAINT `chk_step_cloud` CHECK (`cloud_low` BETWEEN 0 AND 100)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='光路反向步进采样轨迹明细表';

-- ---------------------------------------------------------------------
-- 5. 用户关注与预警订阅表 (user_subscriptions)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `user_subscriptions` (
                                                    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
                                                    `user_id` VARCHAR(64) NOT NULL COMMENT '用户ID或设备Token',
    `point_id` BIGINT NOT NULL COMMENT '关联关注的监测点ID',
    `event_type` ENUM('SUNRISE', 'SUNSET') NOT NULL DEFAULT 'SUNSET' COMMENT '关注事件',
    `min_score_threshold` SMALLINT NOT NULL DEFAULT 75 COMMENT '触发推送的最低分数阈值',
    `notify_channel` VARCHAR(32) NOT NULL DEFAULT 'WECHAT_BOT' COMMENT '通知渠道',
    `webhook_url` TEXT DEFAULT NULL COMMENT '推送地址',
    `is_enabled` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用推送',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX `idx_point_notify` (`point_id`, `is_enabled`),
    CONSTRAINT `fk_sub_point_id` FOREIGN KEY (`point_id`) REFERENCES `geo_watch_points` (`id`) ON DELETE CASCADE,
    CONSTRAINT `chk_threshold` CHECK (`min_score_threshold` BETWEEN 0 AND 100)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='霞光预警通知订阅配置表';
