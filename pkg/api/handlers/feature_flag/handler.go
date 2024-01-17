package featureflaghandler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	api_errors "github.com/Roll-Play/togglelabs/pkg/api/error"
	featureflagmodel "github.com/Roll-Play/togglelabs/pkg/models/feature_flag"
	organizationmodel "github.com/Roll-Play/togglelabs/pkg/models/organization"
	timelinemodel "github.com/Roll-Play/togglelabs/pkg/models/timeline"
	api_utils "github.com/Roll-Play/togglelabs/pkg/utils/api_utils"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

type FeatureFlagHandler struct {
	db     *mongo.Database
	logger *zap.Logger
}

func New(db *mongo.Database, logger *zap.Logger) *FeatureFlagHandler {
	return &FeatureFlagHandler{
		db:     db,
		logger: logger,
	}
}

type PostFeatureFlagRequest struct {
	Name         string                    `json:"name" validate:"required"`
	Type         featureflagmodel.FlagType `json:"type" validate:"required,oneof=boolean json string number"`
	DefaultValue string                    `json:"default_value" validate:"required"`
	Rules        []featureflagmodel.Rule   `json:"rules" validate:"dive,required"`
	Environment  string                    `json:"environment" validate:"required"`
}

type PatchFeatureFlagRequest struct {
	DefaultValue string                  `json:"default_value"`
	Rules        []featureflagmodel.Rule `json:"rules" validate:"dive,required"`
}

type ListFeatureFlagResponse struct {
	Data     []featureflagmodel.FeatureFlagRecord `json:"data"`
	Page     int                                  `json:"page"`
	PageSize int                                  `json:"page_size"`
	Total    int                                  `json:"total"`
}

func (ffh *FeatureFlagHandler) ListFeatureFlags(c echo.Context) error {
	pageQuery := c.QueryParam("page")
	limitQuery := c.QueryParam("page_size")

	page, limit := api_utils.GetPaginationParams(pageQuery, limitQuery)

	userID, err := api_utils.GetUserFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationID, err := api_utils.GetOrganizationFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationModel := organizationmodel.New(ffh.db)
	organization, err := organizationModel.FindByID(context.Background(), organizationID)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	permission := api_utils.UserHasPermission(userID, organization, organizationmodel.ReadOnly)
	if !permission {
		ffh.logger.Debug("Client error",
			zap.String("cause", api_errors.ForbiddenError),
		)
		return api_errors.CustomError(
			c,
			http.StatusForbidden,
			api_errors.ForbiddenError,
		)
	}

	model := featureflagmodel.New(ffh.db)

	featureFlags, err := model.FindMany(context.Background(), organizationID, page, limit)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	return c.JSON(http.StatusOK, ListFeatureFlagResponse{
		Data:     featureFlags,
		Page:     page,
		PageSize: limit,
		Total:    len(featureFlags),
	})
}

func (ffh *FeatureFlagHandler) PostFeatureFlag(c echo.Context) error {
	userID, err := api_utils.GetUserFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationID, err := api_utils.GetOrganizationFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationModel := organizationmodel.New(ffh.db)
	organizationRecord, err := organizationModel.FindByID(context.Background(), organizationID)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	permission := api_utils.UserHasPermission(userID, organizationRecord, organizationmodel.Collaborator)
	if !permission {
		ffh.logger.Debug("Client error",
			zap.String("cause", api_errors.ForbiddenError),
		)
		return api_errors.CustomError(
			c,
			http.StatusForbidden,
			api_errors.ForbiddenError,
		)
	}

	request := new(PostFeatureFlagRequest)
	if err := c.Bind(request); err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	validate := validator.New()

	if err := validate.Struct(request); err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	featureFlagModel := featureflagmodel.New(ffh.db)
	featureFlagRecord := featureflagmodel.NewFeatureFlagRecord(
		request.Name,
		request.DefaultValue,
		request.Type,
		request.Rules,
		organizationID,
		userID,
		request.Environment,
	)

	featureFlagID, err := featureFlagModel.InsertOne(context.Background(), featureFlagRecord)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	timelineModel := timelinemodel.New(ffh.db)
	_, err = timelineModel.InsertOne(context.Background(),
		&timelinemodel.TimelineRecord{
			FeatureFlagID: featureFlagID,
			Entries:       []timelinemodel.TimelineEntry{},
		})
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	timelineEntry := timelinemodel.NewTimelineEntry(
		userID,
		timelinemodel.Created,
	)
	err = timelineModel.UpdateOne(context.Background(), featureFlagID, timelineEntry)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	return c.JSON(http.StatusCreated, featureFlagRecord)
}

