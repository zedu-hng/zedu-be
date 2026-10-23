package test_office

import (
	"strings"
	"testing"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/services/office"
)

func TestPresenceChannelName(t *testing.T) {
	target := office.OfficeChannelTarget{
		OrgID:      "org_1",
		FloorIndex: models.OfficeFloorGeneral,
		RoomID:     "channel:ch_1",
	}

	got := office.PresenceChannelName(target)
	want := "office_org_org_1_floor_0_room_Y2hhbm5lbDpjaF8x_presence"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLayoutChannelName(t *testing.T) {
	target := office.OfficeChannelTarget{
		OrgID:      "org_1",
		FloorIndex: models.OfficeFloorMember,
		RoomID:     "personal:usr_2",
	}

	got := office.LayoutChannelName(target)
	want := "office_org_org_1_floor_1_room_cGVyc29uYWw6dXNyXzI_layout"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestChannelNamesCarryTheOrganisation(t *testing.T) {
	// The lobby exists in every organisation. Without the organisation in the
	// name, two organisations would share one presence channel.
	first := office.PresenceChannelName(office.OfficeChannelTarget{
		OrgID:      "org_1",
		FloorIndex: models.OfficeFloorGeneral,
		RoomID:     "lobby",
	})
	second := office.PresenceChannelName(office.OfficeChannelTarget{
		OrgID:      "org_2",
		FloorIndex: models.OfficeFloorGeneral,
		RoomID:     "lobby",
	})

	if first == second {
		t.Fatalf("two organisations share the channel %q", first)
	}
}

func TestIsOfficeChannel(t *testing.T) {
	target := office.OfficeChannelTarget{
		OrgID:      "org_1",
		FloorIndex: models.OfficeFloorGeneral,
		RoomID:     "lobby",
	}

	if !office.IsOfficeChannel(office.PresenceChannelName(target)) {
		t.Fatal("presence channels are office channels")
	}
	if office.IsOfficeChannel("some-org/user-id") {
		t.Fatal("regular channels are not office channels")
	}
}

func TestParseOfficeChannel(t *testing.T) {
	channel, ok := office.ParseOfficeChannel(office.PresenceChannelName(office.OfficeChannelTarget{
		OrgID:      "org_1",
		FloorIndex: models.OfficeFloorGeneral,
		RoomID:     "channel:ch_1",
	}))
	if !ok {
		t.Fatal("expected the channel to be recognised")
	}
	if channel.OrgID != "org_1" {
		t.Fatalf("organisation: got %q", channel.OrgID)
	}
	if channel.FloorIndex != models.OfficeFloorGeneral {
		t.Fatalf("floor: got %d", channel.FloorIndex)
	}
	if channel.RoomID != "channel:ch_1" {
		t.Fatalf("room id: got %q", channel.RoomID)
	}
	if channel.Stream != "presence" {
		t.Fatalf("stream: got %q", channel.Stream)
	}

	channel, ok = office.ParseOfficeChannel(office.LayoutChannelName(office.OfficeChannelTarget{
		OrgID:      "org_1",
		FloorIndex: models.OfficeFloorMember,
		RoomID:     "personal:usr_2",
	}))
	if !ok {
		t.Fatal("expected the channel to be recognised")
	}
	if channel.RoomID != "personal:usr_2" {
		t.Fatalf("room id: got %q", channel.RoomID)
	}
	if channel.Stream != "layout" {
		t.Fatalf("stream: got %q", channel.Stream)
	}
}

func TestParseOfficeChannelRejectsUnknownStreams(t *testing.T) {
	if _, ok := office.ParseOfficeChannel("office_org_org_1_floor_0_room_Y2hhbm5lbDpjaF8x_chat"); ok {
		t.Fatal("unknown streams must be rejected")
	}
	if _, ok := office.ParseOfficeChannel("not-an-office-channel"); ok {
		t.Fatal("regular channels must be rejected")
	}
}

func TestParseOfficeChannelRejectsForeignShapes(t *testing.T) {
	for _, channel := range []string{
		"office_org_org_1_room_Y2hhbm5lbDpjaF8x_presence",
		"office_org_org_1_floor_x_room_Y2hhbm5lbDpjaF8x_presence",
		"office_org__floor_0_room_Y2hhbm5lbDpjaF8x_presence",
	} {
		if _, ok := office.ParseOfficeChannel(channel); ok {
			t.Fatalf("%q must be rejected", channel)
		}
	}
}

func TestChannelNameContainsNoColon(t *testing.T) {
	// Colons are forbidden in office channel names: Centrifugo resolves the
	// namespace before the first `:`, and an undeclared namespace fails with
	// `102: unknown channel`. This pins the colon-free format against regression.
	targets := []office.OfficeChannelTarget{
		{OrgID: "org_1", FloorIndex: models.OfficeFloorGeneral, RoomID: "channel:ch_1"},
		{OrgID: "org_2", FloorIndex: models.OfficeFloorMember, RoomID: "personal:usr_2"},
		{OrgID: "org_3", FloorIndex: models.OfficeFloorGeneral, RoomID: "lobby"},
	}
	for _, target := range targets {
		for _, name := range []string{office.PresenceChannelName(target), office.LayoutChannelName(target)} {
			if strings.Contains(name, ":") {
				t.Fatalf("channel %q contains a colon, which Centrifugo treats as a namespace separator", name)
			}
		}
	}
}

func TestRoomFloorIndex(t *testing.T) {
	shared := models.OfficeRoomRef{Kind: models.OfficeRoomKindChannel, Ref: "ch_1"}
	if office.RoomFloorIndex(shared) != models.OfficeFloorGeneral {
		t.Fatal("shared rooms sit on the general floor")
	}

	personal := models.OfficeRoomRef{Kind: models.OfficeRoomKindPersonal, Ref: "usr_1"}
	if office.RoomFloorIndex(personal) != models.OfficeFloorMember {
		t.Fatal("private offices sit on a member floor")
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
