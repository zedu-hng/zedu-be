package pushdevices

import (
	"net/http"

	"gorm.io/gorm"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/utility"
)

// RegisterDevice stores the device's FCM token. Android has moved off OneSignal, so the user's
// OneSignal subscription is cleared too; it holds a single device and would otherwise keep
// delivering duplicate pushes to the same phone through the old OneSignal registration.
func RegisterDevice(db *gorm.DB, userID string, req models.RegisterPushDeviceRequest) (int, error) {
	device := models.PushDevice{
		ID:       utility.GenerateUUID(),
		UserID:   userID,
		Token:    req.Token,
		Platform: req.Platform,
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := device.Upsert(tx); err != nil {
			return err
		}
		if req.Platform != models.PushDevicePlatformAndroid {
			return nil
		}
		return tx.Model(&models.User{}).Where("id = ?", userID).Update("onesignal_subscription_id", "").Error
	})
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

func UnregisterDevice(db *gorm.DB, userID string, req models.UnregisterPushDeviceRequest) (int, error) {
	if err := models.DeletePushDeviceForUser(db, userID, req.Token); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}
