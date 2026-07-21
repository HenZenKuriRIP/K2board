package admin

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

// validateNodePQ enforces safe PQ pairing without blocking legacy empty fields.
func validateNodePQ(reality json.RawMessage, vlessDecryption string) error {
	seed := strings.TrimSpace(utils.RealityString(reality, "mldsa65_seed"))
	priv := strings.TrimSpace(utils.RealityString(reality, "private_key"))
	if seed != "" && priv != "" && seed == priv {
		return errors.New("mldsa65_seed 不能与 REALITY private_key 相同")
	}
	// Encryption string presence is allowed; Fallback is node-side config (not in panel model).
	_ = utils.NormalizeVlessCrypto(vlessDecryption)
	return nil
}

type NodeHandler struct {
	nodeSvc *services.NodeService
}

func NewNodeHandler() *NodeHandler {
	return &NodeHandler{
		nodeSvc: services.NewNodeService(),
	}
}

type CreateNodeRequest struct {
	Name            string          `json:"name" binding:"required"`
	GroupID         uint            `json:"group_id"`
	GroupIDs        []uint          `json:"group_ids"`
	NodeType        string          `json:"node_type" binding:"required"`
	Cipher          string          `json:"cipher"`
	Host            string          `json:"host"`
	Port            int             `json:"port"`
	Network         string          `json:"network"`
	TLS             int             `json:"tls"`
	TLStype         string          `json:"tls_type"`
	Path            string          `json:"path"`
	SNI             string          `json:"sni"`
	ServiceName     string          `json:"service_name"`
	Flow             string          `json:"flow"`
	SpeedLimit       float64         `json:"speed_limit"`
	RealitySettings  json.RawMessage `json:"reality_settings"`
	VlessDecryption  string          `json:"vless_decryption"`
	VlessEncryption  string          `json:"vless_encryption"`
}

type UpdateNodeRequest struct {
	Name             *string          `json:"name"`
	GroupID          *uint            `json:"group_id"`
	GroupIDs         []uint           `json:"group_ids"`
	NodeType         *string          `json:"node_type"`
	Cipher           *string          `json:"cipher"`
	Host             *string          `json:"host"`
	Port             *int             `json:"port"`
	Network          *string          `json:"network"`
	TLS              *int             `json:"tls"`
	TLStype          *string          `json:"tls_type"`
	Path             *string          `json:"path"`
	SNI              *string          `json:"sni"`
	ServiceName      *string          `json:"service_name"`
	Flow             *string          `json:"flow"`
	SpeedLimit       *float64         `json:"speed_limit"`
	RealitySettings  *json.RawMessage `json:"reality_settings"`
	VlessDecryption  *string          `json:"vless_decryption"`
	VlessEncryption  *string          `json:"vless_encryption"`
	Enable           *bool            `json:"enable"`
}

// List returns all nodes with online counts.
func (h *NodeHandler) List(c *gin.Context) {
	nodeType := c.Query("node_type")
	nodes, err := h.nodeSvc.ListNode(nodeType)
	if err != nil {
		utils.InternalError(c, "failed to fetch nodes")
		return
	}

	if nodes == nil {
		nodes = []models.Node{}
	}

	// Enrich with online counts
	for i := range nodes {
		nodes[i].OnlineCount = h.nodeSvc.GetNodeOnlineCount(nodes[i].ID)
		nodes[i].Status = nodes[i].ComputeStatus()
	}

	utils.Success(c, nodes)
}

// Get returns a single node by ID.
func (h *NodeHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid node id")
		return
	}

	node, err := h.nodeSvc.GetNodeByID(uint(id))
	if err != nil {
		utils.InternalError(c, "failed to fetch node")
		return
	}
	if node == nil {
		utils.NotFound(c, "node not found")
		return
	}
	node.Status = node.ComputeStatus()

	utils.Success(c, node)
}

