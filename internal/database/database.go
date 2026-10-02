package database

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"moul.io/zapgorm2"

	"github.com/bigredeye/notmanytask/internal/models"
)

type DataBase struct {
	*gorm.DB
}

type DuplicateKey struct {
	nested error
}

func (e *DuplicateKey) Error() string {
	return e.nested.Error()
}

func (e *DuplicateKey) Unwrap() error {
	return e.nested
}

func IsDuplicateKey(err error) bool {
	duplicateKey := &DuplicateKey{}
	return errors.As(err, &duplicateKey)
}

// gorm sucks huge balls:(
// https://github.com/go-gorm/gorm/issues/4037
func isUnqiueViolation(err error) bool {
	perr, ok := err.(*pgconn.PgError)
	if ok {
		return perr.Code == "23505"
	}
	return false
}

func OpenDataBase(logger *zap.Logger, dsn string) (*DataBase, error) {
	zapLogger := zapgorm2.New(logger.Named("gorm"))
	zapLogger.SetAsDefault()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: zapLogger,
	})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&models.User{}, &models.Pipeline{}, &models.Session{}, &models.Flag{}, &models.OverriddenScore{}, &models.MergeRequest{}, &models.BenchmarkResult{}, &models.SubmissionBan{})
	if err != nil {
		return nil, err
	}

	return &DataBase{db}, nil
}

func (db *DataBase) AddUser(user *models.User) (*models.User, error) {
	var res models.User
	err := db.FirstOrCreate(&res, user).Error
	if err != nil {
		if isUnqiueViolation(err) {
			return nil, &DuplicateKey{err}
		}
		return nil, err
	}
	return &res, nil
}

