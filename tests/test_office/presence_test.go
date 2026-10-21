package test_office

import (
	"testing"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/services/office"
)

func TestPresenceChannelName(t *testing.T) {
	got := office.PresenceChannelName("channel:ch_1")
	want := "office:room:channel%3Ach_1:presence"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLayoutChannelName(t *testing.T) {
	got := office.LayoutChannelName("personal:usr_2")
	want := "office:room:personal%3Ausr_2:layout"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestIsOfficeChannel(t *testing.T) {
	if !office.IsOfficeChannel(office.PresenceChannelName("lobby")) {
		t.Fatal("presence channels are office channels")
	}
	if office.IsOfficeChannel("some-org/user-id") {
		t.Fatal("regular channels are not office channels")
	}
}

func TestParseOfficeChannel(t *testing.T) {
	roomID, stream, ok := office.ParseOfficeChannel(office.PresenceChannelName("channel:ch_1"))
	if !ok {
		t.Fatal("expected the channel to be recognised")
	}
	if roomID != "channel:ch_1" {
		t.Fatalf("room id: got %q", roomID)
	}
	if stream != "presence" {
		t.Fatalf("stream: got %q", stream)
	}

	roomID, stream, ok = office.ParseOfficeChannel(office.LayoutChannelName("personal:usr_2"))
	if !ok {
		t.Fatal("expected the channel to be recognised")
	}
	if roomID != "personal:usr_2" {
		t.Fatalf("room id: got %q", roomID)
	}
	if stream != "layout" {
		t.Fatalf("stream: got %q", stream)
	}
}

func TestParseOfficeChannelRejectsUnknownStreams(t *testing.T) {
	if _, _, ok := office.ParseOfficeChannel("office:room:channel%3Ach_1:chat"); ok {
		t.Fatal("unknown streams must be rejected")
	}
	if _, _, ok := office.ParseOfficeChannel("not-an-office-channel"); ok {
		t.Fatal("regular channels must be rejected")
	}
}

func TestDefaultAvatar(t *testing.T) {
	avatar := office.DefaultAvatar()
	if avatar.Tint == "" || avatar.Body == "" || avatar.Outfit == "" || avatar.Accessory == "" {
		t.Fatalf("the default avatar must be complete, got %+v", avatar)
	}
}

func TestDefaultBoundsDoNotOverlap(t *testing.T) {
	// The derived defaults are what every client sees before an override row
	// exists, so the shared rooms must not sit on top of each other.
	rooms := []models.OfficeRoomResponse{
		{ID: "lobby", Bounds: models.OfficeBounds{X: 1, Y: 1, W: 8, H: 6}},
		{ID: "reception", Bounds: models.OfficeBounds{X: 9, Y: 1, W: 8, H: 6}},
		{ID: "lounge", Bounds: models.OfficeBounds{X: 1, Y: 7, W: 8, H: 6}},
		{ID: "meeting:1", Bounds: models.OfficeBounds{X: 18, Y: 1, W: 6, H: 6}},
		{ID: "meeting:2", Bounds: models.OfficeBounds{X: 25, Y: 1, W: 6, H: 6}},
	}

	for i := 0; i < len(rooms); i++ {
		for j := i + 1; j < len(rooms); j++ {
			if overlaps(rooms[i].Bounds, rooms[j].Bounds) {
				t.Fatalf("%s overlaps %s", rooms[i].ID, rooms[j].ID)
			}
		}
	}
}

func overlaps(a, b models.OfficeBounds) bool {
	return a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y
}
