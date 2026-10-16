package models

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const PushDevicePlatformAndroid = "android"

// PushDevice is one FCM registration token. A user can have several (one per installed device).
type PushDevice struct {
	ID        string    `gorm:"column:id; type:uuid; not null; primaryKey" json:"id"`
	UserID    string    `gorm:"column:user_id; type:uuid; not null; index" json:"user_id"`
	Token     string    `gorm:"column:token; type:text; not null; uniqueIndex" json:"token"`
	Platform  string    `gorm:"column:platform; type:varchar(20); not null" json:"platform"`
	CreatedAt time.Time `gorm:"column:created_at; autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at; autoUpdateTime" json:"updated_at"`
}

type RegisterPushDeviceRequest struct {
	Token    string `json:"token" validate:"required,max=4096"`
	Platform string `json:"platform" validate:"required,oneof=android"`
}

type UnregisterPushDeviceRequest struct {
	Token string `json:"token" validate:"required,max=4096"`
}

// Upsert stores the token for the user. A token belongs to whichever account last signed in on that device.
func (d *PushDevice) Upsert(db *gorm.DB) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "token"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_id", "platform", "updated_at"}),
	}).Create(d).Error
}

func DeletePushDeviceForUser(db *gorm.DB, userID, token string) error {
	return db.Where("user_id = ? AND token = ?", userID, token).Delete(&PushDevice{}).Error
}

func DeletePushDeviceTokens(db *gorm.DB, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	return db.Where("token IN ?", tokens).Delete(&PushDevice{}).Error
}

func GetPushDevicesByUserIDs(db *gorm.DB, userIDs []string) ([]PushDevice, error) {
	var devices []PushDevice
	if len(userIDs) == 0 {
		return devices, nil
	}
	err := db.Where("user_id IN ?", userIDs).Find(&devices).Error
	return devices, err
}
