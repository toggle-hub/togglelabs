package organizationhandler

import (
	"context"
	"errors"
	"net/http"

	api_errors "github.com/Roll-Play/togglelabs/pkg/api/error"
	organizationmodel "github.com/Roll-Play/togglelabs/pkg/models/organization"
	usermodel "github.com/Roll-Play/togglelabs/pkg/models/user"
	api_utils "github.com/Roll-Play/togglelabs/pkg/utils/api_utils"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

type OrganizationHandler struct {
	db     *mongo.Database
	logger *zap.Logger
}

type OrganizationPostRequest struct {
	Name string `json:"name" validate:"required"`
}

func (oh *OrganizationHandler) PostOrganization(c echo.Context) error {
	request := new(OrganizationPostRequest)
	if err := c.Bind(request); err != nil {
		oh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	validate := validator.New()

	if err := validate.Struct(request); err != nil {
		oh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	userID, err := api_utils.GetUserFromContext(c)
	if err != nil {
		// Should never happen but better safe than sorry
		if errors.Is(err, api_utils.ErrNotAuthenticated) {
			oh.logger.Debug("Client error",
				zap.String("cause", err.Error()),
			)
			return api_errors.CustomError(
				c,
				http.StatusUnauthorized,
				api_errors.UnauthorizedError,
			)
		}

		oh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	userModel := usermodel.New(oh.db)
	user, err := userModel.FindByID(context.Background(), userID)
	if err != nil {
		oh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	user.Password = ""

	organization := organizationmodel.NewOrganizationRecord(request.Name, []organizationmodel.OrganizationMember{{
		User:            *user,
		PermissionLevel: organizationmodel.Admin,
	}})

	model := organizationmodel.New(oh.db)

	_, err = model.InsertOne(context.Background(), organization)

	if err != nil {
		oh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	return c.JSON(http.StatusCreated, organization)
}

func New(db *mongo.Database, logger *zap.Logger) *OrganizationHandler {
	return &OrganizationHandler{
		db:     db,
		logger: logger,
	}
}