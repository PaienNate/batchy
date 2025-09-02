package test

import (
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 复杂指标结构体
type ComplexMetrics struct {
	gorm.Model

	ID          int `gorm:"primaryKey;autoIncrement"`
	Timestamp   time.Time
	DeviceID    string
	Temperature float64 // 温度指标
	Humidity    float64 // 湿度指标
	Pressure    float64 // 气压指标
	Status      Status  `gorm:"type:smallint"` // 状态枚举
	Metadata    JSONB   `gorm:"type:jsonb"`    // 元数据JSON
}

type Status int

const (
	StatusNormal Status = iota
	StatusWarning
	StatusError
)

type JSONB map[string]interface{}

// 初始化数据库连接
func initDB() *gorm.DB {
	dsn := "host=localhost user=postgres password=123456 dbname=test port=5432 sslmode=disable TimeZone=Asia/Shanghai"

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	// 获取底层sql.DB对象以配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get underlying sql.DB: %v", err)
	}

	// 配置连接池
	sqlDB.SetMaxIdleConns(50)                  // 空闲连接池中的最大连接数
	sqlDB.SetMaxOpenConns(200)                 // 数据库打开的最大连接数
	sqlDB.SetConnMaxLifetime(time.Hour)        // 连接可重用的最长时间
	sqlDB.SetConnMaxIdleTime(30 * time.Minute) // 连接在连接池中的最大空闲时间

	// 自动迁移表结构
	err = db.AutoMigrate(&ComplexMetrics{})
	if err != nil {
		log.Fatalf("Failed to auto migrate: %v", err)
	}

	return db
}
