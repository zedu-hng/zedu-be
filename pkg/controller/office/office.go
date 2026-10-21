package office

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/hngprojects/telex_be/external/request"
	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/middleware"
	"github.com/hngprojects/telex_be/pkg/repository/storage"
	service "github.com/hngprojects/telex_be/services/office"
	"github.com/hngprojects/telex_be/utility"
)

// Controller exposes the office API.
type Controller struct {
	Db        *storage.Database
	Validator *validator.Validate
	Logger    *utility.Logger
	ExtReq    request.ExternalRequest
}

// identity is the requester behind a call.
type identity struct {
	UserID string
	OrgID  string
}

// identityOf pulls the authenticated user and organisation out of the token
// claims set by the Authorize middleware.
func (base *Controller) identityOf(c *gin.Context) (identity, error) {
	userID, err := middleware.GetUserClaims(c, base.Db.Postgresql, "user_id")
	if err != nil {
		return identity{}, err
	}

	orgID, err := middleware.GetUserClaims(c, base.Db.Postgresql, "org_id")
	if err != nil {
		return identity{}, err
	}

	user, ok := userID.(string)
	if !ok || user == "" {
		return identity{}, errors.New("unable to read the user id from the token")
	}

	org, ok := orgID.(string)
	if !ok || org == "" {
		return identity{}, errors.New("unable to read the organisation id from the token")
	}

	return identity{UserID: user, OrgID: org}, nil
}

func (base *Controller) service() *service.Service {
	return service.New(base.Db, base.Logger)
}

// Bootstrap returns the building, its settings, the room directory and the
// published layouts the requester may see.
func (base *Controller) Bootstrap(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	directory, err := office.BuildDirectory(requester.OrgID, requester.UserID)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to build the office directory", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	org := models.Organisation{}
	organisation, err := org.GetOrgByID(base.Db.Postgresql, requester.OrgID)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to load the organisation", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	settings := organisation.OfficeSettings
	if !settings.Enabled {
		settings = models.OfficeSettings{Enabled: true}
	}

	layouts := map[string]models.OfficeLayoutResponse{}
	for _, room := range directory.Rooms {
		ref, err := service.ParseRoomID(room.ID)
		if err != nil {
			continue
		}

		layout, err := office.GetPublishedLayout(requester.OrgID, ref)
		if err != nil {
			rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to load the room layout", err, nil)
			c.JSON(http.StatusInternalServerError, rd)
			return
		}

		if layout == nil && ref.Kind == models.OfficeRoomKindPersonal && ref.Ref == requester.UserID {
			layout, err = office.GetPersonalLayout(requester.OrgID, requester.UserID)
			if err != nil {
				rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to load the room layout", err, nil)
				c.JSON(http.StatusInternalServerError, rd)
				return
			}
		}

		if layout != nil {
			layouts[room.ID] = service.ToResponse(layout)
		}
	}

	payload := models.OfficeBootstrapResponse{
		Building: models.OfficeBuildingResponse{
			ID:    requester.OrgID,
			OrgID: requester.OrgID,
			Name:  organisation.Name,
		},
		Settings:       settings,
		Rooms:          directory.Rooms,
		Layouts:        layouts,
		ChannelRoomMap: directory.ChannelRoomMap,
		DMOfficeMap:    directory.DMOfficeMap,
	}

	rd := utility.BuildSuccessResponse(http.StatusOK, "office bootstrap", payload)
	c.JSON(http.StatusOK, rd)
}

