package models

// Setting stores key-value system configurations.
type Setting struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Key   string `gorm:"uniqueIndex;size:64;not null" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

func (Setting) TableName() string {
	return "settings"
}
