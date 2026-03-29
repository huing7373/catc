package repository

import (
	"github.com/huing7373/catc/server/internal/model"
	"gorm.io/gorm"
)

// UserRepository handles user data access.
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepo creates a new UserRepository.
func NewUserRepo(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByID finds a user by ID, scoped to non-deleted users.
func (r *UserRepository) FindByID(userID string) (*model.User, error) {
	var user model.User
	err := r.db.Where("id = ? AND is_deleted = FALSE", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByAppleID finds a user by Apple ID, scoped to non-deleted users.
func (r *UserRepository) FindByAppleID(appleID string) (*model.User, error) {
	var user model.User
	err := r.db.Where("apple_id = ? AND is_deleted = FALSE", appleID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Create inserts a new user.
func (r *UserRepository) Create(user *model.User) error {
	return r.db.Create(user).Error
}
