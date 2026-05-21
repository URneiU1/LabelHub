package db

import (
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
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=UTC&multiStatements=true",
		getEnv("DB_USER", "labelhub"),
		getEnv("DB_PASSWORD", "labelhub_dev"),
		getEnv("DB_HOST", "127.0.0.1"),
		getEnv("DB_PORT", "13306"),
		getEnv("DB_NAME", "labelhub"),
	)

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	return DB
}

func RunMigrations() {
	if DB == nil {
		log.Fatal("database is not initialized")
	}

	db, err := DB.DB()
	if err != nil {
		log.Fatalf("database handle: %v", err)
	}

	driver, err := mysqldriver.WithInstance(db, &mysqldriver.Config{})
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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
