package firebase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"

	"github.com/hngprojects/telex_be/internal/config"
	"github.com/hngprojects/telex_be/utility"
)

// FCM rejects multicast requests with more than 500 tokens.
const maxTokensPerMulticast = 500

const callTTL = 30 * time.Second

// AndroidMessageChannelID must match the notification channel the mobile app creates.
const AndroidMessageChannelID = "zedu_messages"

func ConnectFirebase(logger *utility.Logger, cfg config.Firebase) {
	opt, ok := credentialsOption(cfg)
	if !ok {
		logger.Info("Firebase credentials not configured, FCM push notifications disabled")
		return
	}

	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		logger.Error("error initializing Firebase app: %v", err)
		return
	}

	client, err := app.Messaging(context.Background())
	if err != nil {
		logger.Error("error getting Firebase messaging client: %v", err)
		return
	}

	Client.App = app
	Client.Client = client
	logger.Info("Successfully initialized Firebase messaging client")
}

func credentialsOption(cfg config.Firebase) (option.ClientOption, bool) {
	raw := strings.TrimSpace(cfg.CredentialsJSON)
	if raw != "" {
		if !strings.HasPrefix(raw, "{") {
			decoded, err := base64.StdEncoding.DecodeString(raw)
			if err != nil {
				return nil, false
			}
			raw = string(decoded)
		}
		return option.WithCredentialsJSON([]byte(raw)), true
	}

	if strings.TrimSpace(cfg.ServiceFilePath) != "" {
		return option.WithCredentialsFile(cfg.ServiceFilePath), true
	}

	return nil, false
}

func IsEnabled() bool {
	return Client.Client != nil
}

// Message is a platform-neutral push. DataOnly messages are delivered to the app's
// messaging service without a system notification, which is how incoming calls ring.
type Message struct {
	Title    string
	Body     string
	ImageURL string
	Data     map[string]string
	DataOnly bool
}

type SendResult struct {
	SuccessCount int
	FailureCount int
	Delivered    []string
	// StaleTokens are tokens FCM reported as no longer registered; callers should delete them.
	StaleTokens []string
}

func SendToTokens(logger *utility.Logger, tokens []string, msg Message) (SendResult, error) {
	var result SendResult

	if !IsEnabled() {
		return result, fmt.Errorf("firebase messaging client not initialized")
	}

	for start := 0; start < len(tokens); start += maxTokensPerMulticast {
		end := min(start+maxTokensPerMulticast, len(tokens))
		batch := tokens[start:end]

		resp, err := Client.Client.SendEachForMulticast(context.Background(), buildMulticast(batch, msg))
		if err != nil {
			return result, fmt.Errorf("error sending FCM multicast: %w", err)
		}

		result.SuccessCount += resp.SuccessCount
		result.FailureCount += resp.FailureCount

		for i, r := range resp.Responses {
			if r.Success {
				result.Delivered = append(result.Delivered, batch[i])
				continue
			}
			if r.Error == nil {
				continue
			}
			// INVALID_ARGUMENT is not treated as stale: FCM also returns it for a malformed message,
			// which would otherwise wipe every valid token in the batch.
			if messaging.IsUnregistered(r.Error) || messaging.IsSenderIDMismatch(r.Error) {
				result.StaleTokens = append(result.StaleTokens, batch[i])
				continue
			}
			logger.Error("FCM send failed for one token: %v", r.Error)
		}
	}

	return result, nil
}

func buildMulticast(tokens []string, msg Message) *messaging.MulticastMessage {
	android := &messaging.AndroidConfig{Priority: "high"}

	m := &messaging.MulticastMessage{
		Tokens:  tokens,
		Data:    msg.Data,
		Android: android,
	}

	if msg.DataOnly {
		ttl := callTTL
		android.TTL = &ttl
		return m
	}

	m.Notification = &messaging.Notification{
		Title:    msg.Title,
		Body:     msg.Body,
		ImageURL: msg.ImageURL,
	}
	android.Notification = &messaging.AndroidNotification{
		ChannelID:    AndroidMessageChannelID,
		Sound:        "default",
		DefaultSound: true,
	}

	return m
}

// StringifyData converts an arbitrary payload into the string map FCM requires.
// Non-string values are JSON-encoded so the app can parse them back.
func StringifyData(payload map[string]interface{}) map[string]string {
	data := make(map[string]string, len(payload))
	for k, v := range payload {
		switch val := v.(type) {
		case nil:
			continue
		case string:
			data[k] = val
		default:
			encoded, err := json.Marshal(val)
			if err != nil {
				continue
			}
			data[k] = string(encoded)
		}
	}
	return data
}
