package test_office

import (
	"testing"

	"github.com/hngprojects/telex_be/internal/models"
)

// The office columns on organisations and profiles are nullable, so the jsonb
// scanner has to survive a NULL value instead of failing the whole query.
func TestNullableJSONColumnsScan(t *testing.T) {
	var settings models.OfficeSettings
	if err := settings.Scan(nil); err != nil {
		t.Fatalf("scanning a null office_settings column failed: %v", err)
	}
	if settings.Enabled {
		t.Fatal("a null office_settings column should decode to the zero value")
	}

	var avatar models.OfficeAvatarConfig
	if err := avatar.Scan(nil); err != nil {
		t.Fatalf("scanning a null avatar_config column failed: %v", err)
	}
	if avatar.Tint != "" {
		t.Fatal("a null avatar_config column should decode to the zero value")
	}

	var payload models.OfficeLayoutPayload
	if err := payload.Scan(nil); err != nil {
		t.Fatalf("scanning a null layout column failed: %v", err)
	}
	if len(payload.Items) != 0 {
		t.Fatal("a null layout column should decode to an empty layout")
	}
}

func TestJSONColumnsScanDocuments(t *testing.T) {
	var settings models.OfficeSettings
	if err := settings.Scan([]byte(`{"enabled":true,"capacity":42}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !settings.Enabled || settings.Capacity != 42 {
		t.Fatalf("unexpected decode: %+v", settings)
	}

	var avatar models.OfficeAvatarConfig
	if err := avatar.Scan(`{"tint":"#8b5cf6","body":"b","outfit":"smart","accessory":"glasses"}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if avatar.Tint != "#8b5cf6" || avatar.Accessory != "glasses" {
		t.Fatalf("unexpected decode: %+v", avatar)
	}
}

func TestJSONColumnsRejectUnsupportedValues(t *testing.T) {
	var settings models.OfficeSettings
	if err := settings.Scan(42); err == nil {
		t.Fatal("expected an error for a value that is not a json document")
	}
}
