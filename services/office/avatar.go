package office

import (
	"time"

	"github.com/hngprojects/telex_be/internal/models"
	"gorm.io/gorm"
)

// DefaultAvatar is used when a user has not picked a look yet.
func DefaultAvatar() models.OfficeAvatarConfig {
	return models.OfficeAvatarConfig{
		Tint:      "#8b5cf6",
		Body:      "b",
		Outfit:    "smart",
		Accessory: "glasses",
	}
}

// GetAvatar returns the office avatar of a user, falling back to the default.
func (s *Service) GetAvatar(userID string) (models.OfficeAvatarConfig, error) {
	var profile models.Profile
	err := s.Db.Postgresql.Where("userid = ?", userID).First(&profile).Error
	if err == gorm.ErrRecordNotFound {
		return DefaultAvatar(), nil
	}
	if err != nil {
		return DefaultAvatar(), err
	}

	if profile.AvatarConfig.Tint == "" {
		return DefaultAvatar(), nil
	}
	return profile.AvatarConfig, nil
}

// SaveAvatar stores the office avatar of a user.
func (s *Service) SaveAvatar(userID string, avatar models.OfficeAvatarConfig) error {
	var profile models.Profile
	err := s.Db.Postgresql.Where("userid = ?", userID).First(&profile).Error
	if err == gorm.ErrRecordNotFound {
		return err
	}
	if err != nil {
		return err
	}

	profile.AvatarConfig = avatar
	profile.UpdatedAt = time.Now().UTC()
	return s.Db.Postgresql.Save(&profile).Error
}
