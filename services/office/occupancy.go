package office

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hngprojects/telex_be/internal/models"
)

const (
	// presenceTTL keeps stale positions from lingering when a client goes away
	// without sending a leave event.
	presenceTTL = 5 * time.Minute
	// presenceKeyPrefix namespaces the presence hashes in Redis.
	presenceKeyPrefix = "office:presence:"
)

func presenceKey(roomID string) string {
	return fmt.Sprintf("%s%s", presenceKeyPrefix, roomID)
}

// JoinRoom records that a user is standing in a room and announces it.
func (s *Service) JoinRoom(command models.OfficePresenceCommand) error {
	roomID := EncodeRoomID(command.Room.Kind, command.Room.Ref)
	presence := command.Presence
	presence.RoomID = roomID
	presence.UpdatedAt = time.Now().UTC()

	if s.Db.Redis != nil {
		payload, err := json.Marshal(presence)
		if err != nil {
			return err
		}

		key := presenceKey(roomID)
		ctx := context.Background()
		if err := s.Db.Redis.HSet(ctx, key, command.UserID, payload).Err(); err != nil {
			return err
		}
		if err := s.Db.Redis.Expire(ctx, key, presenceTTL).Err(); err != nil {
			return err
		}
	}

	return s.PublishPresence(roomID, presence, "join")
}

// UpdatePresence refreshes the position of a user inside a room.
func (s *Service) UpdatePresence(command models.OfficePresenceCommand) error {
	roomID := EncodeRoomID(command.Room.Kind, command.Room.Ref)
	presence := command.Presence
	presence.RoomID = roomID
	presence.UpdatedAt = time.Now().UTC()

	if s.Db.Redis != nil {
		payload, err := json.Marshal(presence)
		if err != nil {
			return err
		}

		key := presenceKey(roomID)
		ctx := context.Background()
		if err := s.Db.Redis.HSet(ctx, key, command.UserID, payload).Err(); err != nil {
			return err
		}
		if err := s.Db.Redis.Expire(ctx, key, presenceTTL).Err(); err != nil {
			return err
		}
	}

	return s.PublishPresence(roomID, presence, "update")
}

// LeaveRoom removes a user from a room and announces it.
func (s *Service) LeaveRoom(orgID, userID string, ref models.OfficeRoomRef) error {
	roomID := EncodeRoomID(ref.Kind, ref.Ref)

	if s.Db.Redis != nil {
		ctx := context.Background()
		key := presenceKey(roomID)
		if err := s.Db.Redis.HDel(ctx, key, userID).Err(); err != nil {
			return err
		}
	}

	presence := models.OfficeOccupantResponse{
		UserID: userID,
		RoomID: roomID,
	}
	return s.PublishPresence(roomID, presence, "leave")
}

// Occupants returns the people currently standing in a room.
func (s *Service) Occupants(orgID string, ref models.OfficeRoomRef) ([]models.OfficeOccupantResponse, error) {
	occupants := []models.OfficeOccupantResponse{}
	if s.Db.Redis == nil {
		return occupants, nil
	}

	roomID := EncodeRoomID(ref.Kind, ref.Ref)
	ctx := context.Background()
	values, err := s.Db.Redis.HGetAll(ctx, presenceKey(roomID)).Result()
	if err != nil {
		return nil, err
	}

	for _, raw := range values {
		var occupant models.OfficeOccupantResponse
		if err := json.Unmarshal([]byte(raw), &occupant); err != nil {
			continue
		}
		occupants = append(occupants, occupant)
	}

	return occupants, nil
}
