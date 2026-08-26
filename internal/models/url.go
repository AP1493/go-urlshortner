package models

import "time"

type URL struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	URL         string    `gorm:"not null" json:"url"`
	ShortenCode string    `gorm:"unique" json:"shorten_code"`
	Count       int       `gorm:"default:0" json:"count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
