package repositories

import (
	dbmodels "user-authentication/app/repositories/db_models"

	"gorm.io/gorm"
)

// EnsureUser creates a users row for the given Supabase auth UUID if one does
// not already exist. It is called once per request by the JWT middleware so that
// all FK references to users(user_id) are satisfied automatically.
func EnsureUser(db *gorm.DB, userID, email string) error {
	if userID == "" {
		return nil
	}
	if email == "" {
		email = userID // placeholder — real email comes from the JWT claim
	}
	row := dbmodels.User{
		UserID:         userID,
		CustomUsername: userID, // UUID is stable and unique
		Email:          email,
	}
	// FirstOrCreate: INSERT only when the row is absent; ignore conflicts.
	return db.Where("user_id = ?", userID).FirstOrCreate(&row).Error
}
