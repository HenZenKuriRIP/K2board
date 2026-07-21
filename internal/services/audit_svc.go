package services

import (
	"K2board/internal/database"
	"K2board/internal/models"
)

type AuditService struct{}

func NewAuditService() *AuditService { return &AuditService{} }

func (s *AuditService) Log(adminID uint, action, target string, targetID uint, detail string) {
	database.DB.Create(&models.AuditLog{
		AdminID: adminID, Action: action, Target: target, TargetID: targetID, Detail: detail,
	})
}

func (s *AuditService) List(page, pageSize int) ([]models.AuditLog, int64, error) {
	var logs []models.AuditLog
	var total int64
	database.DB.Model(&models.AuditLog{}).Count(&total)
	offset := (page - 1) * pageSize
	err := database.DB.Order("id DESC").Offset(offset).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}
