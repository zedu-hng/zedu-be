package token

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/hngprojects/telex_be/internal/config"
	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/storage"
	"github.com/hngprojects/telex_be/services/office"
	"github.com/hngprojects/telex_be/utility"
)

func GetConnToken(userId string, db *gorm.DB) (gin.H, int, error) {

	userClaims := jwt.MapClaims{}

	userClaims["sub"] = userId
	userClaims["exp"] = time.Now().Unix() + int64(3600)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, userClaims)

	connToken, err := token.SignedString([]byte(config.Config.Centrifuge.Secret))
	if err != nil {
		return gin.H{}, http.StatusInternalServerError, err
	}

	res := gin.H{
		"token": connToken,
	}

	return res, http.StatusOK, nil

}

// SubTokenParams carries the identity behind a subscription token request.
type SubTokenParams struct {
	UserID  string
	OrgID   string
	Request models.ChannelSubTokenReq
}

func GetSubToken(params SubTokenParams, db *gorm.DB) (gin.H, int, error) {
	var (
		channelName = params.Request.Channel
		org         = models.Organisation{}
		orgId       = params.OrgID
		cUserId     string
		userId      = params.UserID
	)

	parts := strings.Split(params.Request.Channel, "/")
	if len(parts) == 2 {
		orgId = parts[0]
		cUserId = parts[1]

		if _, err := uuid.Parse(orgId); err != nil {
			return gin.H{}, http.StatusBadRequest, errors.New("invalid channel id format")
		}

		exist, _ := org.CheckUserIsMemberOfOrg(userId, orgId, db)

		if !exist || cUserId != userId {
			return gin.H{}, http.StatusBadRequest, errors.New("invalid channel subscription")
		}

	} else if office.IsOfficeChannel(channelName) {
		channel, ok := office.ParseOfficeChannel(channelName)
		if !ok {
			return gin.H{}, http.StatusBadRequest, errors.New("invalid office channel")
		}

		// The organisation is part of the channel name. A token is only
		// signed for the caller's own organisation, so one organisation can
		// never subscribe to the rooms of another.
		if channel.OrgID != orgId {
			return gin.H{}, http.StatusBadRequest, errors.New("invalid channel subscription")
		}

		ref, err := office.ParseRoomID(channel.RoomID)
		if err != nil {
			return gin.H{}, http.StatusBadRequest, err
		}

		officeService := office.New(&storage.Database{Postgresql: db}, nil)
		access, err := officeService.CanAccess(orgId, userId, ref)
		if err != nil {
			return gin.H{}, http.StatusBadRequest, err
		}
		if !access.CanEnter {
			return gin.H{}, http.StatusBadRequest, errors.New("invalid channel subscription")
		}

	} else {
		if !utility.IsValidBuzzCodeOrUUID(channelName) {
			return gin.H{}, http.StatusBadRequest, errors.New("invalid channel id supplied")
		}

	}

	userClaims := jwt.MapClaims{}
	userClaims["sub"] = userId
	userClaims["channel"] = channelName
	userClaims["exp"] = time.Now().Unix() + int64(3600)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, userClaims)

	subToken, err := token.SignedString([]byte(config.Config.Centrifuge.Secret))
	if err != nil {
		return gin.H{}, http.StatusInternalServerError, err
	}

	res := gin.H{
		"token": subToken,
	}

	return res, http.StatusOK, nil
}
