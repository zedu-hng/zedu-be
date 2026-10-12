package test_admin

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/webhook"

	"github.com/hngprojects/telex_be/external/request"
	"github.com/hngprojects/telex_be/internal/config"
	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/controller/subscriptions"
	"github.com/hngprojects/telex_be/pkg/repository/storage"
	tst "github.com/hngprojects/telex_be/tests"
	"github.com/hngprojects/telex_be/utility"
)

const (
	testWebhookSecret  = "whsec_test_secret"
	testStartingCredit = 100.00
)

func signStripePayload(payload []byte, secret string) string {
	now := time.Now()
	signature := webhook.ComputeSignature(now, payload, secret)
	return fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(signature))
}

// useWebhookSecret must be called after tst.Setup(), which loads the configuration.
func useWebhookSecret(t *testing.T, secret string) {
	t.Helper()
	stripeConfig := &config.GetConfig().Stripe
	previous := stripeConfig.STRIPE_WEBHOOK_SECRET
	stripeConfig.STRIPE_WEBHOOK_SECRET = secret
	t.Cleanup(func() { stripeConfig.STRIPE_WEBHOOK_SECRET = previous })
}

func stubStripeSession(t *testing.T, checkoutSession *stripe.CheckoutSession) {
	t.Helper()
	previous := subscriptions.GetCheckoutSession
	subscriptions.GetCheckoutSession = func(id string) (*stripe.CheckoutSession, error) {
		return checkoutSession, nil
	}
	t.Cleanup(func() { subscriptions.GetCheckoutSession = previous })
}

func paidCreditSession(orgID, packageID, sessionID string) *stripe.CheckoutSession {
	return &stripe.CheckoutSession{
		ID:            sessionID,
		PaymentStatus: stripe.CheckoutSessionPaymentStatusPaid,
		Metadata: map[string]string{
			"flow":    "credit_topup",
			"org_id":  orgID,
			"plan_id": packageID,
		},
	}
}

func topupEventJSON(orgID, packageID, sessionID string) []byte {
	event := CreateTestStripeWebhookEvent(
		orgID,
		packageID,
		sessionID,
		"https://zedu.chat/client/settings/organisation/billing?session_id="+sessionID,
		"https://zedu.chat/client/settings/organisation/billing",
	)
	payload, _ := json.Marshal(event)
	return payload
}

// sendStripeWebhook posts the payload to the webhook. An empty signature sends no header.
func sendStripeWebhook(db *storage.Database, payload []byte, signature string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	webhookController := subscriptions.Controller{
		Db:        db,
		Validator: validator.New(),
		Logger:    utility.NewLogger(),
		ExtReq:    request.ExternalRequest{},
	}

	r := gin.New()
	r.POST("/api/v1/subscriptions/webhook", webhookController.HandleStripeWebhook)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/subscriptions/webhook", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("Stripe-Signature", signature)
	}

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// CreateOrganizationWithCredit seeds one "Top-up" transaction, so "not applied" means that one remains.
func assertTopupNotApplied(t *testing.T, db *storage.Database, orgID string) {
	t.Helper()

	var org models.Organisation
	assert.NoError(t, db.Postgresql.Where("id = ?", orgID).First(&org).Error)
	assert.Equal(t, testStartingCredit, org.CreditBalance)

	var topupCount int64
	db.Postgresql.Model(&models.CreditTransaction{}).
		Where("organisation_id = ? AND type = ?", orgID, "Top-up").
		Count(&topupCount)
	assert.Equal(t, int64(1), topupCount)
}

func cleanupTopupTest(db *storage.Database, packageID string, orgIDs ...string) {
	CleanupSpecificTestData(db.Postgresql, "", orgIDs)
	db.Postgresql.Exec("DELETE FROM credit_packages WHERE id = ?", packageID)
}

func TestStripeWebhook_MissingSignature(t *testing.T) {
	_ = tst.Setup()
	db := storage.Connection()

	orgID := CreateOrganizationWithCredit(t, db.Postgresql, testStartingCredit)
	packageID := CreateTestCreditPackage(t, db.Postgresql, 1000, 5.00)
	sessionID := fmt.Sprintf("cs_test_%s", utility.GenerateUUID())
	t.Cleanup(func() { cleanupTopupTest(db, packageID, orgID) })

	useWebhookSecret(t, testWebhookSecret)
	// Stripe would report this session as paid, so only the signature check can stop the request.
	stubStripeSession(t, paidCreditSession(orgID, packageID, sessionID))

	rr := sendStripeWebhook(db, topupEventJSON(orgID, packageID, sessionID), "")

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assertTopupNotApplied(t, db, orgID)
}

