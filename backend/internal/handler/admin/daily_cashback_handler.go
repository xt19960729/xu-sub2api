package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type DailyCashbackHandler struct {
	cashbackService *service.DailyCashbackService
}

func NewDailyCashbackHandler(cashbackService *service.DailyCashbackService) *DailyCashbackHandler {
	return &DailyCashbackHandler{cashbackService: cashbackService}
}

type DailyCashbackRuleRequest struct {
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	MinAmount   float64  `json:"min_amount"`
	MaxAmount   *float64 `json:"max_amount"`
	RatePercent float64  `json:"rate_percent"`
	SortOrder   int      `json:"sort_order"`
}

func (h *DailyCashbackHandler) ListRules(c *gin.Context) {
	rules, err := h.cashbackService.ListRules(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rules)
}

func (h *DailyCashbackHandler) CreateRule(c *gin.Context) {
	var req DailyCashbackRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	rule, err := h.cashbackService.CreateRule(c.Request.Context(), service.DailyCashbackRule{
		Name:        req.Name,
		Enabled:     req.Enabled,
		MinAmount:   req.MinAmount,
		MaxAmount:   req.MaxAmount,
		RatePercent: req.RatePercent,
		SortOrder:   req.SortOrder,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rule)
}

func (h *DailyCashbackHandler) UpdateRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
	}
	var req DailyCashbackRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	rule, err := h.cashbackService.UpdateRule(c.Request.Context(), service.DailyCashbackRule{
		ID:          id,
		Name:        req.Name,
		Enabled:     req.Enabled,
		MinAmount:   req.MinAmount,
		MaxAmount:   req.MaxAmount,
		RatePercent: req.RatePercent,
		SortOrder:   req.SortOrder,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rule)
}

func (h *DailyCashbackHandler) DeleteRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
	}
	if err := h.cashbackService.DeleteRule(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func (h *DailyCashbackHandler) ListRecords(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.cashbackService.ListRecords(c.Request.Context(), service.DailyCashbackRecordFilter{
		Search:       c.Query("search"),
		BusinessDate: c.Query("business_date"),
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

type RunDailyCashbackRequest struct {
	BusinessDate string `json:"business_date"`
}

func (h *DailyCashbackHandler) Run(c *gin.Context) {
	var req RunDailyCashbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.cashbackService.RunForDate(c.Request.Context(), strings.TrimSpace(req.BusinessDate))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
