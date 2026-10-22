package test_push_devices

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/pushNotifications/firebase"
	push_notifications "github.com/hngprojects/telex_be/services/pushNotifications"
	tst "github.com/hngprojects/telex_be/tests"
)

func TestStringifyData(t *testing.T) {
	data := firebase.StringifyData(map[string]interface{}{
		"notification_type": "dm",
		"thread_count":      3,
		"is_reply":          true,
		"participants":      []map[string]string{{"id": "u1"}},
		"missing":           nil,
	})

	assert.Equal(t, "dm", data["notification_type"])
	assert.Equal(t, "3", data["thread_count"])
	assert.Equal(t, "true", data["is_reply"])
	assert.Equal(t, `[{"id":"u1"}]`, data["participants"])
	assert.NotContains(t, data, "missing")
	assert.Empty(t, firebase.StringifyData(nil))
}

func TestDispatchWithoutConfiguredTransports(t *testing.T) {
	logger := tst.Setup()

	reached, err := push_notifications.Dispatch(logger, nil, push_notifications.PushDispatch{
		Req:     models.PushRequest{Title: "Hello", Message: "World"},
		UserIDs: []string{"user-1"},
	})

	assert.NoError(t, err)
	assert.Equal(t, 0, reached)
}
