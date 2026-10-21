package office

import (
	"fmt"

	"github.com/hngprojects/telex_be/internal/models"
)

// The building grid. Room bounds are expressed in these units; the tile size
// matches the client grid (1 unit = 1 tile).
const (
	OfficeBuildingWidth  = 56.0
	OfficeBuildingHeight = 36.0
	OfficeTileSize       = 1.0
)

// DefaultMeetingRooms and DefaultFocusPods are the shared rooms every
// organisation gets before any office_room_configs override is applied.
const (
	DefaultMeetingRooms = 2
	DefaultFocusPods    = 4
)

// Directory is the derived room list for one requester.
type Directory struct {
	Rooms          []models.OfficeRoomResponse
	ChannelRoomMap map[string]string
	DMOfficeMap    map[string]string
}

// defaultBounds returns the derived footprint for a room. Channel and personal
// rooms are laid out deterministically so every client agrees on the map.
func defaultBounds(kind, ref string, index int) models.OfficeBounds {
	switch kind {
	case models.OfficeRoomKindLobby:
		return models.OfficeBounds{X: 1, Y: 1, W: 8, H: 6}
	case models.OfficeRoomKindReception:
		return models.OfficeBounds{X: 9, Y: 1, W: 8, H: 6}
	case models.OfficeRoomKindLounge:
		return models.OfficeBounds{X: 1, Y: 7, W: 8, H: 6}
	case models.OfficeRoomKindMeeting:
		return models.OfficeBounds{X: 18 + float64(index)*7, Y: 1, W: 6, H: 6}
	case models.OfficeRoomKindFocus:
		return models.OfficeBounds{X: 18 + float64(index%2)*5, Y: 8 + float64(index/2)*5, W: 4, H: 4}
	case models.OfficeRoomKindChannel:
		// Channel rooms sit in a row along the bottom of the building.
		return models.OfficeBounds{X: 2 + float64(index)*9, Y: 26, W: 8, H: 8}
	case models.OfficeRoomKindPersonal:
		// Personal offices sit in a column on the private wing.
		return models.OfficeBounds{X: 46, Y: 3 + float64(index)*8, W: 6, H: 7}
	default:
		return models.OfficeBounds{X: 0, Y: 0, W: 4, H: 4}
	}
}

func defaultCapacity(kind string) int {
	switch kind {
	case models.OfficeRoomKindLobby, models.OfficeRoomKindReception:
		return 60
	case models.OfficeRoomKindLounge:
		return 30
	case models.OfficeRoomKindChannel:
		return 30
	case models.OfficeRoomKindMeeting:
		return 8
	case models.OfficeRoomKindFocus:
		return 1
	case models.OfficeRoomKindPersonal:
		return 4
	default:
		return 8
	}
}

// BuildDirectory derives the rooms a requester can see.
//
// Shared rooms always exist, channel rooms follow the requester's channel
// memberships, and personal offices follow the organisation's members. The
// requester only sees the personal offices they are allowed to visit.
func (s *Service) BuildDirectory(orgID, userID string) (*Directory, error) {
	directory := &Directory{
		Rooms:          []models.OfficeRoomResponse{},
		ChannelRoomMap: map[string]string{},
		DMOfficeMap:    map[string]string{},
	}

	overrides, err := s.loadRoomConfigs(orgID)
	if err != nil {
		return nil, err
	}

	appendRoom := func(kind, ref, name string, index int, locked bool) {
		bounds := defaultBounds(kind, ref, index)
		capacity := defaultCapacity(kind)
		enabled := true

		if override, ok := overrides[roomKey(kind, ref)]; ok {
			if override.Bounds.W > 0 && override.Bounds.H > 0 {
				bounds = override.Bounds
			}
			if override.Capacity > 0 {
				capacity = override.Capacity
			}
			enabled = override.Enabled
		}

		if !enabled {
			return
		}

		directory.Rooms = append(directory.Rooms, models.OfficeRoomResponse{
			ID:       EncodeRoomID(kind, ref),
			Kind:     kind,
			Name:     name,
			RefID:    ref,
			Capacity: capacity,
			IsLocked: locked,
			Bounds:   bounds,
		})
	}

	// Shared rooms.
	appendRoom(models.OfficeRoomKindLobby, "", "Lobby", 0, false)
	appendRoom(models.OfficeRoomKindReception, "", "Reception", 0, false)
	appendRoom(models.OfficeRoomKindLounge, "", "Lounge", 0, false)
	for index := 0; index < DefaultMeetingRooms; index++ {
		appendRoom(models.OfficeRoomKindMeeting, fmt.Sprintf("%d", index+1), fmt.Sprintf("Meeting %d", index+1), index, false)
	}
	for index := 0; index < DefaultFocusPods; index++ {
		appendRoom(models.OfficeRoomKindFocus, fmt.Sprintf("%d", index+1), fmt.Sprintf("Focus %02d", index+1), index, false)
	}

	// Channel rooms follow the requester's memberships.
	channels, err := s.channelsForUser(orgID, userID)
	if err != nil {
		return nil, err
	}
	for index, channel := range channels {
		roomID := EncodeRoomID(models.OfficeRoomKindChannel, channel.ID)
		directory.ChannelRoomMap[channel.ID] = roomID
		appendRoom(models.OfficeRoomKindChannel, channel.ID, channel.Name, index, false)
	}

	// Personal offices follow the organisation's members, minus the offices
	// the requester may not visit. The slot is taken from the full member
	// list so the wing looks the same for everybody.
	members, err := s.organisationMembers(orgID)
	if err != nil {
		return nil, err
	}

	for index, member := range members {
		ref := models.OfficeRoomRef{Kind: models.OfficeRoomKindPersonal, Ref: member.UserID}
		access, err := s.CanAccess(orgID, userID, ref)
		if err != nil {
			return nil, err
		}
		if !access.CanEnter {
			continue
		}
		appendRoom(models.OfficeRoomKindPersonal, member.UserID, member.DisplayName, index, false)
	}

	// Direct messages map to the counterpart's office.
	dmMap, err := s.dmOfficeMap(orgID, userID)
	if err != nil {
		return nil, err
	}
	directory.DMOfficeMap = dmMap

	return directory, nil
}

