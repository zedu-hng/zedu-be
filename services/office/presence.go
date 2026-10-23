package office

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/centrifuge"
)

// Office realtime channels (PRD section 6.4).
//
//	office:org:{orgId}:floor:{floorIndex}:room:{encodedRoomId}:presence
//	office:org:{orgId}:floor:{floorIndex}:room:{encodedRoomId}:layout
//
// The organisation is part of the channel name on purpose: rooms such as the
// lobby or focus pod 1 exist in every organisation, so a name without the
// organisation would carry one organisation's presence to another's.
const (
	OfficeChannelPrefix   = "office_org_"
	PresenceChannelSuffix = "_presence"
	LayoutChannelSuffix   = "_layout"
)

// OfficeChannelTarget names the room a realtime message belongs to.
type OfficeChannelTarget struct {
	OrgID      string
	FloorIndex int
	RoomID     string
}

// OfficeChannel is a parsed office channel name.
type OfficeChannel struct {
	OrgID      string
	FloorIndex int
	RoomID     string
	Stream     string
}

// NewChannelTarget builds the channel target of a room.
func NewChannelTarget(orgID string, ref models.OfficeRoomRef) OfficeChannelTarget {
	return OfficeChannelTarget{
		OrgID:      orgID,
		FloorIndex: RoomFloorIndex(ref),
		RoomID:     EncodeRoomID(ref.Kind, ref.Ref),
	}
}

// PresenceChannelName returns the Centrifugo channel carrying the presence of
// a room.
func PresenceChannelName(target OfficeChannelTarget) string {
	return officeChannelName(target, PresenceChannelSuffix)
}

// LayoutChannelName returns the Centrifugo channel announcing layout changes.
func LayoutChannelName(target OfficeChannelTarget) string {
	return officeChannelName(target, LayoutChannelSuffix)
}

func officeChannelName(target OfficeChannelTarget, suffix string) string {
	encoded := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(target.RoomID))
	return fmt.Sprintf(
		"%s%s_floor_%d_room_%s%s",
		OfficeChannelPrefix,
		target.OrgID,
		target.FloorIndex,
		encoded,
		suffix,
	)
}

// IsOfficeChannel reports whether a channel name belongs to the office.
func IsOfficeChannel(channel string) bool {
	return strings.HasPrefix(channel, OfficeChannelPrefix)
}

// ParseOfficeChannel splits an office channel back into the organisation, the
// floor, the room and the stream it carries.
func ParseOfficeChannel(channel string) (OfficeChannel, bool) {
	if !IsOfficeChannel(channel) {
		return OfficeChannel{}, false
	}

	trimmed := strings.TrimPrefix(channel, OfficeChannelPrefix)
	stream := ""
	switch {
	case strings.HasSuffix(trimmed, PresenceChannelSuffix):
		trimmed = strings.TrimSuffix(trimmed, PresenceChannelSuffix)
		stream = "presence"
	case strings.HasSuffix(trimmed, LayoutChannelSuffix):
		trimmed = strings.TrimSuffix(trimmed, LayoutChannelSuffix)
		stream = "layout"
	default:
		return OfficeChannel{}, false
	}

	// trimmed is now: {orgId}_floor_{n}_room_{base64url(roomId)}
	floorMarker := "_floor_"
	roomMarker := "_room_"

	floorIdx := strings.Index(trimmed, floorMarker)
	if floorIdx < 0 {
		return OfficeChannel{}, false
	}
	orgID := trimmed[:floorIdx]
	if orgID == "" {
		return OfficeChannel{}, false
	}

	rest := trimmed[floorIdx+len(floorMarker):]
	roomIdx := strings.Index(rest, roomMarker)
	if roomIdx < 0 {
		return OfficeChannel{}, false
	}
	floorStr := rest[:roomIdx]

	floorIndex, err := strconv.Atoi(floorStr)
	if err != nil || floorIndex < 0 {
		return OfficeChannel{}, false
	}

	roomB64 := rest[roomIdx+len(roomMarker):]
	roomIDBytes, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(roomB64)
	if err != nil {
		return OfficeChannel{}, false
	}

	return OfficeChannel{
		OrgID:      orgID,
		FloorIndex: floorIndex,
		RoomID:     string(roomIDBytes),
		Stream:     stream,
	}, true
}

// PublishPresence announces that somebody entered, left or moved inside a room.
func (s *Service) PublishPresence(target OfficeChannelTarget, presence models.OfficeOccupantResponse, eventType string) error {
	payload := map[string]any{
		"type":     eventType,
		"presence": presence,
	}

	return centrifuge.PublishChannelOptional(s.Logger, PresenceChannelName(target), payload)
}

// PublishLayoutUpdate announces that the live layout of a room changed.
func (s *Service) PublishLayoutUpdate(target OfficeChannelTarget, version int64, updatedBy string) error {
	payload := map[string]any{
		"type":      "published",
		"version":   version,
		"roomId":    target.RoomID,
		"updatedBy": updatedBy,
	}

	return centrifuge.PublishChannelOptional(s.Logger, LayoutChannelName(target), payload)
}
