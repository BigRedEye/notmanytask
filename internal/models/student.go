package models

type Student struct {
	ID         int    `gorm:"primaryKey"`
	Login      string `gorm:"uniqueIndex"`
	Repository *string

	GroupName string
	FirstName string
	LastName  string
}