// Create creates a new node.
func (h *NodeHandler) Create(c *gin.Context) {
	var req CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	node := &models.Node{
		Name:            req.Name,
		GroupID:         req.GroupID,
		GroupIDs:        resolveGroupIDs(req.GroupIDs, req.GroupID),
		NodeType:        req.NodeType,
		Cipher:          req.Cipher,
		Host:            req.Host,
		Port:            req.Port,
		Network:         req.Network,
		TLS:             req.TLS,
		TLStype:         req.TLStype,
		Path:            req.Path,
		SNI:             req.SNI,
		ServiceName:     req.ServiceName,
		Flow:             req.Flow,
		SpeedLimit:       req.SpeedLimit,
		RealitySettings:  req.RealitySettings,
		VlessDecryption:  req.VlessDecryption,
		VlessEncryption:  req.VlessEncryption,
		Enable:           true,
	}

	// Auto-generate REALITY keypair if TLS=2 and fields are empty
	if node.TLS == 2 && node.NodeType == "vless" {
		node.RealitySettings = utils.MergeRealityJSON(req.RealitySettings, node.SNI)
	}
	if err := validateNodePQ(node.RealitySettings, node.VlessDecryption); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	if node.Network == "" {
		node.Network = "ws"
	}
	if node.Port == 0 {
		node.Port = 443
	}
	if node.TLS == 0 {
		node.TLS = 1
	}
	if node.Cipher == "" {
		node.Cipher = "aes-256-gcm"
	}
	if node.NodeType == "vless" && node.Flow == "" && node.TLS >= 1 {
		node.Flow = "xtls-rprx-vision"
	}

	if err := h.nodeSvc.CreateNode(node); err != nil {
		utils.InternalError(c, "failed to create node: "+err.Error())
		return
	}

	node.Status = node.ComputeStatus()
	utils.Created(c, node)
	services.BumpConfigVersion()
}

// Update updates an existing node.
func (h *NodeHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid node id")
		return
	}

	var req UpdateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// Fetch existing node to determine effective node type for guards
	existingNode, _ := h.nodeSvc.GetNodeByID(uint(id))
	effectiveNodeType := ""
	if existingNode != nil {
		effectiveNodeType = existingNode.NodeType
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.NodeType != nil {
		updates["node_type"] = *req.NodeType
	}
	if req.Host != nil {
		updates["host"] = *req.Host
	}
	if req.Port != nil {
		updates["port"] = *req.Port
	}
	if req.Network != nil {
		updates["network"] = *req.Network
	}
	if req.TLS != nil {
		updates["tls"] = *req.TLS
	}
	if req.TLStype != nil {
		updates["tls_type"] = *req.TLStype
	}
	if req.Path != nil {
		updates["path"] = *req.Path
	}
	if req.SNI != nil {
		updates["sni"] = *req.SNI
	}
	if req.ServiceName != nil {
		updates["service_name"] = *req.ServiceName
	}
	if req.Flow != nil {
		updates["flow"] = *req.Flow
	}
	if req.SpeedLimit != nil {
		updates["speed_limit"] = *req.SpeedLimit
	}
	if req.GroupIDs != nil {
		updates["group_ids"] = req.GroupIDs
	} else if req.GroupID != nil {
		// Legacy single group_id — convert to group_ids
		updates["group_ids"] = []uint{*req.GroupID}
	}
	if req.Cipher != nil {
		updates["cipher"] = *req.Cipher
	}
	if req.RealitySettings != nil {
		updates["reality_settings"] = *req.RealitySettings
	}
	if req.VlessDecryption != nil {
		updates["vless_decryption"] = *req.VlessDecryption
	}
	if req.VlessEncryption != nil {
		updates["vless_encryption"] = *req.VlessEncryption
	}
	// Determine effective node type after potential type change
	nodeType := effectiveNodeType
	if req.NodeType != nil {
		nodeType = *req.NodeType
	}
	// Auto-generate REALITY params on update if TLS=2 for VLESS nodes only
	rs := req.RealitySettings
	if rs == nil && req.TLS != nil && *req.TLS == 2 && nodeType == "vless" {
		// User switched to REALITY mode — auto-generate defaults
		sni := ""
		if req.SNI != nil {
			sni = *req.SNI
		}
		autoRS := utils.MergeRealityJSON(nil, sni)
		updates["reality_settings"] = autoRS
	}
	if req.Enable != nil {
		updates["enable"] = *req.Enable
	}

	// Validate PQ pairing using effective values after update
	var checkRS json.RawMessage
	if req.RealitySettings != nil {
		checkRS = *req.RealitySettings
	} else if existingNode != nil {
		checkRS = existingNode.RealitySettings
	}
	checkDec := ""
	if req.VlessDecryption != nil {
		checkDec = *req.VlessDecryption
	} else if existingNode != nil {
		checkDec = existingNode.VlessDecryption
	}
	if err := validateNodePQ(checkRS, checkDec); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	if len(updates) > 0 {
		if err := h.nodeSvc.UpdateNode(uint(id), updates); err != nil {
			utils.InternalError(c, "failed to update node")
			return
		}
	}

	node, _ := h.nodeSvc.GetNodeByID(uint(id))
	if node == nil {
		utils.NotFound(c, "node not found")
		return
	}
	node.Status = node.ComputeStatus()
	utils.Success(c, node)
	services.BumpConfigVersion()
}

