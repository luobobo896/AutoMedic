package model

// DictItem 下拉/多选选项，按 group 分组维护。
// ParentID 表达跨组父子（如厂家类型 → 模型标识）；多数分组仍是扁平列表。
type DictItem struct {
	BaseModel
	Group    string `gorm:"column:group;size:64;uniqueIndex:uk_dict_group_value;not null" json:"group"`
	Value    string `gorm:"size:256;uniqueIndex:uk_dict_group_value;not null" json:"value"`
	Label    string `gorm:"size:256" json:"label"`
	ParentID *uint  `gorm:"index:idx_dict_parent" json:"parent_id"`
	Extra    JSON   `gorm:"type:text" json:"extra"`
	Sort     int    `json:"sort"`
	Enabled  bool   `gorm:"default:true" json:"enabled"`
}

func (DictItem) TableName() string { return "dicts" }
