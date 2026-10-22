package push_notifications

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/pushNotifications/firebase"
	"github.com/hngprojects/telex_be/pkg/repository/pushNotifications/onesignal"
	onesignalService "github.com/hngprojects/telex_be/services/onesignal"
	"github.com/hngprojects/telex_be/utility"
)

type PushDispatch struct {
	Req     models.PushRequest
	UserIDs []string
	// CallData marks the push as an incoming-call invite. It is sent as the data payload,
	// and on FCM as a data-only message so the Android app can ring natively.
	CallData map[string]interface{}
}

// Dispatch delivers a push over every transport a user is registered on: FCM for Android
// devices and OneSignal for the remaining (iOS and web) subscriptions. Each user gets one
// notification-history row no matter how many transports reached them. It returns how many
// users were reached.
func Dispatch(logger *utility.Logger, db *gorm.DB, p PushDispatch) (int, error) {
	reached := make(map[string]bool)
	var errs []error

	if err := dispatchOneSignal(logger, db, p, reached); err != nil {
		errs = append(errs, err)
	}

	if err := dispatchFCM(logger, db, p, reached); err != nil {
		errs = append(errs, err)
	}

	return len(reached), errors.Join(errs...)
}

func dispatchOneSignal(logger *utility.Logger, db *gorm.DB, p PushDispatch, reached map[string]bool) error {
	if !onesignal.IsEnabled() {
		return nil
	}

	var users []models.User
	if err := db.Where("id IN ? AND onesignal_subscription_id IS NOT NULL AND onesignal_subscription_id != ''", p.UserIDs).
		Select("id", "onesignal_subscription_id").
		Find(&users).Error; err != nil {
		return fmt.Errorf("failed to fetch OneSignal subscriptions: %w", err)
	}

	if len(users) == 0 {
		return nil
	}

	subscriptionIDs := make([]string, 0, len(users))
	userIDs := make([]string, 0, len(users))
	for _, u := range users {
		subscriptionIDs = append(subscriptionIDs, u.OneSignalSubscriptionID)
		userIDs = append(userIDs, u.ID)
	}

	var err error
	if p.CallData != nil {
		err = onesignal.SendDirectCallNotification(logger, subscriptionIDs, p.Req, p.CallData, db, userIDs)
	} else {
		err = onesignal.SendBatchNotifications(logger, subscriptionIDs, p.Req, db, userIDs)
	}
	if err != nil {
		return err
	}

	for _, id := range userIDs {
		reached[id] = true
	}
	return nil
}

func dispatchFCM(logger *utility.Logger, db *gorm.DB, p PushDispatch, reached map[string]bool) error {
	if !firebase.IsEnabled() {
		return nil
	}

	devices, err := models.GetPushDevicesByUserIDs(db, p.UserIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch push devices: %w", err)
	}

	if len(devices) == 0 {
		return nil
	}

	tokens := make([]string, 0, len(devices))
	tokenOwner := make(map[string]string, len(devices))
	for _, d := range devices {
		tokens = append(tokens, d.Token)
		tokenOwner[d.Token] = d.UserID
	}

	data := p.CallData
	if data == nil {
		data, _ = p.Req.Payload.(map[string]interface{})
	}

	result, err := firebase.SendToTokens(logger, tokens, firebase.Message{
		Title:    formatPushTitle(p.Req.Title),
		Body:     p.Req.Message,
		ImageURL: p.Req.AvatarUrl,
		Data:     firebase.StringifyData(data),
		DataOnly: p.CallData != nil,
	})

	if len(result.StaleTokens) > 0 {
		if delErr := models.DeletePushDeviceTokens(db, result.StaleTokens); delErr != nil {
			logger.Error("failed to delete stale FCM tokens: %v", delErr)
		}
	}

	if err != nil {
		return err
	}

	newlyReached := make([]string, 0, len(result.Delivered))
	for _, token := range result.Delivered {
		userID := tokenOwner[token]
		if reached[userID] {
			continue
		}
		reached[userID] = true
		newlyReached = append(newlyReached, userID)
	}

	logger.Info("Sent FCM notification to %d device(s), %d failed", result.SuccessCount, result.FailureCount)

	if len(newlyReached) == 0 {
		return nil
	}

	if _, _, err := onesignalService.SaveBatchNotifications(db, utility.GenerateUUID(), newlyReached, p.Req.Title, p.Req.Message, p.Req.AvatarUrl, data, p.Req.OrgId); err != nil {
		logger.Error("Failed to save FCM notification history: %v", err)
	}

	return nil
}

// formatPushTitle matches the title casing the OneSignal transport applies, so both platforms look the same.
func formatPushTitle(title string) string {
	return strings.Title(strings.ToLower(title))
}