// GetRoom returns the metadata of a single room plus the access result.
func (base *Controller) GetRoom(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	ref, err := service.ParseRoomID(c.Param("roomId"))
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid room id", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	access, err := office.CanAccess(requester.OrgID, requester.UserID, ref)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to check room access", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}
	if !access.CanEnter {
		rd := utility.BuildErrorResponse(http.StatusForbidden, "error", "you cannot enter this room", nil, nil)
		c.JSON(http.StatusForbidden, rd)
		return
	}

	directory, err := office.BuildDirectory(requester.OrgID, requester.UserID)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to build the office directory", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	roomID := service.EncodeRoomID(ref.Kind, ref.Ref)
	for _, room := range directory.Rooms {
		if room.ID == roomID {
			rd := utility.BuildSuccessResponse(http.StatusOK, "room", map[string]any{"room": room, "access": access})
			c.JSON(http.StatusOK, rd)
			return
		}
	}

	rd := utility.BuildErrorResponse(http.StatusNotFound, "error", "room not found", nil, nil)
	c.JSON(http.StatusNotFound, rd)
}

// GetLayout returns the live layout of a room.
func (base *Controller) GetLayout(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	ref, err := service.ParseRoomID(c.Param("roomId"))
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid room id", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	access, err := office.CanAccess(requester.OrgID, requester.UserID, ref)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to check room access", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}
	if !access.CanEnter {
		rd := utility.BuildErrorResponse(http.StatusForbidden, "error", "you cannot view this room", nil, nil)
		c.JSON(http.StatusForbidden, rd)
		return
	}

	layout, err := office.GetPublishedLayout(requester.OrgID, ref)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to load the room layout", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	if layout == nil && ref.Kind == models.OfficeRoomKindPersonal && ref.Ref == requester.UserID {
		layout, err = office.GetPersonalLayout(requester.OrgID, requester.UserID)
		if err != nil {
			rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to load the room layout", err, nil)
			c.JSON(http.StatusInternalServerError, rd)
			return
		}
	}

	if layout == nil {
		rd := utility.BuildSuccessResponse(http.StatusOK, "room layout", models.OfficeLayoutResponse{
			RoomID:  service.EncodeRoomID(ref.Kind, ref.Ref),
			Version: 0,
			Status:  "",
			Grid:    models.OfficeGrid{Width: int(service.OfficeBuildingWidth), Height: int(service.OfficeBuildingHeight), Tile: service.OfficeTileSize},
			Items:   []models.OfficeFurnitureItem{},
		})
		c.JSON(http.StatusOK, rd)
		return
	}

	rd := utility.BuildSuccessResponse(http.StatusOK, "room layout", service.ToResponse(layout))
	c.JSON(http.StatusOK, rd)
}

// GetDraft returns the admin's working copy of a shared room layout.
func (base *Controller) GetDraft(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	ref, err := service.ParseRoomID(c.Param("roomId"))
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid room id", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	access, err := office.CanAccess(requester.OrgID, requester.UserID, ref)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to check room access", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}
	if !access.CanEditFurniture {
		rd := utility.BuildErrorResponse(http.StatusForbidden, "error", "only administrators can edit shared furniture", nil, nil)
		c.JSON(http.StatusForbidden, rd)
		return
	}

	draft, err := office.GetDraft(requester.OrgID, ref, requester.UserID)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to load the room draft", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	if draft == nil {
		rd := utility.BuildSuccessResponse(http.StatusOK, "room draft", models.OfficeLayoutResponse{
			RoomID: service.EncodeRoomID(ref.Kind, ref.Ref),
			Items:  []models.OfficeFurnitureItem{},
		})
		c.JSON(http.StatusOK, rd)
		return
	}

	rd := utility.BuildSuccessResponse(http.StatusOK, "room draft", service.ToResponse(draft))
	c.JSON(http.StatusOK, rd)
}

// SaveDraft stores the admin's working copy of a shared room layout.
func (base *Controller) SaveDraft(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	ref, err := service.ParseRoomID(c.Param("roomId"))
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid room id", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	access, err := office.CanAccess(requester.OrgID, requester.UserID, ref)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to check room access", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}
	if !access.CanEditFurniture {
		rd := utility.BuildErrorResponse(http.StatusForbidden, "error", "only administrators can edit shared furniture", nil, nil)
		c.JSON(http.StatusForbidden, rd)
		return
	}

	var request models.OfficeLayoutRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid request body", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	draft, err := office.SaveDraft(models.OfficeLayoutCommand{
		OrganisationID: requester.OrgID,
		Room:           ref,
		ActorID:        requester.UserID,
		Payload:        models.OfficeLayoutPayload{Grid: request.Grid, Items: request.Items},
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidLayout) {
			rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid layout", err, nil)
			c.JSON(http.StatusBadRequest, rd)
			return
		}
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to save the room draft", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	rd := utility.BuildSuccessResponse(http.StatusOK, "room draft saved", service.ToResponse(draft))
	c.JSON(http.StatusOK, rd)
}

