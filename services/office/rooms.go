package office

import (
	"fmt"

	"github.com/hngprojects/telex_be/internal/models"
	"gorm.io/gorm"
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
	Floors         map[string]int
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
		// Channel rooms sit in rows along the bottom of the building.
		// 6 rooms per row (6*9=54, plus 2 margin = 56 building width).
		row := index / 6
		col := index % 6
		return models.OfficeBounds{
			X: 2 + float64(col)*9,
			Y: 26 + float64(row)*9,
			W: 8,
			H: 8,
		}
	case models.OfficeRoomKindPersonal:
		// Two tiers of twenty walled offices per member floor, so forty
		// offices fit along half a corridor length.
		slot := index % models.OfficeFloorCapacity
		tier := slot / 20
		column := slot % 20
		return models.OfficeBounds{
			X: 2 + float64(column)*2.6,
			Y: 2 + float64(tier)*5.2,
			W: 2.4,
			H: 4.6,
		}
	case models.OfficeRoomKindArea:
		// The extras sit in a row past the desks on the same floor.
		return models.OfficeBounds{X: 2 + float64(index)*8, Y: 56, W: 7, H: 6}
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
	case models.OfficeRoomKindArea:
		return 12
	default:
		return 8
	}
}

// BuildDirectory derives the rooms a requester can see.
//
// Shared rooms always exist, channel rooms follow the requester's channel
// memberships, and personal offices follow the organisation's members. The
// requester only sees the personal offices they are allowed to visit.
func (s *Service) BuildDirectory(orgID, userID string, sets AccessSets) (*Directory, error) {
	directory := &Directory{
		Rooms:          []models.OfficeRoomResponse{},
		ChannelRoomMap: map[string]string{},
		DMOfficeMap:    map[string]string{},
	}

	overrides, err := s.loadRoomConfigs(orgID)
	if err != nil {
		return nil, err
	}

	floors, err := s.SyncFloorAssignments(orgID)
	if err != nil {
		return nil, err
	}
	directory.Floors = floors

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
			ID:         EncodeRoomID(kind, ref),
			Kind:       kind,
			Name:       name,
			RefID:      ref,
			Capacity:   capacity,
			IsLocked:   locked,
			FloorIndex: RoomFloorIndex(models.OfficeRoomRef{Kind: kind, Ref: ref}),
			Bounds:     bounds,
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
		// Access is already known from sets - we only got channels the user belongs to
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
		access := CanAccessFromSets(sets, ref, userID)
		if !access.CanEnter {
			continue
		}
		// The slot comes from the full member list so the wing looks the same
		// for everybody, but the floor comes from the tower assignment.
		before := len(directory.Rooms)
		appendRoom(models.OfficeRoomKindPersonal, member.UserID, member.DisplayName, index, false)
		if len(directory.Rooms) > before {
			directory.Rooms[len(directory.Rooms)-1].FloorIndex = memberFloor(floors, member.UserID)
		}
	}

	// Every member floor carries its extras. They are rooms in their own right
	// so presence and access follow them, and nobody is allocated a desk there.
	for floor := models.OfficeFloorMember; floor <= highestFloor(floors); floor++ {
		for index, area := range AreaRoomsForFloor(floor) {
			_, key, ok := ParseAreaRef(area.Ref)
			if !ok {
				continue
			}
			appendRoom(area.Kind, area.Ref, AreaRoomName(key), index, false)
		}
	}

	// Direct messages map to the counterpart's office.
	dmMap, err := s.dmOfficeMap(orgID, userID)
	if err != nil {
		return nil, err
	}
	directory.DMOfficeMap = dmMap

	return directory, nil
}

