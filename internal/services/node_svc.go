package services

import (
	"errors"

	"gorm.io/gorm"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/utils"
)

type NodeService struct{}

func NewNodeService() *NodeService {
	return &NodeService{}
}

// CreateNode creates a new node with optional group mappings.
func (s *NodeService) CreateNode(node *models.Node) error {
	// Sync legacy group_id column from GroupIDs BEFORE DB insert
	if len(node.GroupIDs) > 0 && node.GroupID == 0 {
		node.GroupID = node.GroupIDs[0]
	}
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(node).Error; err != nil {
			return err
		}
		// Create node_group_mappings from GroupIDs
		if len(node.GroupIDs) > 0 {
			mappings := make([]models.NodeGroupMapping, 0, len(node.GroupIDs))
			for _, gid := range node.GroupIDs {
				mappings = append(mappings, models.NodeGroupMapping{NodeID: node.ID, GroupID: gid})
			}
			if err := tx.Create(&mappings).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetNodeByID retrieves a node by ID with tokens and group mappings loaded.
func (s *NodeService) GetNodeByID(id uint) (*models.Node, error) {
	var node models.Node
	if err := database.DB.Preload("NodeTokens").First(&node, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	s.populateGroupIDs(&node)
	return &node, nil
}

// ListNode returns all nodes with optional type filter.
func (s *NodeService) ListNode(nodeType string) ([]models.Node, error) {
	var nodes []models.Node
	query := database.DB.Preload("NodeTokens").Order("id DESC")
	if nodeType != "" {
		query = query.Where("node_type = ?", nodeType)
	}
	if err := query.Find(&nodes).Error; err != nil {
		return nil, err
	}
	s.populateGroupIDsBatch(nodes)
	s.attachServiceUserCounts(nodes)
	return nodes, nil
}

// attachServiceUserCounts sets UserCount = sum of non-admin users in the node's mapped groups.
// A user has a single group_id, so summing per-group counts equals the entitled population
// (users who may see this node under group mapping).
func (s *NodeService) attachServiceUserCounts(nodes []models.Node) {
	if len(nodes) == 0 {
		return
	}
	// Collect unique group IDs across nodes
	seen := make(map[uint]struct{})
	var gids []uint
	for _, n := range nodes {
		for _, gid := range n.GroupIDs {
			if gid == 0 {
				continue
			}
			if _, ok := seen[gid]; ok {
				continue
			}
			seen[gid] = struct{}{}
			gids = append(gids, gid)
		}
	}
	if len(gids) == 0 {
		return
	}

	type row struct {
		GroupID uint  `gorm:"column:group_id"`
		Cnt     int64 `gorm:"column:cnt"`
	}
	var rows []row
	_ = database.DB.Model(&models.User{}).
		Select("group_id, COUNT(*) AS cnt").
		Where("is_admin = ? AND group_id IN ?", false, gids).
		Group("group_id").
		Scan(&rows).Error
	byGroup := make(map[uint]int64, len(rows))
	for _, r := range rows {
		byGroup[r.GroupID] = r.Cnt
	}
	for i := range nodes {
		var sum int64
		for _, gid := range nodes[i].GroupIDs {
			sum += byGroup[gid]
		}
		nodes[i].UserCount = sum
	}
}

// UpdateNode updates a node's fields including group mappings.
func (s *NodeService) UpdateNode(id uint, updates map[string]interface{}) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		// Handle group_ids separately: replace all mappings
		groupIDsProcessed := false
		if rawGroupIDs, ok := updates["group_ids"]; ok {
			delete(updates, "group_ids")
			groupIDsProcessed = true

			// Delete existing mappings
			if err := tx.Where("node_id = ?", id).Delete(&models.NodeGroupMapping{}).Error; err != nil {
				return err
			}

			// Insert new mappings
			if groupIDs, ok := rawGroupIDs.([]uint); ok && len(groupIDs) > 0 {
				mappings := make([]models.NodeGroupMapping, 0, len(groupIDs))
				for _, gid := range groupIDs {
					mappings = append(mappings, models.NodeGroupMapping{NodeID: id, GroupID: gid})
				}
				if err := tx.Create(&mappings).Error; err != nil {
					return err
				}
			}
		}

		// Handle legacy group_id: convert to group_ids if no explicit group_ids given
		if rawGroupID, ok := updates["group_id"]; ok {
			delete(updates, "group_id")
			if !groupIDsProcessed {
				if err := tx.Where("node_id = ?", id).Delete(&models.NodeGroupMapping{}).Error; err != nil {
					return err
				}
				if gid, ok := rawGroupID.(uint); ok && gid > 0 {
					if err := tx.Create(&models.NodeGroupMapping{NodeID: id, GroupID: gid}).Error; err != nil {
						return err
					}
				}
			}
		}

		// Sync legacy group_id column to stay consistent with junction table
		if groupIDsProcessed {
			var currentMappings []models.NodeGroupMapping
			tx.Where("node_id = ?", id).Find(&currentMappings)
			syncGroupID := uint(0)
			if len(currentMappings) > 0 {
				syncGroupID = currentMappings[0].GroupID
			}
			tx.Model(&models.Node{}).Where("id = ?", id).Update("group_id", syncGroupID)
		}

		if len(updates) == 0 {
			return nil
		}

		result := tx.Model(&models.Node{}).Where("id = ?", id).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// DeleteNode deletes a node and its tokens + group mappings.
func (s *NodeService) DeleteNode(id uint) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ?", id).Delete(&models.NodeGroupMapping{}).Error; err != nil {
			return err
		}
		if err := tx.Where("node_id = ?", id).Delete(&models.NodeToken{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.Node{}, id).Error; err != nil {
			return err
		}
		return nil
	})
}