// PublishLayout publishes the admin's draft as the live layout of a room.
func (base *Controller) PublishLayout(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	ref, err := service.ParseRoomID(c.Param("roomId"))
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid room id", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	access, err := office.CanAccess(requester.OrgID, requester.UserID, ref)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to check room access", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}
	if !access.CanPublish {
		rd := utility.BuildErrorResponse(http.StatusForbidden, "error", "only administrators can publish shared layouts", nil, nil)
		c.JSON(http.StatusForbidden, rd)
		return
	}

	var request models.OfficePublishLayoutRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid request body", err, nil)
			c.JSON(http.StatusBadRequest, rd)
			return
		}
	}

	layout, err := office.Publish(models.OfficeLayoutCommand{
		OrganisationID: requester.OrgID,
		Room:           ref,
		ActorID:        requester.UserID,
		Version:        request.Version,
	})
	if err != nil {
		if errors.Is(err, service.ErrVersionConflict) {
			rd := utility.BuildErrorResponse(http.StatusConflict, "error", "the layout was changed by someone else", err, nil)
			c.JSON(http.StatusConflict, rd)
			return
		}
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to publish the layout", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	office.PublishLayoutUpdate(service.EncodeRoomID(ref.Kind, ref.Ref), layout.Version, requester.UserID)

	rd := utility.BuildSuccessResponse(http.StatusOK, "layout published", service.ToResponse(layout))
	c.JSON(http.StatusOK, rd)
}

// ResetLayout restores a shared room to its template.
func (base *Controller) ResetLayout(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	ref, err := service.ParseRoomID(c.Param("roomId"))
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid room id", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	access, err := office.CanAccess(requester.OrgID, requester.UserID, ref)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to check room access", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}
	if !access.CanReset {
		rd := utility.BuildErrorResponse(http.StatusForbidden, "error", "only administrators can reset a room", nil, nil)
		c.JSON(http.StatusForbidden, rd)
		return
	}

	layout, err := office.Reset(requester.OrgID, ref, requester.UserID)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to reset the room", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	office.PublishLayoutUpdate(service.EncodeRoomID(ref.Kind, ref.Ref), layout.Version, requester.UserID)

	rd := utility.BuildSuccessResponse(http.StatusOK, "room reset", service.ToResponse(layout))
	c.JSON(http.StatusOK, rd)
}

// SavePersonalLayout autosaves the layout of the requester's own office.
func (base *Controller) SavePersonalLayout(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	ref, err := service.ParseRoomID(c.Param("roomId"))
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid room id", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	if ref.Kind != models.OfficeRoomKindPersonal {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "this route only saves personal offices", nil, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	if ref.Ref != requester.UserID {
		rd := utility.BuildErrorResponse(http.StatusForbidden, "error", "you can only edit your own office", nil, nil)
		c.JSON(http.StatusForbidden, rd)
		return
	}

	var request models.OfficeLayoutRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid request body", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	layout, err := office.SavePersonalLayout(requester.OrgID, requester.UserID, models.OfficeLayoutPayload{
		Grid:  request.Grid,
		Items: request.Items,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidLayout) {
			rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid layout", err, nil)
			c.JSON(http.StatusBadRequest, rd)
			return
		}
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to save your office", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	rd := utility.BuildSuccessResponse(http.StatusOK, "office saved", service.ToResponse(layout))
	c.JSON(http.StatusOK, rd)
}