func (ffh *FeatureFlagHandler) PatchFeatureFlag(c echo.Context) error {
	userID, err := api_utils.GetUserFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationID, err := api_utils.GetOrganizationFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationModel := organizationmodel.New(ffh.db)
	organizationRecord, err := organizationModel.FindByID(context.Background(), organizationID)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	permission := api_utils.UserHasPermission(userID, organizationRecord, organizationmodel.Collaborator)
	if !permission {
		ffh.logger.Debug("Client error",
			zap.String("cause", api_errors.ForbiddenError),
		)
		return api_errors.CustomError(
			c,
			http.StatusForbidden,
			api_errors.ForbiddenError,
		)
	}

	featureFlagID, err := primitive.ObjectIDFromHex(c.Param("featureFlagID"))
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	request := new(PatchFeatureFlagRequest)
	if err := c.Bind(request); err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	featureFlagModel := featureflagmodel.New(ffh.db)

	revision := featureflagmodel.NewRevisionRecord(
		request.DefaultValue,
		request.Rules,
		userID,
	)
	_, err = featureFlagModel.UpdateOne(
		context.Background(),
		bson.D{{Key: "_id", Value: featureFlagID}},
		bson.D{{Key: "$push", Value: bson.M{"revisions": revision}}},
	)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	timelineModel := timelinemodel.New(ffh.db)
	timelineEntry := timelinemodel.NewTimelineEntry(userID, timelinemodel.RevisionCreated)
	err = timelineModel.UpdateOne(context.Background(), featureFlagID, timelineEntry)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	return c.JSON(http.StatusOK, revision)
}

func (ffh *FeatureFlagHandler) ApproveRevision(c echo.Context) error {
	userID, err := api_utils.GetUserFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationID, err := api_utils.GetOrganizationFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationModel := organizationmodel.New(ffh.db)
	organizationRecord, err := organizationModel.FindByID(context.Background(), organizationID)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	permission := api_utils.UserHasPermission(userID, organizationRecord, organizationmodel.Collaborator)
	if !permission {
		ffh.logger.Debug("Server error",
			zap.String("cause", api_errors.UnauthorizedError),
		)
		return api_errors.CustomError(
			c,
			http.StatusUnauthorized,
			api_errors.UnauthorizedError,
		)
	}

	featureFlagID, err := primitive.ObjectIDFromHex(c.Param("featureFlagID"))
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	revisionID, err := primitive.ObjectIDFromHex(c.Param("revisionID"))
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	model := featureflagmodel.New(ffh.db)
	featureFlagRecord, err := model.FindByID(context.Background(), featureFlagID)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	var lastRevisionID primitive.ObjectID
	for index, revision := range featureFlagRecord.Revisions {
		if revision.Status == featureflagmodel.Live {
			featureFlagRecord.Revisions[index].Status = featureflagmodel.Archived
			lastRevisionID = revision.ID
		}
		if revision.ID == revisionID && revision.Status == featureflagmodel.Draft {
			featureFlagRecord.Revisions[index].Status = featureflagmodel.Live
			featureFlagRecord.Revisions[index].LastRevisionID = lastRevisionID
		}
	}
	featureFlagRecord.Version++

	filters := bson.D{{Key: "_id", Value: featureFlagID}}
	newValues := bson.D{
		{
			Key: "$set", Value: bson.D{
				{Key: "version", Value: featureFlagRecord.Version},
				{Key: "revisions", Value: featureFlagRecord.Revisions},
			},
		},
	}
	_, err = model.UpdateOne(context.Background(), filters, newValues)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	timelineModel := timelinemodel.New(ffh.db)
	timelineEntry := timelinemodel.NewTimelineEntry(userID, timelinemodel.RevisionApproved)
	err = timelineModel.UpdateOne(context.Background(), featureFlagID, timelineEntry)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	return c.JSON(http.StatusOK, featureFlagRecord)
}

func (ffh *FeatureFlagHandler) RollbackFeatureFlagVersion(c echo.Context) error {
	userID, err := api_utils.GetUserFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationID, err := api_utils.GetOrganizationFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationModel := organizationmodel.New(ffh.db)
	organizationRecord, err := organizationModel.FindByID(context.Background(), organizationID)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	permission := api_utils.UserHasPermission(userID, organizationRecord, organizationmodel.Collaborator)
	if !permission {
		ffh.logger.Debug("Server error",
			zap.String("cause", api_errors.ForbiddenError),
		)
		return api_errors.CustomError(
			c,
			http.StatusForbidden,
			api_errors.ForbiddenError,
		)
	}

	featureFlagID, err := primitive.ObjectIDFromHex(c.Param("featureFlagID"))
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	model := featureflagmodel.New(ffh.db)
	featureFlagRecord, err := model.FindByID(context.Background(), featureFlagID)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	var newRevisionID primitive.ObjectID
	for index, revision := range featureFlagRecord.Revisions {
		if revision.Status == featureflagmodel.Live {
			featureFlagRecord.Revisions[index].Status = featureflagmodel.Draft
			newRevisionID = revision.LastRevisionID
			featureFlagRecord.Revisions[index].LastRevisionID = primitive.NilObjectID
		}
	}
	for index, revision := range featureFlagRecord.Revisions {
		if revision.ID == newRevisionID && revision.Status == featureflagmodel.Archived {
			featureFlagRecord.Revisions[index].Status = featureflagmodel.Live
		}
	}
	featureFlagRecord.Version--

	filters := bson.D{{Key: "_id", Value: featureFlagID}}
	newValues := bson.D{
		{
			Key: "$set", Value: bson.D{
				{Key: "version", Value: featureFlagRecord.Version},
				{Key: "revisions", Value: featureFlagRecord.Revisions},
			},
		},
	}
	_, err = model.UpdateOne(context.Background(), filters, newValues)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}
	timelineModel := timelinemodel.New(ffh.db)
	timelineEntry := timelinemodel.NewTimelineEntry(userID, timelinemodel.FeatureFlagRollback)
	err = timelineModel.UpdateOne(context.Background(), featureFlagID, timelineEntry)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	return c.JSON(http.StatusOK, featureFlagRecord)
}