// Delete deletes a node.
func (h *NodeHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid node id")
		return
	}

	if err := h.nodeSvc.DeleteNode(uint(id)); err != nil {
		utils.InternalError(c, "failed to delete node")
		return
	}

	utils.SuccessMessage(c, "deleted")
	services.BumpConfigVersion() // config push
}

// AddCustomToken allows manual input of a communication key.
func (h *NodeHandler) AddCustomToken(c *gin.Context) {
	var req struct {
		NodeID uint   `json:"node_id"`
		Token  string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.NodeID == 0 || req.Token == "" {
		utils.BadRequest(c, "invalid request")
		return
	}
	_, err := h.nodeSvc.AddCustomNodeToken(req.NodeID, req.Token)
	if err != nil {
		utils.InternalError(c, "failed to add token")
		return
	}
	utils.Created(c, gin.H{"message": "token added"})
}

// DeleteToken removes a node communication key by token ID.
func (h *NodeHandler) DeleteToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("tokenId"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid token id")
		return
	}
	if err := h.nodeSvc.DeleteNodeToken(uint(id)); err != nil {
		utils.InternalError(c, "failed to delete token")
		return
	}
	utils.SuccessMessage(c, "token deleted")
}

// GenerateRealityParams returns auto-generated REALITY keypair + ShortID.
func (h *NodeHandler) GenerateRealityParams(c *gin.Context) {
	sni := c.Query("sni")
	p := utils.GenerateReality(sni)
	utils.Success(c, p)
}

// Metrics returns time-series CPU/mem/disk data for trend charts.
func (h *NodeHandler) Metrics(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid node id")
		return
	}
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	var metrics []models.NodeMetric
	database.DB.Where("node_id = ? AND created_at >= ?", uint(id), since).Order("created_at ASC").Find(&metrics)
	utils.Success(c, metrics)
}

// GenerateToken creates a new API token for a node.
func (h *NodeHandler) GenerateToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid node id")
		return
	}

	// Verify node exists
	node, err := h.nodeSvc.GetNodeByID(uint(id))
	if err != nil || node == nil {
		utils.NotFound(c, "node not found")
		return
	}
	node.Status = node.ComputeStatus()

	_, plaintext, err := h.nodeSvc.GenerateNodeToken(uint(id))
	if err != nil {
		utils.InternalError(c, "failed to generate token")
		return
	}

	utils.Created(c, gin.H{"token": plaintext, "warning": "save this token now — it cannot be retrieved again"})
	services.BumpConfigVersion()
}

// resolveGroupIDs merges new group_ids with legacy group_id.
// If group_ids is provided, it takes precedence. Otherwise falls back to single group_id.
func resolveGroupIDs(groupIDs []uint, groupID uint) []uint {
	if len(groupIDs) > 0 {
		return groupIDs
	}
	if groupID > 0 {
		return []uint{groupID}
	}
	return nil
}
