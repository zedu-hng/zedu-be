package office

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/storage"
	"github.com/hngprojects/telex_be/utility"
	"gorm.io/gorm"
)

// Service holds the dependencies for the office domain.
type Service struct {
	Db     *storage.Database
	Logger *utility.Logger
}

// New builds the office service.
func New(db *storage.Database, logger *utility.Logger) *Service {
	return &Service{Db: db, Logger: logger}
}

// ---------------------------------------------------------------------------
// Room identifiers
// ---------------------------------------------------------------------------

// EncodeRoomID builds the opaque composite id used by every office route.
// Shared rooms without a reference (lobby, reception, lounge) are just the
// kind; everything else is "kind:ref".
func EncodeRoomID(kind, ref string) string {
	if ref == "" {
		return kind
	}
	return fmt.Sprintf("%s:%s", kind, ref)
}

// ParseRoomID turns an opaque room id back into its kind and reference. It
// accepts both the decoded ("channel:ch_1") and the URL encoded
// ("channel%3Ach_1") form, because clients may send either.
func ParseRoomID(roomID string) (models.OfficeRoomRef, error) {
	decoded, err := url.PathUnescape(strings.TrimSpace(roomID))
	if err != nil {
		decoded = strings.TrimSpace(roomID)
	}

	if decoded == "" {
		return models.OfficeRoomRef{}, errors.New("room id is required")
	}

	kind, ref, found := strings.Cut(decoded, ":")
	switch kind {
	case models.OfficeRoomKindLobby, models.OfficeRoomKindReception, models.OfficeRoomKindLounge:
		return models.OfficeRoomRef{Kind: kind}, nil
	case models.OfficeRoomKindMeeting, models.OfficeRoomKindFocus:
		if !found || ref == "" {
			return models.OfficeRoomRef{}, fmt.Errorf("room id %q is missing its reference", roomID)
		}
		return models.OfficeRoomRef{Kind: kind, Ref: ref}, nil
	case models.OfficeRoomKindChannel, models.OfficeRoomKindPersonal:
		if !found || ref == "" {
			return models.OfficeRoomRef{}, fmt.Errorf("room id %q is missing its reference", roomID)
		}
		return models.OfficeRoomRef{Kind: kind, Ref: ref}, nil
	default:
		return models.OfficeRoomRef{}, fmt.Errorf("unknown room kind %q", kind)
	}
}

// IsSharedRoom reports whether the room is shared by the whole organisation
// (as opposed to a personal office).
func IsSharedRoom(ref models.OfficeRoomRef) bool {
	return ref.Kind != models.OfficeRoomKindPersonal
}

// ---------------------------------------------------------------------------
// Access control (PRD section 6.5)
// ---------------------------------------------------------------------------

// Access describes what a requester may do inside a room.
type Access struct {
	CanEnter         bool
	CanEditFurniture bool
	CanPublish       bool
	CanReset         bool
}

// IsOrganisationAdmin reports whether the user holds the owner or administrator
// role in the organisation.
func (s *Service) IsOrganisationAdmin(orgID, userID string) bool {
	var membership models.OrgUserManagement
	if err := s.Db.Postgresql.Where("organisation_id = ? AND user_id = ?", orgID, userID).First(&membership).Error; err != nil {
		return false
	}
	if membership.RoleID == "" {
		return false
	}

	var role models.OrgRole
	if err := s.Db.Postgresql.Where("id = ?", membership.RoleID).First(&role).Error; err != nil {
		return false
	}

	if strings.EqualFold(role.Name, models.OrgRoleNameOwner) ||
		strings.EqualFold(role.Name, models.OrgRoleNameAdministrator) {
		return true
	}

	return models.OrgUserHasPermission(role.Permissions.PermissionList, models.PermManageOrganization)
}

// IsOrganisationMember reports whether the user belongs to the organisation.
func (s *Service) IsOrganisationMember(orgID, userID string) bool {
	var membership models.OrgUserManagement
	return s.Db.Postgresql.Where("organisation_id = ? AND user_id = ? AND is_deactivated = ?", orgID, userID, false).
		First(&membership).Error == nil
}

// CanAccess resolves the permissions of a requester for a room.
func (s *Service) CanAccess(orgID, userID string, ref models.OfficeRoomRef) (Access, error) {
	access := Access{}

	if !s.IsOrganisationMember(orgID, userID) {
		return access, nil
	}

	isAdmin := s.IsOrganisationAdmin(orgID, userID)

	// Personal offices: owner, organisation admin, or a DM participant.
	if ref.Kind == models.OfficeRoomKindPersonal {
		if ref.Ref == userID {
			access.CanEnter = true
			access.CanEditFurniture = true
			return access, nil
		}
		if isAdmin {
			access.CanEnter = true
			return access, nil
		}
		guest, err := s.isDirectMessageParticipant(orgID, userID, ref.Ref)
		if err != nil {
			return access, err
		}
		access.CanEnter = guest
		return access, nil
	}

	// Shared rooms: everybody in the organisation may enter the building's
	// common areas, but a channel room needs a membership.
	if ref.Kind == models.OfficeRoomKindChannel {
		member, err := s.IsChannelMember(userID, ref.Ref)
		if err != nil {
			return access, err
		}
		if !member && !isAdmin {
			return access, nil
		}
	}

	access.CanEnter = true
	access.CanEditFurniture = isAdmin
	access.CanPublish = isAdmin
	access.CanReset = isAdmin
	return access, nil
}

// isDirectMessageParticipant reports whether requesterID has a direct message
// (1:1 or group) with counterpartID inside the organisation.
func (s *Service) isDirectMessageParticipant(orgID, requesterID, counterpartID string) (bool, error) {
	var count int64

	// 1:1 direct messages: dm_channels holds one row per participant pair.
	err := s.Db.Postgresql.Model(&models.DmChannels{}).
		Where("org_id = ? AND user_id = ? AND participant_id = ?", orgID, requesterID, counterpartID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}

	// Group direct messages resolve through channel_participants.
	err = s.Db.Postgresql.Model(&models.ChannelParticipant{}).
		Where("org_id = ? AND user_id = ?", orgID, requesterID).
		Where("channel_id IN (?)", s.Db.Postgresql.Model(&models.ChannelParticipant{}).
			Select("channel_id").
			Where("org_id = ? AND user_id = ?", orgID, counterpartID)).
		Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// IsChannelMember reports whether the user belongs to a channel.
func (s *Service) IsChannelMember(userID, channelID string) (bool, error) {
	var count int64
	err := s.Db.Postgresql.Model(&models.UserChannels{}).
		Where("user_id = ? AND channels_id = ?", userID, channelID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// loadRoomConfig returns the override row for a room, if any.
func (s *Service) loadRoomConfig(orgID string, ref models.OfficeRoomRef) (*models.OfficeRoomConfig, error) {
	var config models.OfficeRoomConfig
	err := s.Db.Postgresql.Where("organisation_id = ? AND room_kind = ? AND ref_id = ?", orgID, ref.Kind, ref.Ref).
		First(&config).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}
