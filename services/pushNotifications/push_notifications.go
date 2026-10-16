package push_notifications

import (
	"fmt"
	"strings"

	"github.com/SherClockHolmes/webpush-go"
	"gorm.io/gorm"

	"github.com/hngprojects/telex_be/internal/models"
	repoWebpush "github.com/hngprojects/telex_be/pkg/repository/webpush"
	fcmtokens "github.com/hngprojects/telex_be/services/fcmTokens"
	notificationpref "github.com/hngprojects/telex_be/services/notification_pref"
	"github.com/hngprojects/telex_be/utility"
)

// SendPush sends a push notification to a user
func SendWebPush(req models.PushRequest, logger *utility.Logger, db *gorm.DB) error {

	if req.Payload == "" {
		return nil
	}

	sub, exists, _ := fcmtokens.GetWebPushTokenByUserId(req.UserId, db)

	if !exists {
		return nil
	}

	subscription := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.Keys.P256dh,
			Auth:   sub.Keys.Auth,
		},
	}

	err := repoWebpush.SendPush(req.Payload, subscription)

	return err
}

// SendPush sends a push notification to users
func SendWebPushToUsers(req models.PushRequest, logger *utility.Logger, db *gorm.DB) error {
	var (
		channel models.Channels
	)

	userArr := make([]string, 0)

	if len(req.UserIds) == 0 {
		users, err := channel.FetchChannelUsers(db, req.ChannelId, req.UserId)

		if err != nil {
			logger.Error(fmt.Sprintf("Failed to send mass push notification, %s", err.Error()))
			return err
		}

		userArr = users

	} else {
		userArr = req.UserIds
	}

	webPushTokens, err := fcmtokens.GetWebPushTokenByUserIds(userArr, db)

	if len(*webPushTokens) == 0 {
		return nil
	}

	errs := ""

	for _, sub := range *webPushTokens {

		subscription := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.Keys.P256dh,
				Auth:   sub.Keys.Auth,
			},
		}

		err := repoWebpush.SendPush(req.Payload, subscription)
		errs = fmt.Sprintf("%s///%s", errs, err.Error())

	}

	if errs != "" {
		logger.Error(fmt.Sprintf("Failed to send mass push notification, %s", err.Error()))
		return fmt.Errorf("%s", errs)
	}

	return nil
}

func ResolveNotifType(payload any) notificationpref.NotificationType {
	if payloadMap, ok := payload.(map[string]interface{}); ok {
		if notifType, exists := payloadMap["notification_type"].(string); exists {
			switch notifType {
			case "dm":
				return notificationpref.NotificationTypeDirectMessage
			case "channel":
				return notificationpref.NotificationTypeAllMessages
			}
		}
	}
	return notificationpref.NotificationTypeAllMessages
}

// PushToUser sends a push notification to a single user on every device they registered.
func PushToUser(req models.PushRequest, logger *utility.Logger, db *gorm.DB) error {
	if isSystemMessage(req) {
		logger.Info("Skipping push notification for system message in channel %s", req.ChannelId)
		return nil
	}

	if req.ChannelId != "" && req.OrgId != "" {
		shouldSend, err := notificationpref.ShouldSendNotification(db, req.UserId, req.ChannelId, req.OrgId, ResolveNotifType(req.Payload))
		if err != nil {
			logger.Error("failed to check notification preferences: %v", err)
			return nil
		}
		if !shouldSend {
			logger.Info("User %s has muted or disabled notifications for channel %s", req.UserId, req.ChannelId)
			return nil
		}
	}

	if _, err := Dispatch(logger, db, PushDispatch{Req: req, UserIDs: []string{req.UserId}}); err != nil {
		logger.Error("failed to send push notification: %s", err.Error())
	}

	return nil
}

