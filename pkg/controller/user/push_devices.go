package user

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/middleware/common"
	pushdevices "github.com/hngprojects/telex_be/services/pushDevices"
	"github.com/hngprojects/telex_be/utility"
)

func (base *Controller) RegisterPushDevice(c *gin.Context) {
	var req models.RegisterPushDeviceRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "Failed to parse request body", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	if err := base.Validator.Struct(&req); err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnprocessableEntity, "error", "Validation failed", utility.ValidationResponse(err, base.Validator), nil)
		c.JSON(http.StatusUnprocessableEntity, rd)
		return
	}

	userID, ok := common.GetAllUserClaims(c)["user_id"].(string)
	if !ok || userID == "" {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "Unauthorized", nil, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	code, err := pushdevices.RegisterDevice(base.Db.Postgresql, userID, req)
	if err != nil {
		base.Logger.Error("failed to register push device: %v", err)
		rd := utility.BuildErrorResponse(code, "error", "Failed to register push device", nil, nil)
		c.JSON(code, rd)
		return
	}

	rd := utility.BuildSuccessResponse(code, "Push device registered successfully", gin.H{
		"platform": req.Platform,
	})
	c.JSON(code, rd)
}

func (base *Controller) UnregisterPushDevice(c *gin.Context) {
	var req models.UnregisterPushDeviceRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		rd := utility.BuildErrorResponse(http.StatusBadRequest, "error", "Failed to parse request body", err, nil)
		c.JSON(http.StatusBadRequest, rd)
		return
	}

	if err := base.Validator.Struct(&req); err != nil {
		rd := utility.BuildErrorResponse(http.StatusUnprocessableEntity, "error", "Validation failed", utility.ValidationResponse(err, base.Validator), nil)
		c.JSON(http.StatusUnprocessableEntity, rd)
		return
	}

	userID, ok := common.GetAllUserClaims(c)["user_id"].(string)
	if !ok || userID == "" {
		rd := utility.BuildErrorResponse(http.StatusUnauthorized, "error", "Unauthorized", nil, nil)
		c.JSON(http.StatusUnauthorized, rd)
		return
	}

	code, err := pushdevices.UnregisterDevice(base.Db.Postgresql, userID, req)
	if err != nil {
		base.Logger.Error("failed to unregister push device: %v", err)
		rd := utility.BuildErrorResponse(code, "error", "Failed to unregister push device", nil, nil)
		c.JSON(code, rd)
		return
	}

	rd := utility.BuildSuccessResponse(code, "Push device unregistered successfully", nil)
	c.JSON(code, rd)
}
