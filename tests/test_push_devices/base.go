package test_push_devices

import (
	"fmt"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt"

	"github.com/hngprojects/telex_be/external/request"
	"github.com/hngprojects/telex_be/internal/config"
	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/controller/user"
	"github.com/hngprojects/telex_be/pkg/middleware"
	"github.com/hngprojects/telex_be/pkg/repository/storage"
	tst "github.com/hngprojects/telex_be/tests"
	"github.com/hngprojects/telex_be/utility"
)

func SetupPushDeviceTestRouter() (*gin.Engine, *storage.Database) {
	gin.SetMode(gin.TestMode)

	logger := tst.Setup()
	db := storage.Connection()

	userController := &user.Controller{
		Db:        db,
		Validator: validator.New(),
		Logger:    logger,
		ExtReq:    request.ExternalRequest{Logger: logger, Test: true},
	}

	r := gin.Default()
	userUrl := r.Group("/api/v1", middleware.Authorize(db.Postgresql))
	{
		userUrl.POST("/users/push-devices", userController.RegisterPushDevice)
		userUrl.DELETE("/users/push-devices", userController.UnregisterPushDevice)
	}

	return r, db
}

func CreateTestUser(t *testing.T, db *storage.Database) models.User {
	uuid := utility.GenerateUUID()
	u := models.User{
		ID:                      uuid,
		Email:                   fmt.Sprintf("push%v@example.com", uuid),
		Name:                    fmt.Sprintf("pushuser%v", uuid),
		Password:                "password123",
		Role:                    int(models.RoleIdentity.User),
		OneSignalSubscriptionID: "legacy-onesignal-sub",
	}

	if err := db.Postgresql.Create(&u).Error; err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return u
}

func GenerateTestToken(t *testing.T, db *storage.Database, userID string) string {
	accessUUID := utility.GenerateUUID()
	claims := jwt.MapClaims{
		"user_id":     userID,
		"org_id":      "00000000-0000-0000-0000-000000000000",
		"access_uuid": accessUUID,
		"exp":         time.Now().Add(time.Hour * 24).Unix(),
		"authorised":  true,
	}

	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.GetConfig().Server.Secret))
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	accessToken := models.AccessToken{
		ID:               accessUUID,
		LoginAccessToken: tokenString,
		OwnerID:          userID,
		IsLive:           true,
	}
	if err := db.Postgresql.Create(&accessToken).Error; err != nil {
		t.Fatalf("Failed to create access token: %v", err)
	}

	return tokenString
}