// PushToUsers sends a push notification to the given users, or to the channel's members when UserIds is empty.
func PushToUsers(req models.PushRequest, logger *utility.Logger, db *gorm.DB) error {
	if isSystemMessage(req) {
		logger.Info("Skipping push notification for system message in channel %s", req.ChannelId)
		return nil
	}

	var channel models.Channels
	userArr := req.UserIds

	if len(userArr) == 0 {
		users, err := channel.FetchChannelUsers(db, req.ChannelId, req.UserId)
		if err != nil {
			logger.Error("failed to fetch channel users: %s", err.Error())
			return nil
		}
		userArr = users
	}

	if !req.SkipPreferenceFilter && req.ChannelId != "" && req.OrgId != "" {
		filteredUsers, err := notificationpref.FilterUsersByPreferences(db, userArr, req.ChannelId, req.OrgId, ResolveNotifType(req.Payload))
		if err != nil {
			logger.Error("failed to filter users by preferences: %v", err)
			return err
		}
		userArr = filteredUsers
	}

	if len(userArr) == 0 {
		logger.Info("No users to send push notification to after preference filtering")
		return nil
	}

	req.Title = ResolvePushTitle(req, db)
	req.Message = ResolvePushBody(req.Username, req.Message)

	if _, err := Dispatch(logger, db, PushDispatch{Req: req, UserIDs: userArr}); err != nil {
		logger.Error("failed to send batch push notification: %s", err.Error())
	}

	return nil
}

// SendNotificationToUser sends a push notification to a specific user and fails when they have no registered device.
func SendNotificationToUser(logger *utility.Logger, db *gorm.DB, userID string, req models.OneSignalPushRequest) error {
	pushReq := models.PushRequest{
		Title:     req.Title,
		Message:   req.Message,
		AvatarUrl: req.AvatarUrl,
	}

	reached, err := Dispatch(logger, db, PushDispatch{Req: pushReq, UserIDs: []string{userID}})
	if err != nil {
		logger.Error("failed to send push notification: %s", err.Error())
		return fmt.Errorf("failed to send push notification")
	}

	if reached == 0 {
		return fmt.Errorf("user has no registered push device")
	}

	return nil
}

// PushToUsersForBroadcast sends an admin broadcast and returns how many users had a device to receive it.
func PushToUsersForBroadcast(req models.PushRequest, logger *utility.Logger, db *gorm.DB, userIDs []string) (int, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}

	reached, err := Dispatch(logger, db, PushDispatch{Req: req, UserIDs: userIDs})
	if err != nil {
		logger.Error("failed to send broadcast push notification: %s", err.Error())
		return 0, fmt.Errorf("failed to send broadcast notification: %w", err)
	}

	logger.Info("Successfully sent broadcast notification to %d users", reached)
	return reached, nil
}

func isSystemMessage(req models.PushRequest) bool {
	payloadMap, ok := req.Payload.(map[string]interface{})
	if !ok {
		return false
	}
	msgType, exists := payloadMap["type"].(string)
	return exists && msgType == "system"
}

func ResolvePushTitle(req models.PushRequest, db *gorm.DB) string {
	channelName := strings.TrimSpace(req.ChannelName)

	if channelName == "" && req.ChannelId != "" {
		var ch models.Channels
		if err := db.Where("id = ?", req.ChannelId).Select("name").First(&ch).Error; err == nil && ch.Name != "" {
			channelName = strings.TrimSpace(ch.Name)
		}
	}

	isDM := false
	if payloadMap, ok := req.Payload.(map[string]interface{}); ok {
		if notifType, exists := payloadMap["notification_type"].(string); exists && (notifType == "dm" || notifType == "group_dm") {
			isDM = true
		}
		if section, exists := payloadMap["section"].(string); exists && (section == "dm" || section == "reply_dm") {
			isDM = true
		}
	}

	if channelName != "" {
		if isDM || strings.HasPrefix(channelName, "@") {
			return channelName
		}
		if strings.HasPrefix(channelName, "#") {
			return channelName
		}
		return fmt.Sprintf("#%s", channelName)
	}

	title := strings.TrimSpace(req.Title)
	if title != "" && title != "#" && title != "# " {
		return title
	}
	if req.Username != "" {
		return req.Username
	}
	return "New Message"
}

func ResolvePushBody(username, message string) string {
	msg := strings.TrimSpace(message)
	uname := strings.TrimSpace(username)
	if uname != "" && !strings.HasPrefix(msg, fmt.Sprintf("(%s):", uname)) && !strings.HasPrefix(msg, fmt.Sprintf("(%s)", uname)) {
		return fmt.Sprintf("(%s): %s", uname, msg)
	}
	return msg
}
