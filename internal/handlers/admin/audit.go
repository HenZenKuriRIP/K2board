package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"K2board/internal/services"
	"K2board/internal/utils"
)

type AuditHandler struct {
	svc *services.AuditService
}

func NewAuditHandler() *AuditHandler {
	return &AuditHandler{svc: services.NewAuditService()}
}

func (h *AuditHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 { page = 1 }
	if pageSize < 1 || pageSize > 100 { pageSize = 20 }

	logs, total, err := h.svc.List(page, pageSize)
	if err != nil {
		utils.InternalError(c, "failed")
		return
	}
	utils.Success(c, gin.H{"list": logs, "total": total, "page": page, "page_size": pageSize})
}