func (db *DataBase) FindUserByID(id uint) (*models.User, error) {
	var user models.User
	err := db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (db *DataBase) FindUserByGitlabLogin(login string) (*models.User, error) {
	var user models.User
	err := db.First(&user, "gitlab_login = ?", login).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (db *DataBase) FindUserByGitlabID(id int) (*models.User, error) {
	var user models.User
	err := db.First(&user, "gitlab_id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (db *DataBase) FindUserByTelegramID(id int64) (*models.User, error) {
	var user models.User
	err := db.First(&user, "telegram_id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (db *DataBase) ListUsersWithoutRepos() ([]*models.User, error) {
	var users []*models.User
	err := db.Find(&users, "repository IS NULL AND gitlab_id IS NOT NULL AND gitlab_login IS NOT NULL").Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (db *DataBase) ListGroupUsers(groupName string) ([]*models.User, error) {
	var users []*models.User
	err := db.Find(&users, "repository IS NOT NULL AND group_name = ?", groupName).Order("created_at").Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (db *DataBase) SetUserGitlabAccount(uid uint, user *models.GitlabUser) error {
	res := db.Model(&models.User{}).
		Where("id = ? AND (gitlab_id IS NULL OR gitlab_login IS NULL)", uid).
		Updates(map[string]interface{}{
			"gitlab_id":    user.GitlabID,
			"gitlab_login": user.GitlabLogin,
		})

	if res.Error != nil {
		if isUnqiueViolation(res.Error) {
			return &DuplicateKey{res.Error}
		}
		return res.Error
	}

	if res.RowsAffected < 1 {
		return fmt.Errorf("unknown user %d", uid)
	}
	return nil
}

func (db *DataBase) SetUserRepository(user *models.User) error {
	res := db.Model(user).Update("repository", user.Repository)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected < 1 {
		return fmt.Errorf("unknown user %d", user.ID)
	}
	return nil
}

func (db *DataBase) SetUserTelegramID(user *models.User) error {
	res := db.Model(user).Update("telegram_id", user.TelegramID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected < 1 {
		return fmt.Errorf("unknown user %d", user.ID)
	}
	return nil
}

func (db *DataBase) SetUserGroupName(user *models.User) error {
	res := db.Model(user).Update("group_name", user.GroupName)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected < 1 {
		return fmt.Errorf("unknown user %d", user.ID)
	}
	return nil
}

func (db *DataBase) AddPipeline(pipeline *models.Pipeline) error {
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		// task: a merge request pipeline is stored under its ref until the
		// request is synced, see MergeRequestPipelineTask.
		DoUpdates: clause.AssignmentColumns([]string{"status", "task"}),
	}).Create(pipeline).Error
}

var mergeRequestRef = regexp.MustCompile(`^refs/merge-requests/(\d+)/(head|merge)$`)

// parseMergeRequestRef returns the iid of a merge request pipeline ref
// (refs/merge-requests/<iid>/head or /merge).
func parseMergeRequestRef(ref string) (int, bool) {
	match := mergeRequestRef.FindStringSubmatch(ref)
	if match == nil {
		return 0, false
	}
	iid, err := strconv.Atoi(match[1])
	return iid, err == nil
}

// MergeRequestPipelineTask resolves the task of a merge request pipeline
// through the synced merge request. isMergeRequest is false for any other
// ref; an empty task with isMergeRequest set means the request is not
// synced yet.
func (db *DataBase) MergeRequestPipelineTask(project, ref string) (task string, isMergeRequest bool) {
	iid, ok := parseMergeRequestRef(ref)
	if !ok {
		return "", false
	}
	var mergeRequest models.MergeRequest
	if err := db.First(&mergeRequest, "project = ? AND iid = ?", project, iid).Error; err != nil {
		return "", true
	}
	return mergeRequest.Task, true
}

func (db *DataBase) FindPipelineByID(id int) (*models.Pipeline, error) {
	var pipeline models.Pipeline
	if err := db.First(&pipeline, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &pipeline, nil
}

func (db *DataBase) BanSubmission(pipelineID int, reason, adminLogin string) error {
	ban := &models.SubmissionBan{PipelineID: pipelineID, Reason: reason, AdminLogin: adminLogin}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "pipeline_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"reason", "admin_login", "created_at"}),
	}).Create(ban).Error
}

// UnbanSubmission lifts the ban; found is false if there was none.
func (db *DataBase) UnbanSubmission(pipelineID int) (found bool, err error) {
	res := db.Delete(&models.SubmissionBan{}, "pipeline_id = ?", pipelineID)
	return res.RowsAffected > 0, res.Error
}

func (db *DataBase) ListSubmissionBans() (bans []models.SubmissionBan, err error) {
	bans = make([]models.SubmissionBan, 0)
	err = db.Find(&bans).Error
	return
}

func (db *DataBase) ListProjectPipelines(project string) (pipelines []models.Pipeline, err error) {
	pipelines = make([]models.Pipeline, 0)
	err = db.Find(&pipelines, "project = ?", project).Error
	if err != nil {
		pipelines = nil
	}
	return
}

func (db *DataBase) AddBenchmarkResult(result *models.BenchmarkResult) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "pipeline_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"gitlab_login", "task", "metric", "created_at"}),
	}).Create(result).Error
}

// GroupBenchmark is a benchmark result of a student from the requested
// group whose pipeline succeeded on the branch of the reported task and is
// not banned.
type GroupBenchmark struct {
	GitlabLogin string
	FirstName   string
	LastName    string
	Task        string
	PipelineID  int
	Metric      float64
	SubmittedAt time.Time
}

func (db *DataBase) ListGroupBenchmarks(group string) (results []GroupBenchmark, err error) {
	results = make([]GroupBenchmark, 0)
	err = db.Table("benchmark_results AS b").
		Select("b.gitlab_login, u.first_name, u.last_name, b.task, b.pipeline_id, b.metric, p.started_at AS submitted_at").
		Joins("JOIN pipelines AS p ON p.id = b.pipeline_id AND p.task = b.task AND p.status = ?", models.PipelineStatusSuccess).
		Joins("JOIN users AS u ON u.gitlab_login = b.gitlab_login AND u.deleted_at IS NULL").
		Joins("LEFT JOIN submission_bans AS sb ON sb.pipeline_id = p.id").
		Where("u.group_name = ? AND u.repository IS NOT NULL AND sb.pipeline_id IS NULL", group).
		Scan(&results).Error
	if err != nil {
		results = nil
	}
	return
}

func (db *DataBase) ListAllPipelines() (pipelines []models.Pipeline, err error) {
	pipelines = make([]models.Pipeline, 0)
	err = db.Find(&pipelines).Error
	if err != nil {
		pipelines = nil
	}
	return
}

func (db *DataBase) UpsertMergeRequest(mergeRequest *models.MergeRequest) error {
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"state",
			"user_notes_count",
			"merge_status",
			"sha",
			"merge_user_login",
			"has_unresolved_notes",
			"last_note_created_at",
			"last_pipeline_id",
			"last_pipeline_status",
			"last_pipeline_created_at",
			"extra_changes",
			"no_changes",
		}),
	}).Create(mergeRequest).Error
}