// GetNodeOnlineCount returns the count of distinct online users per node (OnlineTTL window).
func (s *NodeService) GetNodeOnlineCount(nodeID uint) int64 {
	return CountOnlineUsersOnNode(nodeID)
}

// GenerateNodeToken creates a new API token for a node.
// Returns the plaintext token (once), stores only the SHA256 hash.
func (s *NodeService) GenerateNodeToken(nodeID uint) (*models.NodeToken, string, error) {
	plaintext, err := utils.GenerateToken(16) // 32-char hex
	if err != nil {
		return nil, "", err
	}

	hashed := utils.SHA256(plaintext)

	nodeToken := &models.NodeToken{
		NodeID: nodeID,
		Token:  hashed,
	}

	if err := database.DB.Create(nodeToken).Error; err != nil {
		return nil, "", err
	}

	return nodeToken, plaintext, nil
}

// AddCustomNodeToken stores a manually-provided token (hashed before storage).
func (s *NodeService) AddCustomNodeToken(nodeID uint, plaintext string) (*models.NodeToken, error) {
	hashed := utils.SHA256(plaintext)
	nt := &models.NodeToken{NodeID: nodeID, Token: hashed}
	if err := database.DB.Create(nt).Error; err != nil {
		return nil, err
	}
	return nt, nil
}

// DeleteNodeToken removes a token by ID.
func (s *NodeService) DeleteNodeToken(tokenID uint) error {
	return database.DB.Delete(&models.NodeToken{}, tokenID).Error
}

// GetNodeTokenByValue looks up a node by its token value.
func (s *NodeService) GetNodeTokenByValue(nodeID uint, token string) (*models.NodeToken, error) {
	var nodeToken models.NodeToken
	if err := database.DB.Where("node_id = ? AND token = ?", nodeID, token).First(&nodeToken).Error; err != nil {
		return nil, err
	}
	return &nodeToken, nil
}

// GetNodeGroupIDs loads the group IDs for a single node from the junction table.
func (s *NodeService) GetNodeGroupIDs(nodeID uint) []uint {
	var mappings []models.NodeGroupMapping
	database.DB.Where("node_id = ?", nodeID).Find(&mappings)
	ids := make([]uint, 0, len(mappings))
	for _, m := range mappings {
		ids = append(ids, m.GroupID)
	}
	return ids
}

// populateGroupIDs loads GroupIDs for a single node.
func (s *NodeService) populateGroupIDs(node *models.Node) {
	node.GroupIDs = s.GetNodeGroupIDs(node.ID)
}

// populateGroupIDsBatch loads GroupIDs for a slice of nodes in a single query.
func (s *NodeService) populateGroupIDsBatch(nodes []models.Node) {
	if len(nodes) == 0 {
		return
	}
	nodeIDs := make([]uint, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.ID
	}

	var mappings []models.NodeGroupMapping
	database.DB.Where("node_id IN ?", nodeIDs).Find(&mappings)

	// Build lookup map
	groupMap := make(map[uint][]uint, len(nodes))
	for _, m := range mappings {
		groupMap[m.NodeID] = append(groupMap[m.NodeID], m.GroupID)
	}

	for i := range nodes {
		nodes[i].GroupIDs = groupMap[nodes[i].ID]
		if nodes[i].GroupIDs == nil {
			nodes[i].GroupIDs = []uint{}
		}
	}
}
