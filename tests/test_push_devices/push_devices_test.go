package test_push_devices

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/storage"
	tst "github.com/hngprojects/telex_be/tests"
	"github.com/hngprojects/telex_be/utility"
)

func sendJSON(t *testing.T, r *gin.Engine, method, token string, body any) *httptest.ResponseRecorder {
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(method, "/api/v1/users/push-devices", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func devicesForToken(t *testing.T, db *storage.Database, token string) []models.PushDevice {
	var devices []models.PushDevice
	require.NoError(t, db.Postgresql.Where("token = ?", token).Find(&devices).Error)
	return devices
}

func TestRegisterPushDevice(t *testing.T) {
	r, db := SetupPushDeviceTestRouter()
	t.Cleanup(func() { tst.Cleanup(db) })

	owner := CreateTestUser(t, db)
	ownerToken := GenerateTestToken(t, db, owner.ID)
	fcmToken := "fcm-" + utility.GenerateUUID()

	t.Run("registers an android device and clears the legacy OneSignal subscription", func(t *testing.T) {
		w := sendJSON(t, r, http.MethodPost, ownerToken, models.RegisterPushDeviceRequest{Token: fcmToken, Platform: "android"})
		assert.Equal(t, http.StatusOK, w.Code)

		devices := devicesForToken(t, db, fcmToken)
		require.Len(t, devices, 1)
		assert.Equal(t, owner.ID, devices[0].UserID)

		var u models.User
		require.NoError(t, db.Postgresql.Where("id = ?", owner.ID).First(&u).Error)
		assert.Empty(t, u.OneSignalSubscriptionID)
	})

	t.Run("registering the same token twice keeps one row", func(t *testing.T) {
		w := sendJSON(t, r, http.MethodPost, ownerToken, models.RegisterPushDeviceRequest{Token: fcmToken, Platform: "android"})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Len(t, devicesForToken(t, db, fcmToken), 1)
	})

	t.Run("a token moves to the account that signed in last on the device", func(t *testing.T) {
		other := CreateTestUser(t, db)
		otherToken := GenerateTestToken(t, db, other.ID)

		w := sendJSON(t, r, http.MethodPost, otherToken, models.RegisterPushDeviceRequest{Token: fcmToken, Platform: "android"})
		assert.Equal(t, http.StatusOK, w.Code)

		devices := devicesForToken(t, db, fcmToken)
		require.Len(t, devices, 1)
		assert.Equal(t, other.ID, devices[0].UserID)
	})

	t.Run("rejects an unsupported platform", func(t *testing.T) {
		w := sendJSON(t, r, http.MethodPost, ownerToken, models.RegisterPushDeviceRequest{Token: "fcm-ios", Platform: "ios"})
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("rejects a missing token", func(t *testing.T) {
		w := sendJSON(t, r, http.MethodPost, ownerToken, models.RegisterPushDeviceRequest{Platform: "android"})
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("requires authentication", func(t *testing.T) {
		w := sendJSON(t, r, http.MethodPost, "invalid", models.RegisterPushDeviceRequest{Token: fcmToken, Platform: "android"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestUnregisterPushDevice(t *testing.T) {
	r, db := SetupPushDeviceTestRouter()
	t.Cleanup(func() { tst.Cleanup(db) })

	owner := CreateTestUser(t, db)
	ownerToken := GenerateTestToken(t, db, owner.ID)
	other := CreateTestUser(t, db)
	otherToken := GenerateTestToken(t, db, other.ID)
	fcmToken := "fcm-" + utility.GenerateUUID()

	w := sendJSON(t, r, http.MethodPost, ownerToken, models.RegisterPushDeviceRequest{Token: fcmToken, Platform: "android"})
	require.Equal(t, http.StatusOK, w.Code)

	t.Run("another user cannot remove the device", func(t *testing.T) {
		w := sendJSON(t, r, http.MethodDelete, otherToken, models.UnregisterPushDeviceRequest{Token: fcmToken})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Len(t, devicesForToken(t, db, fcmToken), 1)
	})

	t.Run("the owner removes the device", func(t *testing.T) {
		w := sendJSON(t, r, http.MethodDelete, ownerToken, models.UnregisterPushDeviceRequest{Token: fcmToken})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, devicesForToken(t, db, fcmToken))
	})
}
