package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"K2board/internal/services"
	"K2board/internal/utils"
)

type SubscribeHandler struct {
	subSvc *services.SubscribeService
}

func NewSubscribeHandler() *SubscribeHandler {
	return &SubscribeHandler{
		subSvc: services.NewSubscribeService(),
	}
}

// List returns paginated user subscription info.
func (h *SubscribeHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := c.Query("search")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	users, total, err := h.subSvc.GetAllUserSubscriptions(page, pageSize, search)
	if err != nil {
		utils.InternalError(c, "failed to fetch subscriptions")
		return
	}

	utils.Success(c, gin.H{
		"list":      users,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
