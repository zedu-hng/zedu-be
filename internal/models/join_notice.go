package models

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/hngprojects/telex_be/utility"
)

// NoticeLabelForProfile resolves the display label for a channel join/leave
// system notice. The configured, non-empty profiles.display_name wins;
// otherwise the stored username is used. No error is returned: an empty
// label is valid and callers decide how to handle it.
func NoticeLabelForProfile(profile Profile) string {
	if label := strings.TrimSpace(profile.DisplayName); label != "" {
		return label
	}
	return strings.TrimSpace(profile.UserName)
}

// noticeLabelForUserName resolves the final trusted account-level fallback
// labels from an already-loaded user row. It never touches the database and
// never uses client-supplied request data.
func noticeLabelForUserName(user User) string {
	if label := strings.TrimSpace(user.Name); label != "" {
		return label
	}
	return strings.TrimSpace(utility.SplitEmailString(user.Email))
}

// ErrJoinNoticeNoLabel is returned only when a user record exists but
// carries no usable trusted label at all (blank profile names, blank
// account name, and blank email prefix). Callers skip the participant
// rather than publish a blank mention.
var ErrJoinNoticeNoLabel = errors.New("join notice: no server-side label available")

// NewChannelJoinMention builds the system-notice participant for a single
// channel joiner, resolving the label server-side from trusted backend data.
// Precedence: organization-specific display_name, then profile user_name,
// then the trusted account username, then the email-prefix fallback.
// The mention UserID is always preserved. No client-controlled string is
// ever used as a label.
//
// GetUserByID returns the user row even when optional profile resolution
// fails, so a missing profile never suppresses an otherwise valid notice:
// the account-level fallback still applies. Only a missing user record
// (or a completely blank identity) prevents notice generation.
func NewChannelJoinMention(db *gorm.DB, userID, orgID string) (UserMention, error) {
	var user User
	details, err := user.GetUserByID(db, userID, orgID)
	if err != nil {
		return UserMention{UserID: userID}, err
	}
	if label := NoticeLabelForProfile(details.Profile); label != "" {
		return UserMention{UserID: userID, Username: label}, nil
	}
	if label := noticeLabelForUserName(details); label != "" {
		return UserMention{UserID: userID, Username: label}, nil
	}
	return UserMention{UserID: userID}, ErrJoinNoticeNoLabel
}

// BuildChannelJoinMentions resolves server-side labels for a batch of
// channel joiners, preserving their order and IDs. Entries that cannot be
// resolved to any label are skipped so no blank mention is ever stored.
func BuildChannelJoinMentions(db *gorm.DB, userIDs []string, orgID string) []UserMention {
	if len(userIDs) == 0 {
		return nil
	}
	mentions := make([]UserMention, 0, len(userIDs))
	seen := make(map[string]bool, len(userIDs))
	for _, userID := range userIDs {
		if userID == "" || seen[userID] {
			continue
		}
		seen[userID] = true
		mention, err := NewChannelJoinMention(db, userID, orgID)
		if err != nil || mention.Username == "" {
			continue
		}
		mentions = append(mentions, mention)
	}
	return mentions
}
