# Sunset-Watcher: 全球日出日落（晚霞/朝霞）智能监测与预测系统

> **项目定位**：基于 Clean Architecture 架构与物理大气模型的服务端预测系统。通过对垂直分层云量（低/中/高云）、太阳天文几何（高度角/方位角）、光路反向射线步进（Ray Marching）及大气能见度进行量化评估，预测特定地理点位发生高质量晚霞/火烧云（Twilight Burn）的概率与观赏等级。
> **核心技术栈**：Go (Golang) + MySQL 8.0 (Spatial Point, SRID 4326, Generated Columns) + GORM + Redis。

---

## 1. 系统总体架构与分层

系统遵循 **Clean Architecture（整洁架构）** 原则，严格保持核心物理计算逻辑与底层存储/外部接口的解耦。

```
┌────────────────────────────────────────────────────────────────────────┐
│                        4. 入口适配层 (Delivery)                         │
│         HTTP REST API (Gin)     /     分布式定时任务 (Cron/Worker)       │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
┌───────────────────────────────────▼────────────────────────────────────┐
│                         3. 业务编排层 (Usecase)                         │
│  点位评估管线 / 批量并发扫描引擎 / 缓存优先策略 / 预警规则触发与分发   │
└───────────────────────┬────────────────────────┬───────────────────────┘
                        │                        │
┌───────────────────────▼──────────────┐ ┌───────▼───────────────────────┐
│       2. 核心领域与算法层 (Domain)     │ │        数据仓储契约 (Repository)      │
│  天体几何计算 (Solar) / 大圆步进采样  │ │  定义 WatchPointRepo / ForecastRepo   │
│  大气衰减与云层折射 / 霞光综合打分模型│ │  PredictionRepo / SubscriptionRepo   │
└───────────────────────┬──────────────┘ └───────┬───────────────────────┘
                        │                        │
┌───────────────────────▼────────────────────────▼───────────────────────┐
│                       1. 基础设施与外部驱动 (Infra)                     │
│  GORM 持久化实现 (MySQL 8.0 + 空间生成列)  /  空间缓存 (Redis GEO)       │
│  外部气象源客户端 (Open-Meteo API)          /  通知管道 (Webhook / Bot)  │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 工程目录结构规范

```text
sunset-watcher/
├── cmd/
│   ├── server/                     # HTTP API 主服务入口
│   │   └── main.go
│   ├── worker/                     # 后台离线批处理引擎（定时抓取全国数据、打分入库）
│   │   └── main.go
│   └── migrate/                    # 数据库迁移与初始化点位种子数据脚本
│       └── main.go
├── configs/
│   ├── config.yaml                 # 数据库连接、Redis、气象接口配置
│   └── watch_points.json           # 默认预设重点观测机位列表（全国/全球）
├── internal/
│   ├── domain/                     # 核心领域实体与业务契约（纯 Go 业务，零外部依赖）
│   │   ├── astro.go                # 太阳方位角、高度角、日落黄金时刻计算器
│   │   ├── geometry.go             # 大圆航线距离推算、反向方位角步进点计算
│   │   ├── scorer.go               # 霞光物理打分模型（幕布系数、光路截断、湿度惩罚）
│   │   ├── weather.go              # 垂直云量、能见度、气溶胶等领域结构体
│   │   ├── prediction.go           # 预测结果与评级枚举实体
│   │   └── repository.go           # 核心仓储接口定义（WatchPointRepo、PredictionRepo 等）
│   ├── usecase/                    # 业务用例与编排
│   │   ├── evaluate.go             # 单点计算流（查缓存 -> 算天文 -> 查气象 -> 打分 -> 落库）
│   │   ├── batch_scan.go           # WorkerPool 并发批量处理引擎
│   │   └── alert_dispatcher.go     # 达标预测的阈值筛选与告警推送编排
│   ├── infra/                      # 基础设施驱动与持久化实现
│   │   ├── persistence/            # GORM 驱动与数据库操作
│   │   │   ├── mysql.go            # GORM 连接池初始化、Ping 检测
│   │   │   ├── migration.go        # 空间生成列补丁与 SPATIAL INDEX 修复逻辑
│   │   │   ├── model/              # GORM 数据库映射 Model（对应 5 张业务表）
│   │   │   │   ├── watch_point.go  # geo_watch_points 结构
│   │   │   │   ├── forecast.go     # weather_forecast_snapshots 结构
│   │   │   │   ├── prediction.go   # twilight_predictions 结构
│   │   │   │   ├── sample.go       # light_path_samples 结构
│   │   │   │   └── subscription.go # user_subscriptions 结构
│   │   │   └── repository_impl/    # GORM 对 domain.Repository 接口的具体实现
│   │   │       ├── watch_point_repo.go
│   │   │       └── prediction_repo.go
│   │   ├── weather/                # 气象源具体驱动
│   │   │   ├── client.go           # HTTP 客户端配置（超时、重试、并发限流）
│   │   │   └── open_meteo.go       # 对接 Open-Meteo 垂直层 API
│   │   ├── cache/                  # Redis 缓存层
│   │   │   ├── redis.go
│   │   │   └── prediction_cache.go # 预测结果热缓存、空间半径检索加速
│   │   └── notify/                 # 多渠道告警推送客户端
│   │       ├── webhook.go          # 企业微信 / 飞书 Bot 推送
│   │       └── apns.go             # 移动端推送（可选）
│   └── delivery/                   # 交付协议入口
│       ├── http/                   # HTTP 接口路由与控制器
│       │   ├── router.go
│       │   ├── handler_prediction.go  # 预测得分看板、光路切片明细接口
│       │   └── handler_watch_point.go # 点位周边空间检索接口（ST_Distance_Sphere）
│       └── cron/                   # 定时任务调度器
│           └── scheduler.go        # 每日早晚自动化定时任务配置
├── pkg/
│   ├── types/
│   │   └── point.go                # 自定义 MySQL 空间 Point 类型（实现 Scanner/Valuer）
│   └── logger/                     # 结构化日志工具
├── go.mod
├── go.sum
└── docker-compose.yml              # 本地 MySQL 8.0、Redis 容器快速启动编排
```

---

## 3. 核心数学与物理模型

系统的预测质量依赖于三个物理事实的量化：

1. **幕布是否就绪**：观测点头顶与前方必须有足够的中云或高云作为漫反射媒介（云画布）。
2. **光路是否通畅**：沿落日方向 100~300km 处，不能有低厚积雨云/层云遮挡日暮低角度切向入射光。
3. **本地视线是否清澈**：观测点自身不能被低云完全覆盖，且底层大气能见度需维持纯净。

### 3.1 太阳天文几何推算 (Solar Geometry)

* **太阳赤纬角 ($\delta$) 与时角 ($H$)**：根据目标日期 $DayOfYear$ 与地理经纬度推导精确太阳中心位置。
* **太阳天顶角 ($\theta_z$) 与高度角 ($h$)**：
  $$\sin h = \cos\theta_z = \sin\phi\sin\delta + \cos\phi\cos\delta\cos H$$
* **核心观测窗口**：
  * 锁定太阳高度角处于 $[-6^\circ, +2^\circ]$ 区间（民用曙暮光至日落前夕）。
  * 晚霞最艳丽的峰值时刻通常发生于太阳位于地平线以下约 $-2^\circ \sim -4^\circ$（日落后约 15~20 分钟）。

### 3.2 反向光路步进 (Reverse Ray Marching)

* **光线反向追踪**：以观测点 $(\text{lat}_0, \text{lon}_0)$ 为起点，沿着太阳方位角 $\alpha$（晚霞沿方位角正向，朝霞沿反向），使用地球大圆球面公式向外推算采样点：
  * 步长：$40 \sim 60\text{ km}$
  * 步数：$3 \sim 5$ 步（外推 $120 \sim 300\text{ km}$）
* **光路阻断判定**：若沿途连续出现高密度的低云（例如连续步进点低云量 $> 80\%$），则判定日落入光口完全“堵死”，发生光路彻底截断（`path_blocked = true`）。

### 3.3 综合评分模型公式

综合得分 $S \in [0, 100]$ 由四大因子共同决定：

$$S = \left( C_{\text{canvas}} \times T_{\text{path}} \times A_{\text{purity}} - P_{\text{low\_cloud}} \right)$$

1. **画布因子 ($C_{\text{canvas}} \in [0, 100]$)**：高云（卷云）权重最高，中云（高积云）次之。覆盖率在 $40\% \sim 70\%$ 时达到峰值，全阴（$100\%$）或全晴（$0\%$）得分衰减。
2. **光路通透率 ($T_{\text{path}} \in [0.1, 1.0]$)**：沿步进采样点低云量的指数衰减连乘积。
3. **本地低云惩罚 ($P_{\text{low\_cloud}} \in [0, 80]$)**：观测点本地低云若 $> 70\%$，视线受阻严重，扣分急剧放大。
4. **大气纯净修正 ($A_{\text{purity}} \in [0.7, 1.1]$)**：根据能见度与相对湿度微调，剔除重霾与水汽混浊影响。

---

## 4. 全套工程落地实施步骤

### 第一阶段：存储基石与自定义空间类型落地
1. **容器环境启动**：
   - 配置 `docker-compose.yml`，启动 MySQL 8.0（开启 `utf8mb4`）与 Redis 服务。
2. **底层自定义空间数据类型实现 (`pkg/types/point.go`)**：
   - 实现 `types.Point` 结构体，包含 `Lng` 与 `Lat` 两个浮点字段。
   - 实现 `database/sql/driver.Valuer` 接口，输出 `POINT(lng lat)` 格式 WKT 文本。
   - 实现 `sql.Scanner` 接口，解析 MySQL 8.0 特有的前置 4 字节 SRID 头部及 21 字节小端序标准 WKB 数据。
3. **数据库初始化与补丁脚本 (`cmd/migrate/main.go`)**：
   - 编写 GORM 模型，为生成列 `Location` 声明 `->` 只读标记。
   - 封装迁移流程：先调用 `db.AutoMigrate` 生成基础表结构，再执行空间补丁 SQL，将 `location` 修改为带有 `SRID 4326` 的 `STORED` 生成列，并补齐 `SPATIAL INDEX idx_spatial_location`。
4. **初始点位播种 (Seed Data)**：
   - 读取预设点位配置文件（如广州塔、上海陆家嘴、北京天安门等），执行点位写入。
   - 验证通过经纬度写入后，MySQL 是否自动在 `location` 列生成有效的空间数据。

---

### 第二阶段：纯领域天体与光路算法实现 (Zero-Dependency Domain)
1. **天体几何引擎 (`internal/domain/astro.go`)**：
   - 编写纯数学函数：给定目标日期与点位经纬度，推导儒略日（Julian Day）与太阳位置。
   - 计算民用曙光、日出、日暮、民用暮光四个阶段的准确 UTC 时间戳。
   - 计算目标观赏时刻（日落后 15 分钟）的太阳高度角与方位角（$0^\circ \sim 360^\circ$）。
2. **大圆航线步进推算器 (`internal/domain/geometry.go`)**：
   - 编写 Vincenty 或 Haversine 大圆正算函数：输入起点坐标 $(\text{lat}_1, \text{lon}_1)$、移动方位角 $\theta$ 和距离 $d$（单位：公里），计算终点坐标 $(\text{lat}_2, \text{lon}_2)$。
   - 封装步进采样逻辑：晚霞沿太阳方位角正向向外推算 3~5 个步进经纬度（如 50km, 100km, 180km, 260km），朝霞则沿反向（$\theta \pm 180^\circ$）外推。
3. **霞光打分模型 (`internal/domain/scorer.go`)**：
   - 编写纯计算用例，输入一组垂直分层气象指标（本地低中高云量、能见度、相对湿度）以及光路各步进点的低云量。
   - 实现画布评分、光路衰减连乘、本地低云惩罚和大气纯度系数计算，输出 0-100 综合得分与评级。
4. **领域单元测试覆盖**：
   - 针对天文函数比对已知日落时刻，验证偏差在 $\pm 1$ 分钟以内。
   - 针对打分模型设计 4 组用例（晴空万里、全阴暴雨、中高云火烧云、远处低云截断），验证得分输出是否符合物理直觉。

---

### 第三阶段：外部气象数据接入与基础设施仓储
1. **气象 API 客户端封装 (`internal/infra/weather/open_meteo.go`)**：
   - 对接 Open-Meteo 免费气象接口，配置带有连接池与重试机制的 HTTP Client。
   - 封装按经纬度拉取逐小时垂直云层（`cloud_cover_low`, `cloud_cover_mid`, `cloud_cover_high`）、能见度、相对湿度的请求逻辑。
2. **GORM 仓储实现 (`internal/infra/persistence/repository_impl/`)**：
   - 实现 `WatchPointRepo`：根据状态拉取待计算点位，通过 `ST_Distance_Sphere` 实现周边半径检索。
   - 实现 `ForecastRepo`：批量保存气象快照，利用联合唯一索引 `(point_id, forecast_time)` 实现存在即更新（Upsert）。
   - 实现 `PredictionRepo`：存储最终得分与光路采样切片记录。
3. **空间缓存与热数据加速 (`internal/infra/cache/`)**：
   - 封装 Redis 客户端，将当天的最新预测卡片以 JSON 形式存入 Redis，设置 1~2 小时过期时间。

---

### 第四阶段：业务编排层（Usecase）开发
1. **单点预测执行编排 (`internal/usecase/evaluate.go`)**：
   - 串联单个点位的预测完整链路：
     1. 从仓储获取监测点信息。
     2. 运行天文引擎计算当天日出与日落黄金观测时刻。
     3. 沿落日方位角计算光路步进点坐标列表。
     4. 并发拉取观测点自身及光路采样点的气象快照。
     5. 调用打分引擎计算综合得分与因子明细。
     6. 持久化至 MySQL 并写入 Redis 缓存。
2. **批量扫描与 Worker 线程池 (`internal/usecase/batch_scan.go`)**：
   - 针对全国上百个活跃监测网格点，设计带缓冲的 Worker Pool（设置并发度如 5~10）。
   - 控制并发抓取频率，防止触发外部气象源的 API 限流，支持批量任务失败重试与超时控制。
3. **预警筛选与派发编排 (`internal/usecase/alert_dispatcher.go`)**：
   - 扫描用户订阅配置 `user_subscriptions`。
   - 筛选出预测分数超过用户预设阈值（如 $\ge 75$ 分）的记录。
   - 组装格式化告警内容，调用通知模块异步下发。

---

### 第五阶段：服务交付层与自动化调度 (Delivery Layer)
1. **RESTful HTTP API 接口暴露 (`internal/delivery/http/`)**：
   - 接入 Gin 引擎并配置中间件（跨域 CORS、链路日志、Recovery）。
   - 实现 `GET /api/v1/points/nearby`：入参经纬度与半径（米），调用 MySQL 空间函数快速返回周边监测点。
   - 实现 `GET /api/v1/predictions/latest`：根据点位 ID 获取最新朝霞/晚霞预测结果卡片、评级标签及诊断简评。
   - 实现 `GET /api/v1/predictions/:id/samples`：返回特定预测的光路步进采样明细，用于前端在地图上渲染光路射线。
2. **离线定时调度任务 (`internal/delivery/cron/scheduler.go`)**：
   - 集成 `robfig/cron` 调度器：
     - **凌晨 03:00 (UTC)**：触发全量点位当日朝霞（Sunrise）的批处理评估。
     - **上午 10:00 (UTC)**：触发全量点位当日晚霞（Sunset）的批处理评估。
     - **计算完成后 10 分钟**：自动触发预警通知分发流。

---

### 第六阶段：集成联调、压测与物理模型微调
1. **端到端链路集成验证**：
   - 启动 `server` 与 `worker` 服务，手动触发一次批量扫描任务，观察 MySQL 中 5 张表的数据流转与外键级联。
2. **性能与空间检索测试**：
   - 在 `geo_watch_points` 表中插入上千个测试网格点，运行周边距离检索 SQL，通过 `EXPLAIN` 验证 `idx_spatial_location` 空间索引是否有效命中。
3. **评分模型校准**：
   - 对照近期的实际晚霞照片与历史气象数据（例如典型火烧云日与严重阴雨日），反向调优高云/中云反射权重比及光路衰减步进距离，完成生产交付。