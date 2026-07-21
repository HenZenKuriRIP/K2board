package admin

import (
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

type DashboardHandler struct{}

func NewDashboardHandler() *DashboardHandler { return &DashboardHandler{} }

func (h *DashboardHandler) GetStats(c *gin.Context) {
	var totalUsers, activeUsers, totalNodes, activeNodes int64
	// Exclude admin accounts from user counts
	database.DB.Model(&models.User{}).Where("is_admin = ?", false).Count(&totalUsers)
	database.DB.Model(&models.User{}).Where("is_admin = ? AND enable = ?", false, true).Count(&activeUsers)
	database.DB.Model(&models.Node{}).Count(&totalNodes)
	database.DB.Model(&models.Node{}).Where("enable = ?", true).Count(&activeNodes)

	var traffic struct{ TotalUpload, TotalDownload int64 }
	database.DB.Model(&models.TrafficLog{}).Select("COALESCE(SUM(upload), 0) as total_upload, COALESCE(SUM(download), 0) as total_download").Scan(&traffic)

	var totalTrafficUsed int64
	database.DB.Model(&models.User{}).Where("is_admin = ?", false).Select("COALESCE(SUM(traffic_used), 0)").Scan(&totalTrafficUsed)

	onlineUsers := services.CountDistinctOnlineUsers()
	host := services.CollectHostHealth()

	utils.Success(c, gin.H{
		"total_users": totalUsers, "active_users": activeUsers, "online_users": onlineUsers,
		"total_nodes": totalNodes, "active_nodes": activeNodes,
		"total_upload": traffic.TotalUpload, "total_download": traffic.TotalDownload,
		"total_traffic_used": totalTrafficUsed,
		"panel_version":      services.PanelVersion,
		"host":               host,
	})
}

// Trend returns daily traffic trend for the past 30 days (sorted by date ascending).
func (h *DashboardHandler) Trend(c *gin.Context) {
	today := time.Now().Truncate(24 * time.Hour).Unix()
	since := today - int64(30*86400)

	var stats []models.StatServer
	database.DB.Where("record_at >= ?", since).Order("record_at ASC").Find(&stats)

	type DayStat struct {
		Date     string `json:"date"`
		Upload   int64  `json:"upload"`
		Download int64  `json:"download"`
	}
	dayMap := make(map[int64]*DayStat)
	for _, s := range stats {
		if e, ok := dayMap[s.RecordAt]; ok {
			e.Upload += s.Upload
			e.Download += s.Download
		} else {
			dayMap[s.RecordAt] = &DayStat{
				Date:     time.Unix(s.RecordAt, 0).Format("01-02"),
				Upload:   s.Upload,
				Download: s.Download,
			}
		}
	}

	keys := make([]int64, 0, len(dayMap))
	for k := range dayMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	result := make([]DayStat, 0, len(keys))
	for _, k := range keys {
		result = append(result, *dayMap[k])
	}
	utils.Success(c, result)
}
