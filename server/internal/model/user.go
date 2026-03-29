package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents the users table.
type User struct {
	ID                  string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AppleID             string     `gorm:"column:apple_id;type:varchar(255);not null" json:"apple_id"`
	DisplayName         string     `gorm:"type:varchar(100);not null;default:''" json:"display_name"`
	DeviceID            string     `gorm:"column:device_id;type:varchar(255);not null;default:''" json:"device_id"`
	DNDStart            *string    `gorm:"column:dnd_start;type:time" json:"dnd_start,omitempty"`
	DNDEnd              *string    `gorm:"column:dnd_end;type:time" json:"dnd_end,omitempty"`
	IsDeleted           bool       `gorm:"not null;default:false" json:"is_deleted"`
	DeletionScheduledAt *time.Time `gorm:"type:timestamptz" json:"deletion_scheduled_at,omitempty"`
	CreatedAt           time.Time  `gorm:"type:timestamptz;not null;default:now()" json:"created_at"`
	LastActiveAt        time.Time  `gorm:"column:last_active_at;type:timestamptz;not null;default:now()" json:"last_active_at"`
}

// TableName returns the table name for GORM.
func (User) TableName() string {
	return "users"
}

// BeforeCreate generates a UUID if not set.
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}
