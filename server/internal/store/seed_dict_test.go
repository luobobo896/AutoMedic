package store

import (
	"testing"

	"github.com/automedic/automedic/internal/model"
)

func TestSeedDictsDoesNotOverwriteEdits(t *testing.T) {
	db := isolatedDB(t)
	if err := SeedDicts(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.DictItem{}).Where("\"group\" = ? AND value = ?", "language", "Go").
		Updates(map[string]any{"label": "Golang", "enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedDicts(db); err != nil {
		t.Fatal(err)
	}
	var it model.DictItem
	if err := db.Where("\"group\" = ? AND value = ?", "language", "Go").First(&it).Error; err != nil {
		t.Fatal(err)
	}
	if it.Label != "Golang" || it.Enabled {
		t.Fatalf("seed 不应覆盖已改项: %+v", it)
	}
	var n int64
	db.Model(&model.DictItem{}).Where("\"group\" = ?", "log_level").Count(&n)
	if n < 4 {
		t.Fatalf("log_level 应有默认项, n=%d", n)
	}
}
