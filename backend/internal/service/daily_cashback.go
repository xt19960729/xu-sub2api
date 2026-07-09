package service

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	dailyCashbackLeaderLockKey = "daily_cashback:run:yesterday"
	dailyCashbackLeaderLockTTL = 30 * time.Minute
)

var ErrDailyCashbackRuleNotFound = infraerrors.NotFound("DAILY_CASHBACK_RULE_NOT_FOUND", "daily cashback rule not found")

type DailyCashbackRule struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Enabled     bool      `json:"enabled"`
	MinAmount   float64   `json:"min_amount"`
	MaxAmount   *float64  `json:"max_amount,omitempty"`
	RatePercent float64   `json:"rate_percent"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DailyCashbackRecord struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	UserEmail      string    `json:"user_email"`
	Username       string    `json:"username"`
	RuleID         *int64    `json:"rule_id,omitempty"`
	RuleName       string    `json:"rule_name,omitempty"`
	BusinessDate   string    `json:"business_date"`
	SpendAmount    float64   `json:"spend_amount"`
	RatePercent    float64   `json:"rate_percent"`
	CashbackAmount float64   `json:"cashback_amount"`
	BalanceAfter   *float64  `json:"balance_after,omitempty"`
	Status         string    `json:"status"`
	AppliedAt      time.Time `json:"applied_at"`
}

type DailyCashbackSpend struct {
	UserID int64
	Amount float64
}

type DailyCashbackRecordFilter struct {
	Search       string
	BusinessDate string
	Page         int
	PageSize     int
}

type DailyCashbackRunResult struct {
	BusinessDate  string  `json:"business_date"`
	MatchedUsers  int     `json:"matched_users"`
	AppliedUsers  int     `json:"applied_users"`
	SkippedUsers  int     `json:"skipped_users"`
	TotalSpend    float64 `json:"total_spend"`
	TotalCashback float64 `json:"total_cashback"`
}

type DailyCashbackRepository interface {
	ListRules(ctx context.Context) ([]DailyCashbackRule, error)
	CreateRule(ctx context.Context, rule DailyCashbackRule) (*DailyCashbackRule, error)
	UpdateRule(ctx context.Context, rule DailyCashbackRule) (*DailyCashbackRule, error)
	DeleteRule(ctx context.Context, id int64) error
	ListDailySpends(ctx context.Context, startTime, endTime time.Time) ([]DailyCashbackSpend, error)
	InsertRecordAndCredit(ctx context.Context, record DailyCashbackRecord) (bool, *DailyCashbackRecord, error)
	ListRecords(ctx context.Context, filter DailyCashbackRecordFilter) ([]DailyCashbackRecord, int64, error)
}

type DailyCashbackService struct {
	repo                 DailyCashbackRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCacheService  *BillingCacheService
	lockCache            LeaderLockCache
	db                   *sql.DB

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
}

func NewDailyCashbackService(repo DailyCashbackRepository, authCacheInvalidator APIKeyAuthCacheInvalidator, billingCacheService *BillingCacheService) *DailyCashbackService {
	return &DailyCashbackService{
		repo:                 repo,
		authCacheInvalidator: authCacheInvalidator,
		billingCacheService:  billingCacheService,
		stopCh:               make(chan struct{}),
	}
}

func (s *DailyCashbackService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func (s *DailyCashbackService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.startOnce.Do(func() {
		go s.runLoop()
		logger.LegacyPrintf("service.daily_cashback", "[DailyCashback] scheduler started")
	})
}

func (s *DailyCashbackService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *DailyCashbackService) runLoop() {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-timer.C:
			s.runYesterdayWithLock()
			timer.Reset(time.Hour)
		}
	}
}

func (s *DailyCashbackService) runYesterdayWithLock() {
	ctx, cancel := context.WithTimeout(context.Background(), dailyCashbackLeaderLockTTL)
	defer cancel()

	owner := timezone.Now().Format(time.RFC3339Nano)
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, dailyCashbackLeaderLockKey, owner, dailyCashbackLeaderLockTTL)
	if !ok {
		return
	}
	defer release()

	date := timezone.Today().AddDate(0, 0, -1).Format("2006-01-02")
	result, err := s.RunForDate(ctx, date)
	if err != nil {
		logger.LegacyPrintf("service.daily_cashback", "[DailyCashback] scheduled run failed date=%s err=%v", date, err)
		return
	}
	if result.AppliedUsers > 0 {
		logger.LegacyPrintf("service.daily_cashback", "[DailyCashback] scheduled run done date=%s applied=%d cashback=%.8f", date, result.AppliedUsers, result.TotalCashback)
	}
}

func (s *DailyCashbackService) ListRules(ctx context.Context) ([]DailyCashbackRule, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "daily cashback service unavailable")
	}
	return s.repo.ListRules(ctx)
}

func (s *DailyCashbackService) CreateRule(ctx context.Context, rule DailyCashbackRule) (*DailyCashbackRule, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "daily cashback service unavailable")
	}
	rule = normalizeDailyCashbackRule(rule)
	if err := validateDailyCashbackRule(rule); err != nil {
		return nil, err
	}
	return s.repo.CreateRule(ctx, rule)
}

func (s *DailyCashbackService) UpdateRule(ctx context.Context, rule DailyCashbackRule) (*DailyCashbackRule, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "daily cashback service unavailable")
	}
	if rule.ID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE", "invalid cashback rule")
	}
	rule = normalizeDailyCashbackRule(rule)
	if err := validateDailyCashbackRule(rule); err != nil {
		return nil, err
	}
	return s.repo.UpdateRule(ctx, rule)
}

func (s *DailyCashbackService) DeleteRule(ctx context.Context, id int64) error {
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "daily cashback service unavailable")
	}
	if id <= 0 {
		return infraerrors.BadRequest("INVALID_RULE", "invalid cashback rule")
	}
	return s.repo.DeleteRule(ctx, id)
}

func (s *DailyCashbackService) ListRecords(ctx context.Context, filter DailyCashbackRecordFilter) ([]DailyCashbackRecord, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "daily cashback service unavailable")
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	filter.Search = strings.TrimSpace(filter.Search)
	filter.BusinessDate = strings.TrimSpace(filter.BusinessDate)
	return s.repo.ListRecords(ctx, filter)
}

func (s *DailyCashbackService) RunForDate(ctx context.Context, businessDate string) (*DailyCashbackRunResult, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "daily cashback service unavailable")
	}
	day, err := parseBusinessDate(businessDate)
	if err != nil {
		return nil, err
	}
	rules, err := s.repo.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	enabled := make([]DailyCashbackRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			enabled = append(enabled, rule)
		}
	}
	result := &DailyCashbackRunResult{BusinessDate: day.Format("2006-01-02")}
	if len(enabled) == 0 {
		return result, nil
	}

	start := timezone.StartOfDay(day)
	end := start.AddDate(0, 0, 1)
	spends, err := s.repo.ListDailySpends(ctx, start, end)
	if err != nil {
		return nil, err
	}

	for _, spend := range spends {
		if spend.UserID <= 0 || spend.Amount <= 0 {
			continue
		}
		rule := matchDailyCashbackRule(spend.Amount, enabled)
		if rule == nil {
			continue
		}
		cashback := roundTo(spend.Amount*(rule.RatePercent/100), 8)
		if cashback <= 0 {
			continue
		}

		result.MatchedUsers++
		result.TotalSpend = roundTo(result.TotalSpend+spend.Amount, 8)
		record := DailyCashbackRecord{
			UserID:         spend.UserID,
			RuleID:         &rule.ID,
			BusinessDate:   result.BusinessDate,
			SpendAmount:    roundTo(spend.Amount, 8),
			RatePercent:    rule.RatePercent,
			CashbackAmount: cashback,
			Status:         "applied",
		}
		applied, appliedRecord, err := s.repo.InsertRecordAndCredit(ctx, record)
		if err != nil {
			return nil, err
		}
		if !applied {
			result.SkippedUsers++
			continue
		}
		result.AppliedUsers++
		result.TotalCashback = roundTo(result.TotalCashback+cashback, 8)
		s.invalidateUserCaches(ctx, spend.UserID)
		if appliedRecord != nil && appliedRecord.CashbackAmount > 0 {
			logger.LegacyPrintf("service.daily_cashback", "[DailyCashback] applied user=%d date=%s spend=%.8f cashback=%.8f", spend.UserID, result.BusinessDate, spend.Amount, cashback)
		}
	}
	return result, nil
}

func normalizeDailyCashbackRule(rule DailyCashbackRule) DailyCashbackRule {
	rule.Name = strings.TrimSpace(rule.Name)
	if len(rule.Name) > 100 {
		rule.Name = rule.Name[:100]
	}
	rule.MinAmount = roundTo(rule.MinAmount, 8)
	rule.RatePercent = roundTo(rule.RatePercent, 4)
	if rule.MaxAmount != nil {
		v := roundTo(*rule.MaxAmount, 8)
		rule.MaxAmount = &v
	}
	return rule
}

func validateDailyCashbackRule(rule DailyCashbackRule) error {
	if math.IsNaN(rule.MinAmount) || math.IsInf(rule.MinAmount, 0) || rule.MinAmount < 0 {
		return infraerrors.BadRequest("INVALID_AMOUNT", "invalid min amount")
	}
	if rule.MaxAmount != nil {
		v := *rule.MaxAmount
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= rule.MinAmount {
			return infraerrors.BadRequest("INVALID_AMOUNT", "max amount must be greater than min amount")
		}
	}
	if math.IsNaN(rule.RatePercent) || math.IsInf(rule.RatePercent, 0) || rule.RatePercent <= 0 || rule.RatePercent > 100 {
		return infraerrors.BadRequest("INVALID_RATE", "cashback rate must be between 0 and 100")
	}
	return nil
}

func parseBusinessDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, infraerrors.BadRequest("INVALID_DATE", "business_date is required")
	}
	day, err := timezone.ParseInLocation("2006-01-02", raw)
	if err != nil {
		return time.Time{}, infraerrors.BadRequest("INVALID_DATE", "invalid business_date")
	}
	return day, nil
}

func matchDailyCashbackRule(amount float64, rules []DailyCashbackRule) *DailyCashbackRule {
	for i := range rules {
		rule := &rules[i]
		if amount < rule.MinAmount {
			continue
		}
		if rule.MaxAmount != nil && amount >= *rule.MaxAmount {
			continue
		}
		return rule
	}
	return nil
}

func (s *DailyCashbackService) invalidateUserCaches(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCacheService != nil {
		if err := s.billingCacheService.InvalidateUserBalance(ctx, userID); err != nil {
			logger.LegacyPrintf("service.daily_cashback", "[DailyCashback] invalidate user balance cache failed user=%d err=%v", userID, err)
		}
	}
}
