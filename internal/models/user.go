package models

type User struct {
	ID         int    `gorm:"primaryKey"`
	Login      string `gorm:"uniqueIndex"`
	Repository *string

	FirstName string
	LastName  string
	GroupName string
}