func TestStripeWebhook_InvalidSignature(t *testing.T) {
	_ = tst.Setup()
	db := storage.Connection()

	orgID := CreateOrganizationWithCredit(t, db.Postgresql, testStartingCredit)
	packageID := CreateTestCreditPackage(t, db.Postgresql, 1000, 5.00)
	sessionID := fmt.Sprintf("cs_test_%s", utility.GenerateUUID())
	t.Cleanup(func() { cleanupTopupTest(db, packageID, orgID) })

	useWebhookSecret(t, testWebhookSecret)
	stubStripeSession(t, paidCreditSession(orgID, packageID, sessionID))

	payload := topupEventJSON(orgID, packageID, sessionID)
	rr := sendStripeWebhook(db, payload, signStripePayload(payload, "whsec_wrong_secret"))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assertTopupNotApplied(t, db, orgID)
}

func TestStripeWebhook_SecretNotConfigured(t *testing.T) {
	_ = tst.Setup()
	db := storage.Connection()

	orgID := CreateOrganizationWithCredit(t, db.Postgresql, testStartingCredit)
	packageID := CreateTestCreditPackage(t, db.Postgresql, 1000, 5.00)
	sessionID := fmt.Sprintf("cs_test_%s", utility.GenerateUUID())
	t.Cleanup(func() { cleanupTopupTest(db, packageID, orgID) })

	useWebhookSecret(t, "")
	stubStripeSession(t, paidCreditSession(orgID, packageID, sessionID))

	// Signed with an empty secret: this must not be accepted just because the secret is empty.
	payload := topupEventJSON(orgID, packageID, sessionID)
	rr := sendStripeWebhook(db, payload, signStripePayload(payload, ""))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assertTopupNotApplied(t, db, orgID)
}

func TestStripeWebhook_CreditTopupNotPaid(t *testing.T) {
	_ = tst.Setup()
	db := storage.Connection()

	orgID := CreateOrganizationWithCredit(t, db.Postgresql, testStartingCredit)
	packageID := CreateTestCreditPackage(t, db.Postgresql, 1000, 5.00)
	sessionID := fmt.Sprintf("cs_test_%s", utility.GenerateUUID())
	t.Cleanup(func() { cleanupTopupTest(db, packageID, orgID) })

	useWebhookSecret(t, testWebhookSecret)
	unpaid := paidCreditSession(orgID, packageID, sessionID)
	unpaid.PaymentStatus = stripe.CheckoutSessionPaymentStatusUnpaid
	stubStripeSession(t, unpaid)

	// The signature is valid, but Stripe says the session was not paid.
	payload := topupEventJSON(orgID, packageID, sessionID)
	rr := sendStripeWebhook(db, payload, signStripePayload(payload, testWebhookSecret))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assertTopupNotApplied(t, db, orgID)
}

func TestStripeWebhook_CreditTopupUsesStripeSessionData(t *testing.T) {
	_ = tst.Setup()
	db := storage.Connection()

	orgInEvent := CreateOrganizationWithCredit(t, db.Postgresql, testStartingCredit)
	orgInStripe := CreateOrganizationWithCredit(t, db.Postgresql, testStartingCredit)
	packageID := CreateTestCreditPackage(t, db.Postgresql, 1000, 5.00)
	sessionID := fmt.Sprintf("cs_test_%s", utility.GenerateUUID())
	t.Cleanup(func() { cleanupTopupTest(db, packageID, orgInEvent, orgInStripe) })

	useWebhookSecret(t, testWebhookSecret)
	// Stripe says this session belongs to orgInStripe; the event body names a different organisation.
	stubStripeSession(t, paidCreditSession(orgInStripe, packageID, sessionID))

	payload := topupEventJSON(orgInEvent, packageID, sessionID)
	rr := sendStripeWebhook(db, payload, signStripePayload(payload, testWebhookSecret))

	assert.Equal(t, http.StatusOK, rr.Code)
	assertTopupNotApplied(t, db, orgInEvent)

	var credited models.Organisation
	assert.NoError(t, db.Postgresql.Where("id = ?", orgInStripe).First(&credited).Error)
	assert.Equal(t, float64(1100.00), credited.CreditBalance)
}