func (ffh *FeatureFlagHandler) DeleteFeatureFlag(c echo.Context) error {
	userID, err := api_utils.GetUserFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationID, err := api_utils.GetOrganizationFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationModel := organizationmodel.New(ffh.db)
	organizationRecord, err := organizationModel.FindByID(context.Background(), organizationID)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	permission := api_utils.UserHasPermission(userID, organizationRecord, organizationmodel.Collaborator)
	if !permission {
		ffh.logger.Debug("Server error",
			zap.String("cause", api_errors.ForbiddenError),
		)
		return api_errors.CustomError(
			c,
			http.StatusForbidden,
			api_errors.ForbiddenError,
		)
	}

	featureFlagID, err := primitive.ObjectIDFromHex(c.Param("featureFlagID"))
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	model := featureflagmodel.New(ffh.db)

	objectID, err := model.UpdateOne(
		context.Background(),
		bson.D{{Key: "_id", Value: featureFlagID}},
		bson.D{
			{Key: "$set", Value: bson.D{
				{
					Key:   "deleted_at",
					Value: primitive.NewDateTimeFromTime(time.Now().UTC()),
				},
			}},
		},
	)

	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()))
		return api_errors.CustomError(
			c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	timelineModel := timelinemodel.New(ffh.db)
	timelineEntry := timelinemodel.NewTimelineEntry(userID, timelinemodel.FeatureFlagDeleted)
	err = timelineModel.UpdateOne(context.Background(), featureFlagID, timelineEntry)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	ffh.logger.Info("Soft deleted feature flag",
		zap.String("_id", objectID.Hex()))
	return c.JSON(http.StatusNoContent, nil)
}

func (ffh *FeatureFlagHandler) ToggleFeatureFlag(c echo.Context) error {
	userID, err := api_utils.GetUserFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationID, err := api_utils.GetOrganizationFromContext(c)
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	organizationModel := organizationmodel.New(ffh.db)
	organizationRecord, err := organizationModel.FindByID(context.Background(), organizationID)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}
	permission := api_utils.UserHasPermission(userID, organizationRecord, organizationmodel.Collaborator)
	if !permission {
		ffh.logger.Debug("Server error",
			zap.String("cause", api_errors.ForbiddenError),
		)
		return api_errors.CustomError(
			c,
			http.StatusForbidden,
			api_errors.ForbiddenError,
		)
	}

	featureFlagID, err := primitive.ObjectIDFromHex(c.Param("featureFlagID"))
	if err != nil {
		ffh.logger.Debug("Client error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusBadRequest,
			api_errors.BadRequestError,
		)
	}

	model := featureflagmodel.New(ffh.db)
	featureFlagRecord, err := model.FindByID(context.Background(), featureFlagID)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(
			c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	environmentName := c.QueryParams().Get("env")
	for index, environment := range featureFlagRecord.Environments {
		if environment.Name == environmentName {
			featureFlagRecord.Environments[index].IsEnabled = !(featureFlagRecord.Environments[index].IsEnabled)
		}
	}

	filters := bson.D{{Key: "_id", Value: featureFlagID}}
	newValues := bson.D{
		{
			Key: "$set", Value: bson.D{
				{Key: "environments", Value: featureFlagRecord.Environments},
			},
		},
	}
	_, err = model.UpdateOne(context.Background(), filters, newValues)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	timelineModel := timelinemodel.New(ffh.db)
	timelineEntry := timelinemodel.NewTimelineEntry(userID, fmt.Sprintf(timelinemodel.FeatureFlagToggle, environmentName))
	err = timelineModel.UpdateOne(context.Background(), featureFlagID, timelineEntry)
	if err != nil {
		ffh.logger.Debug("Server error",
			zap.String("cause", err.Error()),
		)
		return api_errors.CustomError(c,
			http.StatusInternalServerError,
			api_errors.InternalServerError,
		)
	}

	return c.JSON(http.StatusOK, featureFlagRecord)
}