package services

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/utils"
)

type UserService struct{}

func NewUserService() *UserService {
	return &UserService{}
}

// CreateUser creates a new user with auto-generated UUID, subscribe token, and invite code.
func (s *UserService) CreateUser(user *models.User) error {
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))
	user.UUID = utils.GenerateUUID()
	token, err := utils.GenerateToken(16)
	if err != nil {
		return err
	}
	user.Token = token

	if user.Password != "" {
		hashed, err := utils.HashPassword(user.Password)
		if err != nil {
			return err
		}
		user.Password = hashed
	}

	// Unique invite code (retry on rare collision; app-level uniqueness)
	if user.InviteCode == "" {
		for i := 0; i < 12; i++ {
			code, err := GenerateInviteCode()
			if err != nil {
				return err
			}
			var n int64
			database.DB.Model(&models.User{}).Where("invite_code = ?", code).Count(&n)
			if n > 0 {
				continue
			}
			user.InviteCode = code
			return database.DB.Create(user).Error
		}
		return errors.New("failed to allocate invite code")
	}

	return database.DB.Create(user).Error
}

// GetUserByID retrieves a user by ID.
func (s *UserService) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	if err := database.DB.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by email.
// Callers and CreateUser always normalize email to lowercase, so exact match
// is correct and keeps the unique index usable.
func (s *UserService) GetUserByEmail(email string) (*models.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var user models.User
	if err := database.DB.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// Allowed user list sort columns (whitelist — never interpolate raw client input).
var userListSortColumns = map[string]string{
	"id":            "id",
	"email":         "email",
	"traffic_used":  "traffic_used",
	"traffic_limit": "traffic_limit",
	"expire_at":     "expire_at",
	"created_at":    "created_at",
	"group_id":      "group_id",
	"plan_id":       "plan_id",
	"enable":        "enable",
	"device_limit":  "device_limit",
	"speed_limit":   "speed_limit",
}

// NormalizeUserListSort returns safe SQL column + ASC/DESC.
func NormalizeUserListSort(sortBy, sortOrder string) (col, order string) {
	col = userListSortColumns[strings.ToLower(strings.TrimSpace(sortBy))]
	if col == "" {
		col = "id"
	}
	o := strings.ToLower(strings.TrimSpace(sortOrder))
	if o == "asc" {
		order = "ASC"
	} else {
		order = "DESC"
	}
	return col, order
}

// ListUsers returns paginated users with optional search and full-table sort.
func (s *UserService) ListUsers(page, pageSize int, search, sortBy, sortOrder string) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	query := database.DB.Model(&models.User{}).Where("is_admin = ?", false)
	if search != "" {
		if len(search) > 100 {
			return nil, 0, errors.New("search too long")
		}
		searchPattern := "%" + search + "%"
		query = query.Where("email LIKE ?", searchPattern)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	col, dir := NormalizeUserListSort(sortBy, sortOrder)
	// Stable secondary key so equal values don't jump across pages
	orderClause := col + " " + dir + ", id " + dir

	offset := (page - 1) * pageSize
	if err := query.Order(orderClause).Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// UpdateUser updates a user's fields. Does not update password via this method.
func (s *UserService) UpdateUser(id uint, updates map[string]interface{}) error {
	if raw, ok := updates["email"]; ok {
		if email, ok := raw.(string); ok {
			updates["email"] = strings.ToLower(strings.TrimSpace(email))
		}
	}
	result := database.DB.Model(&models.User{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteUser deletes a user by ID.
func (s *UserService) DeleteUser(id uint) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&models.User{}, id).Error; err != nil {
			return err
		}
		// Clean up associated data
		tx.Where("user_id = ?", id).Delete(&models.TrafficLog{})
		tx.Where("user_id = ?", id).Delete(&models.NodeOnline{})
		tx.Where("user_id = ?", id).Delete(&models.CommissionLedger{})
		tx.Where("user_id = ?", id).Delete(&models.CommissionWithdraw{})
		// Clear inviter binding on invitees (keep accounts)
		tx.Model(&models.User{}).Where("inviter_id = ?", id).Update("inviter_id", 0)
		return nil
	})
}

// ResetUserUUID generates a new UUID for the user.
func (s *UserService) ResetUserUUID(id uint) (string, error) {
	newUUID := utils.GenerateUUID()
	if err := database.DB.Model(&models.User{}).Where("id = ?", id).Update("uuid", newUUID).Error; err != nil {
		return "", err
	}
	return newUUID, nil
}

// ResetUserToken generates a new subscribe token for the user.
func (s *UserService) ResetUserToken(id uint) (string, error) {
	newToken, err := utils.GenerateToken(16)
	if err != nil {
		return "", err
	}
	if err := database.DB.Model(&models.User{}).Where("id = ?", id).Update("token", newToken).Error; err != nil {
		return "", err
	}
	return newToken, nil
}

// Authenticate verifies email/password and returns the user.
func (s *UserService) Authenticate(email, password string) (*models.User, error) {
	user, err := s.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	hash := strings.TrimSpace(user.Password)
	if hash == "" {
		return nil, errors.New("invalid password")
	}
	// Reject non-bcrypt storage (corrupt / accidental plaintext write)
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") && !strings.HasPrefix(hash, "$2y$") {
		return nil, errors.New("invalid password")
	}
	if !utils.CheckPassword(password, hash) {
		return nil, errors.New("invalid password")
	}

	return user, nil
}

// ResetTraffic zeros a user's traffic_used counter.
func (s *UserService) ResetTraffic(id uint) error {
	return database.DB.Model(&models.User{}).Where("id = ?", id).Updates(map[string]any{
		"traffic_used":          0,
		"last_traffic_reset_at": time.Now().Unix(),
	}).Error
}

// UpdatePassword changes a user's password (bcrypt). Verifies round-trip before return.
func (s *UserService) UpdatePassword(id uint, newPassword string) error {
	newPassword = strings.TrimSpace(newPassword) // trim accidental paste spaces around simple passwords
	if len(newPassword) < 6 {
		return errors.New("password too short")
	}
	// Never store an already-hashed-looking string without re-hashing is fine;
	// admin always sends plaintext. Guard against double-hash if caller passes bcrypt.
	if strings.HasPrefix(newPassword, "$2a$") || strings.HasPrefix(newPassword, "$2b$") || strings.HasPrefix(newPassword, "$2y$") {
		return errors.New("invalid password format")
	}
	hashed, err := utils.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if !utils.CheckPassword(newPassword, hashed) {
		return errors.New("password hash verify failed")
	}
	res := database.DB.Model(&models.User{}).Where("id = ?", id).Update("password", hashed)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	// Reload and verify the value that is actually in DB (catches truncation / wrong DB)
	var u models.User
	if err := database.DB.Select("id, password").First(&u, id).Error; err != nil {
		return err
	}
	if !utils.CheckPassword(newPassword, strings.TrimSpace(u.Password)) {
		return errors.New("password not readable after save")
	}
	return nil
}