func roomKey(kind, ref string) string {
	return kind + ":" + ref
}

// loadRoomConfigs returns the room overrides of an organisation keyed by
// "kind:ref".
func (s *Service) loadRoomConfigs(orgID string) (map[string]models.OfficeRoomConfig, error) {
	var configs []models.OfficeRoomConfig
	if err := s.Db.Postgresql.Where("organisation_id = ?", orgID).Find(&configs).Error; err != nil {
		return nil, err
	}

	overrides := make(map[string]models.OfficeRoomConfig, len(configs))
	for _, config := range configs {
		overrides[roomKey(config.RoomKind, config.RefID)] = config
	}
	return overrides, nil
}

// channelSummary is the little bit of channel data the directory needs.
type channelSummary struct {
	ID   string
	Name string
}

// channelsForUser returns the channels the requester belongs to.
func (s *Service) channelsForUser(orgID, userID string) ([]channelSummary, error) {
	type row struct {
		ID   string
		Name string
	}

	var rows []row
	err := s.Db.Postgresql.Table("channels").
		Select("channels.id, channels.name").
		Joins("JOIN user_channels ON user_channels.channels_id = channels.id").
		Where("user_channels.user_id = ?", userID).
		Where("channels.organisation_id = ?", orgID).
		Where("channels.deleted_at IS NULL").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	channels := make([]channelSummary, 0, len(rows))
	for _, row := range rows {
		channels = append(channels, channelSummary{ID: row.ID, Name: row.Name})
	}
	return channels, nil
}

// memberSummary is a member of the organisation.
type memberSummary struct {
	UserID      string
	DisplayName string
}

// organisationMembers returns the active members of the organisation.
func (s *Service) organisationMembers(orgID string) ([]memberSummary, error) {
	type row struct {
		UserID      string
		DisplayName string
	}

	var rows []row
	err := s.Db.Postgresql.Table("org_user_managements").
		Select("org_user_managements.user_id, COALESCE(profiles.display_name, users.name) AS display_name").
		Joins("LEFT JOIN profiles ON profiles.user_id = org_user_managements.user_id AND profiles.organisation_id = org_user_managements.organisation_id").
		Joins("LEFT JOIN users ON users.id = org_user_managements.user_id").
		Where("org_user_managements.organisation_id = ?", orgID).
		Where("org_user_managements.is_deactivated = ?", false).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	members := make([]memberSummary, 0, len(rows))
	for _, row := range rows {
		name := row.DisplayName
		if name == "" {
			name = "Teammate"
		}
		members = append(members, memberSummary{UserID: row.UserID, DisplayName: name})
	}
	return members, nil
}

// dmOfficeMap maps the id of a direct message to the personal office of the
// other participant. It is built per requester, so the entries only ever
// point at rooms the requester may visit.
func (s *Service) dmOfficeMap(orgID, userID string) (map[string]string, error) {
	officeMap := map[string]string{}

	// 1:1 direct messages.
	type dmRow struct {
		ID            string
		ChannelID     string
		UserID        string
		ParticipantID *string
		ChannelType   string
	}

	var rows []dmRow
	err := s.Db.Postgresql.Model(&models.DmChannels{}).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		if row.ParticipantID == nil || *row.ParticipantID == "" {
			continue
		}
		officeMap[row.ChannelID] = EncodeRoomID(models.OfficeRoomKindPersonal, *row.ParticipantID)
	}

	// Group direct messages resolve through channel_participants.
	type participantRow struct {
		ChannelID string
		UserID    string
	}

	var participants []participantRow
	err = s.Db.Postgresql.Model(&models.ChannelParticipant{}).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Find(&participants).Error
	if err != nil {
		return nil, err
	}

	for _, participant := range participants {
		officeMap[participant.ChannelID] = EncodeRoomID(models.OfficeRoomKindPersonal, participant.UserID)
	}

	return officeMap, nil
}
