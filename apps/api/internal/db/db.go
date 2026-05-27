package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	mysqldriver "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/file"

	"github.com/golang-migrate/migrate/v4"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Init() *gorm.DB {
	var err error
	// 应用主连接不开 multiStatements,避免任何未来误用的 Raw 查询被放大成多语句注入。
	DB, err = gorm.Open(mysql.Open(baseDSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	return DB
}

func RunMigrations() {
	// migrate 的 .sql 文件每个含多条语句(001 就有 17 张表),必须 multiStatements=true 才能在单次
	// Exec 里执行;为此用一条独立、用完即关的迁移连接,而不污染应用主连接。
	migDB, err := sql.Open("mysql", baseDSN()+"&multiStatements=true")
	if err != nil {
		log.Fatalf("migrate open: %v", err)
	}
	defer migDB.Close()

	driver, err := mysqldriver.WithInstance(migDB, &mysqldriver.Config{})
	if err != nil {
		log.Fatalf("migrate driver: %v", err)
	}

	source, err := (&file.File{}).Open("file://internal/migration")
	if err != nil {
		log.Fatalf("migrate source: %v", err)
	}

	m, err := migrate.NewWithInstance("file", source, "mysql", driver)
	if err != nil {
		log.Fatalf("migrate init: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migrate up: %v", err)
	}
	log.Println("migrations applied")
}

// baseDSN 构造不含 multiStatements 的连接串。DB_PASSWORD 必须显式提供,
// 缺失即 fatal —— 不再用硬编码的开发口令兜底(与 JWT_SECRET 的处理一致)。
func baseDSN() string {
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		log.Fatal("DB_PASSWORD must be set")
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=UTC",
		getEnv("DB_USER", "labelhub"),
		password,
		getEnv("DB_HOST", "127.0.0.1"),
		getEnv("DB_PORT", "13306"),
		getEnv("DB_NAME", "labelhub"),
	)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