// Occupants returns the people currently standing in a room.
func (base *Controller) Occupants(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	ref, err := service.ParseRoomID(c.Param("roomId"))
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid room id", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	access, err := office.CanAccess(requester.OrgID, requester.UserID, ref)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to check room access", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}
	if !access.CanEnter {
		rd := utility.BuildErrorResponse(http.StatusForbidden, "error", "you cannot view this room", nil, nil)
		c.JSON(http.StatusForbidden, rd)
		return
	}

	occupants, err := office.Occupants(requester.OrgID, ref)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to load the room occupants", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	rd := utility.BuildSuccessResponse(http.StatusOK, "room occupants", occupants)
	c.JSON(http.StatusOK, rd)
}

// EnterRoom records that the requester entered a room and returns the
// realtime channels they may subscribe to.
func (base *Controller) EnterRoom(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	ref, err := service.ParseRoomID(c.Param("roomId"))
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid room id", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	access, err := office.CanAccess(requester.OrgID, requester.UserID, ref)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to check room access", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}
	if !access.CanEnter {
		rd := utility.BuildErrorResponse(http.StatusForbidden, "error", "you cannot enter this room", nil, nil)
		c.JSON(http.StatusForbidden, rd)
		return
	}

	avatar, err := office.GetAvatar(requester.UserID)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to load your avatar", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	roomID := service.EncodeRoomID(ref.Kind, ref.Ref)
	presence := models.OfficeOccupantResponse{
		UserID:    requester.UserID,
		Avatar:    avatar,
		RoomID:    roomID,
		Status:    "online",
		UpdatedAt: time.Now().UTC(),
	}

	if err := office.JoinRoom(models.OfficePresenceCommand{
		OrganisationID: requester.OrgID,
		Room:           ref,
		UserID:         requester.UserID,
		Presence:       presence,
	}); err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to enter the room", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	payload := map[string]any{
		"roomId":   roomID,
		"presence": presence,
		"channels": []string{
			service.PresenceChannelName(roomID),
			service.LayoutChannelName(roomID),
		},
		"access": access,
	}

	rd := utility.BuildSuccessResponse(http.StatusOK, "room entered", payload)
	c.JSON(http.StatusOK, rd)
}

// LeaveRoom records that the requester left a room.
func (base *Controller) LeaveRoom(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	office := base.service()

	ref, err := service.ParseRoomID(c.Param("roomId"))
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid room id", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	if err := office.LeaveRoom(requester.OrgID, requester.UserID, ref); err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to leave the room", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	rd := utility.BuildSuccessResponse(http.StatusOK, "room left", map[string]any{
		"roomId": service.EncodeRoomID(ref.Kind, ref.Ref),
	})
	c.JSON(http.StatusOK, rd)
}

// GetAvatar returns the office avatar of the requester.
func (base *Controller) GetAvatar(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	avatar, err := base.service().GetAvatar(requester.UserID)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to load your avatar", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	rd := utility.BuildSuccessResponse(http.StatusOK, "avatar", avatar)
	c.JSON(http.StatusOK, rd)
}

// SaveAvatar stores the office avatar of the requester.
func (base *Controller) SaveAvatar(c *gin.Context) {
	requester, err := base.identityOf(c)
	if err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "unable to get user claims", err, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	var request models.OfficeAvatarRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "invalid request body", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	avatar := models.OfficeAvatarConfig{
		Tint:      request.Tint,
		Body:      request.Body,
		Outfit:    request.Outfit,
		Accessory: request.Accessory,
	}

	if err := base.service().SaveAvatar(requester.UserID, avatar); err != nil {
		rd := utility.BuildErrorResponse(http.StatusInternalServerError, "error", "failed to save your avatar", err, nil)
		c.JSON(http.StatusInternalServerError, rd)
		return
	}

	rd := utility.BuildSuccessResponse(http.StatusOK, "avatar saved", avatar)
	c.JSON(http.StatusOK, rd)
}
