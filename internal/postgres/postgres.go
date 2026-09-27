package postgres

import (
	"fmt"
	"os"

	"github.com/AP1493/go-urlshortner/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Postgresql struct {
	db *gorm.DB
}

func InitPostgres() (Postgresql, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", os.Getenv("DB_HOST"), os.Getenv("DB_PORT"),
		os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"), os.Getenv("POSTGRES_DB"), os.Getenv("DB_SSLMODE"))

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return Postgresql{}, fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return Postgresql{}, fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)

	if err := db.AutoMigrate(&models.URL{}); err != nil {
		return Postgresql{}, fmt.Errorf("failed to run migrations: %w", err)
	}

	return Postgresql{db: db}, nil
}

func (p *Postgresql) DB() *gorm.DB {
	return p.db
}

func (p *Postgresql) Close() error {
	sqlDB, err := p.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	return sqlDB.Close()
}