func (db *DataBase) ListProjectMergeRequests(project string) (mergeRequests []models.MergeRequest, err error) {
	mergeRequests = make([]models.MergeRequest, 0)
	err = db.Order("id").Find(&mergeRequests, "project = ?", project).Error
	if err != nil {
		mergeRequests = nil
	}
	return
}

func (db *DataBase) ListAllMergeRequests() (mergeRequests []models.MergeRequest, err error) {
	mergeRequests = make([]models.MergeRequest, 0)
	err = db.Order("id").Find(&mergeRequests).Error
	if err != nil {
		mergeRequests = nil
	}
	return
}

func (db *DataBase) ListProjectTaskMergeRequests(project, task string) (mergeRequests []models.MergeRequest, err error) {
	mergeRequests = make([]models.MergeRequest, 0)
	err = db.Order("id").Find(&mergeRequests, "project = ? AND task = ?", project, task).Error
	if err != nil {
		mergeRequests = nil
	}
	return
}

func (db *DataBase) CreateSession(user uint) (*models.Session, error) {
	session := &models.Session{
		Token:  uuid.Must(uuid.NewUUID()).String(),
		UserID: user,
	}
	res := db.Create(session)
	if res.Error != nil {
		return nil, res.Error
	}
	return session, nil
}

func (db *DataBase) FindSession(token string) (*models.Session, error) {
	var session models.Session
	res := db.DB.Where("token", token).Take(&session)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected < 1 {
		return nil, fmt.Errorf("unknown session")
	}
	return &session, nil
}

func (db *DataBase) FindUserBySession(token string) (*models.User, *models.Session, error) {
	session, err := db.FindSession(token)
	if err != nil {
		return nil, nil, err
	}
	user, err := db.FindUserByID(session.UserID)
	if err != nil {
		return nil, session, err
	}
	return user, session, nil
}

func (db *DataBase) CreateFlag(task string) (*models.Flag, error) {
	flag := &models.Flag{
		ID:        fmt.Sprintf("{FLAG-%s-%s}", task, uuid.New().String()),
		Task:      task,
		CreatedAt: time.Now(),
	}
	err := db.Create(flag).Error
	if err != nil {
		return nil, err
	}
	return flag, nil
}

func (db *DataBase) SubmitFlag(id, gitlabLogin string) error {
	result := db.Model(&models.Flag{}).Where("id = ? AND gitlab_login IS NULL", id).Update("gitlab_login", gitlabLogin)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("unknown flag")
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("unknown flag")
	}
	return nil
}

func (db *DataBase) ListUserFlags(gitlabLogin string) (flags []models.Flag, err error) {
	flags = make([]models.Flag, 0)
	err = db.Find(&flags, "gitlab_login = ?", gitlabLogin).Error
	if err != nil {
		flags = nil
	}
	return
}

func (db *DataBase) ListSubmittedFlags() (flags []models.Flag, err error) {
	flags = make([]models.Flag, 0)
	err = db.Find(&flags, "gitlab_login IS NOT NULL").Error
	if err != nil {
		flags = nil
	}
	return
}

func (db *DataBase) ListUserOverrides(login string) (overrides []models.OverriddenScore, err error) {
	overrides = make([]models.OverriddenScore, 0)
	err = db.Find(&overrides, "gitlab_login = ?", login).Error
	if err != nil {
		overrides = nil
	}
	return
}

func (db *DataBase) ListOverrides() (overrides []models.OverriddenScore, err error) {
	overrides = make([]models.OverriddenScore, 0)
	err = db.Find(&overrides).Error
	if err != nil {
		overrides = nil
	}
	return
}

func (db *DataBase) AddOverride(gitlabLogin, task string, score int, status models.PipelineStatus) error {
	overridenScore := &models.OverriddenScore{
		GitlabLogin: gitlabLogin,
		Task:        task,
		Score:       score,
		Status:      status,
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "gitlab_login"}, {Name: "task"}},
		DoUpdates: clause.AssignmentColumns([]string{"score", "status"}),
	}).Create(overridenScore).Error
}

func (db *DataBase) RemoveOverride(gitlabLogin, task string) error {
	return db.
		Where("gitlab_login = ? AND task = ?", gitlabLogin, task).
		Delete(models.OverriddenScore{}).
		Error
}
