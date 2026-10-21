package office

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/centrifuge"
)

// Office realtime channels (PRD section 6.4).
//
//	office:room:{encodedRoomId}:presence
//	office:room:{encodedRoomId}:layout
const (
	OfficeChannelPrefix   = "office:room:"
	PresenceChannelSuffix = ":presence"
	LayoutChannelSuffix   = ":layout"
)

// PresenceChannelName returns the Centrifugo channel carrying the presence of
// a room. The room id is encoded so the ":" inside it cannot break the
// channel name.
func PresenceChannelName(roomID string) string {
	return fmt.Sprintf("%s%s%s", OfficeChannelPrefix, url.QueryEscape(roomID), PresenceChannelSuffix)
}

// LayoutChannelName returns the Centrifugo channel announcing layout changes.
func LayoutChannelName(roomID string) string {
	return fmt.Sprintf("%s%s%s", OfficeChannelPrefix, url.QueryEscape(roomID), LayoutChannelSuffix)
}

// IsOfficeChannel reports whether a channel name belongs to the office.
func IsOfficeChannel(channel string) bool {
	return strings.HasPrefix(channel, OfficeChannelPrefix)
}

// ParseOfficeChannel splits an office channel back into the room id and the
// stream it carries.
func ParseOfficeChannel(channel string) (roomID string, stream string, ok bool) {
	if !IsOfficeChannel(channel) {
		return "", "", false
	}

	trimmed := strings.TrimPrefix(channel, OfficeChannelPrefix)
	switch {
	case strings.HasSuffix(trimmed, PresenceChannelSuffix):
		roomID = strings.TrimSuffix(trimmed, PresenceChannelSuffix)
		stream = "presence"
	case strings.HasSuffix(trimmed, LayoutChannelSuffix):
		roomID = strings.TrimSuffix(trimmed, LayoutChannelSuffix)
		stream = "layout"
	default:
		return "", "", false
	}

	decoded, err := url.QueryUnescape(roomID)
	if err != nil {
		return "", "", false
	}
	return decoded, stream, true
}

// PublishPresence announces that somebody entered, left or moved inside a room.
func (s *Service) PublishPresence(roomID string, presence models.OfficeOccupantResponse, eventType string) error {
	payload := map[string]any{
		"type":     eventType,
		"presence": presence,
	}

	return centrifuge.PublishChannelOptional(s.Logger, PresenceChannelName(roomID), payload)
}

// PublishLayoutUpdate announces that the live layout of a room changed.
func (s *Service) PublishLayoutUpdate(roomID string, version int64, updatedBy string) error {
	payload := map[string]any{
		"type":      "published",
		"version":   version,
		"roomId":    roomID,
		"updatedBy": updatedBy,
	}

	return centrifuge.PublishChannelOptional(s.Logger, LayoutChannelName(roomID), payload)
}