// GetRoom returns a single room by its reference, using the precomputed access sets.
// It builds only the necessary room without constructing the entire directory.
func (s *Service) GetRoom(orgID, userID string, sets AccessSets, ref models.OfficeRoomRef) (*models.OfficeRoomResponse, error) {
	overrides, err := s.loadRoomConfigs(orgID)
	if err != nil {
		return nil, err
	}

	// Check access first
	access := CanAccessFromSets(sets, ref, userID)
	if !access.CanEnter {
		return nil, nil // caller will handle 403/404
	}

	floors, err := s.SyncFloorAssignments(orgID)
	if err != nil {
		return nil, err
	}

	// Build just this room
	kind := ref.Kind
	refStr := ref.Ref
	name := ""
	index := 0
	locked := false

	switch kind {
	case models.OfficeRoomKindLobby:
		name = "Lobby"
	case models.OfficeRoomKindReception:
		name = "Reception"
	case models.OfficeRoomKindLounge:
		name = "Lounge"
	case models.OfficeRoomKindMeeting:
		name = fmt.Sprintf("Meeting %s", refStr)
	case models.OfficeRoomKindFocus:
		name = fmt.Sprintf("Focus %02s", refStr)
	case models.OfficeRoomKindChannel:
		// Need to get channel name
		channels, err := s.channelsForUser(orgID, "")
		if err == nil {
			for _, ch := range channels {
				if ch.ID == refStr {
					name = ch.Name
					break
				}
			}
		}
		if name == "" {
			name = fmt.Sprintf("Channel %s", refStr)
		}
	case models.OfficeRoomKindPersonal:
		// Get member display name
		members, err := s.organisationMembers(orgID)
		if err == nil {
			for _, m := range members {
				if m.UserID == refStr {
					name = m.DisplayName
					break
				}
			}
		}
		if name == "" {
			name = fmt.Sprintf("Office %s", refStr)
		}
	case models.OfficeRoomKindArea:
		if _, key, ok := ParseAreaRef(refStr); ok {
			name = AreaRoomName(key)
		} else {
			name = "Area"
		}
	}

	bounds := defaultBounds(kind, refStr, index)
	capacity := defaultCapacity(kind)
	enabled := true

	if override, ok := overrides[roomKey(kind, refStr)]; ok {
		if override.Bounds.W > 0 && override.Bounds.H > 0 {
			bounds = override.Bounds
		}
		if override.Capacity > 0 {
			capacity = override.Capacity
		}
		enabled = override.Enabled
	}

	if !enabled {
		return nil, nil
	}

	floorIndex := RoomFloorIndex(ref)
	if kind == models.OfficeRoomKindPersonal {
		floorIndex = memberFloor(floors, refStr)
	}

	return &models.OfficeRoomResponse{
		ID:         EncodeRoomID(kind, refStr),
		Kind:       kind,
		Name:       name,
		RefID:      refStr,
		Capacity:   capacity,
		IsLocked:   locked,
		FloorIndex: floorIndex,
		Bounds:     bounds,
	}, nil
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

// RoomCapacity is the number of people a room holds. A focus pod holds one
// person, a meeting room a handful, a channel room a crowd.
func (s *Service) RoomCapacity(orgID string, ref models.OfficeRoomRef) int {
	capacity := defaultCapacity(ref.Kind)

	var config models.OfficeRoomConfig
	err := s.Db.Postgresql.Where("organisation_id = ? AND room_kind = ? AND ref_id = ?", orgID, ref.Kind, ref.Ref).
		First(&config).Error
	if err == gorm.ErrRecordNotFound {
		return capacity
	}
	if err != nil {
		return capacity
	}
	if config.Capacity > 0 {
		return config.Capacity
	}

	return capacity
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
		Joins("LEFT JOIN profiles ON profiles.userid = org_user_managements.user_id AND (profiles.organisation_id IS NULL OR profiles.organisation_id = org_user_managements.organisation_id)").
		Joins("LEFT JOIN users ON users.id = org_user_managements.user_id").
		Where("org_user_managements.organisation_id = ?", orgID).
		Where("org_user_managements.is_deactivated = ?", false).
		Order("org_user_managements.created_at ASC, org_user_managements.user_id ASC").
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

// DisplayName is the name a member is known by inside the office.
func (s *Service) DisplayName(orgID, userID string) string {
	var profile models.Profile
	err := s.Db.Postgresql.Where("userid = ? AND (organisation_id IS NULL OR organisation_id = ?)", userID, orgID).
		First(&profile).Error
	if err != nil {
		return ""
	}

	if profile.DisplayName != "" {
		return profile.DisplayName
	}

	var user models.User
	if err := s.Db.Postgresql.Where("id = ?", userID).First(&user).Error; err == nil {
		return user.Name
	}

	return ""
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
