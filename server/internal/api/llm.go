package api

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/automedic/automedic/internal/crypto"
	"github.com/automedic/automedic/internal/model"
	"github.com/gin-gonic/gin"
)

// ---------- 厂家 ----------

func (h *Handlers) ListProviders(c *gin.Context) {
	var list []model.Provider
	if err := h.db.Order("id ASC").Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	out := make([]gin.H, 0, len(list))
	for _, p := range list {
		key, _ := h.crypt.Decrypt(p.APIKeyEnc)
		out = append(out, gin.H{
			"id": p.ID, "name": p.Name, "key": p.Key, "kind": p.Kind, "base_url": p.BaseURL,
			"max_input_context": p.MaxInputContext, "max_output_context": p.MaxOutputContext,
			"enabled": p.Enabled, "remark": p.Remark, "api_key_masked": maskKey(key),
			"created_at": p.CreatedAt, "updated_at": p.UpdatedAt,
		})
	}
	var models []model.LLMModel
	h.db.Preload("Provider").Order("id ASC").Find(&models)
	OK(c, gin.H{"providers": out, "models": models})
}

func (h *Handlers) CreateProvider(c *gin.Context) {
	var in struct {
		Name             string `json:"name"`
		Key              string `json:"key"`
		Kind             string `json:"kind"`
		BaseURL          string `json:"base_url"`
		APIKey           string `json:"api_key"`
		MaxInputContext  int64  `json:"max_input_context"`
		MaxOutputContext int64  `json:"max_output_context"`
		Enabled          *bool  `json:"enabled"`
		Remark           string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if in.Name == "" || in.Key == "" {
		BadRequest(c, "厂家名称与标识不能为空")
		return
	}
	p := &model.Provider{
		Name: in.Name, Key: in.Key, Kind: in.Kind, BaseURL: in.BaseURL,
		MaxInputContext: in.MaxInputContext, MaxOutputContext: in.MaxOutputContext,
		Remark: in.Remark, Enabled: true,
	}
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}
	if in.APIKey != "" {
		enc, err := h.crypt.Encrypt(in.APIKey)
		if err != nil {
			BadRequest(c, err)
			return
		}
		p.APIKeyEnc = enc
	}
	if p.MaxInputContext == 0 {
		p.MaxInputContext = 1000000
	}
	if p.MaxOutputContext == 0 {
		p.MaxOutputContext = 65536
	}
	if err := h.db.Create(p).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, p)
}

func (h *Handlers) UpdateProvider(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var p model.Provider
	if err := h.db.First(&p, id).Error; err != nil {
		NotFound(c, "厂家不存在")
		return
	}
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if v, ok := in["name"].(string); ok && v != "" {
		p.Name = v
	}
	if v, ok := in["kind"].(string); ok {
		p.Kind = v
	}
	if v, ok := in["base_url"].(string); ok {
		p.BaseURL = v
	}
	if v, ok := in["remark"].(string); ok {
		p.Remark = v
	}
	if v, ok := in["enabled"].(bool); ok {
		p.Enabled = v
	}
	if v, ok := in["max_input_context"].(float64); ok {
		p.MaxInputContext = int64(v)
	}
	if v, ok := in["max_output_context"].(float64); ok {
		p.MaxOutputContext = int64(v)
	}
	if v, ok := in["api_key"].(string); ok && v != "" {
		enc, err := h.crypt.Encrypt(v)
		if err != nil {
			BadRequest(c, err)
			return
		}
		p.APIKeyEnc = enc
	}
	if err := h.db.Save(&p).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, p)
}

func (h *Handlers) DeleteProvider(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var n int64
	h.db.Model(&model.LLMModel{}).Where("provider_id = ?", id).Count(&n)
	if n > 0 {
		BadRequest(c, fmt.Sprintf("该厂家下仍有 %d 个模型，请先删除", n))
		return
	}
	if err := h.db.Delete(&model.Provider{}, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

// ---------- 模型 ----------

func (h *Handlers) ListModels(c *gin.Context) {
	var list []model.LLMModel
	q := h.db.Preload("Provider").Order("id ASC")
	if pid := c.Query("provider_id"); pid != "" {
		q = q.Where("provider_id = ?", pid)
	}
	if err := q.Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, list)
}

func (h *Handlers) GetModel(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var m model.LLMModel
	if err := h.db.Preload("Provider").First(&m, id).Error; err != nil {
		NotFound(c, "模型不存在")
		return
	}
	OK(c, m)
}

func (h *Handlers) CreateModel(c *gin.Context) {
	var m model.LLMModel
	if err := c.ShouldBindJSON(&m); err != nil {
		BadRequest(c, err)
		return
	}
	if m.ProviderID == 0 || m.Name == "" || m.Slug == "" {
		BadRequest(c, "厂家、模型名称与模型标识不能为空")
		return
	}
	if m.InputContext <= 0 {
		m.InputContext = 131072
	}
	if m.OutputContext <= 0 {
		m.OutputContext = 65536
	}
	if m.IsDefault {
		h.db.Model(&model.LLMModel{}).Where("1=1").Update("is_default", false)
	}
	if err := h.db.Create(&m).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, m)
}

func (h *Handlers) UpdateModel(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var m model.LLMModel
	if err := h.db.First(&m, id).Error; err != nil {
		NotFound(c, "模型不存在")
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, err)
		return
	}
	delete(body, "id")
	if d, ok := body["is_default"].(bool); ok && d {
		h.db.Model(&model.LLMModel{}).Where("1=1").Update("is_default", false)
	}
	if err := h.db.Model(&m).Updates(body).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, m)
}

func (h *Handlers) DeleteModel(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var n int64
	h.db.Model(&model.Task{}).Where("model_id = ?", id).Count(&n)
	if n > 0 {
		BadRequest(c, fmt.Sprintf("已有 %d 个任务使用该模型，建议禁用而非删除", n))
		return
	}
	if err := h.db.Delete(&model.LLMModel{}, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

// ---------- 项目令牌 ----------

func cryptoRand(b []byte) (int, error) { return rand.Read(b) }

func generateToken() (string, string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	if _, err := cryptoRand(b); err != nil {
		return "", "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	raw := "am_" + string(b)
	return raw, raw[:8], nil
}

func (h *Handlers) ListTokens(c *gin.Context) {
	var list []model.IngestToken
	q := h.db.Model(&model.IngestToken{}).Preload("Project").Order("id DESC")
	if pid := c.Query("project_id"); pid != "" {
		q = q.Where("project_id = ?", pid)
	}
	var total int64
	q.Count(&total)
	page, size := QueryPage(c)
	if err := q.Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	for i := range list {
		list[i].TokenHash = ""
	}
	OKPage(c, list, Page{Page: page, PageSize: size, Total: total})
}

func (h *Handlers) CreateToken(c *gin.Context) {
	var in struct {
		ProjectID uint       `json:"project_id"`
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
		AllowCIDR string     `json:"allow_cidr"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if in.ProjectID == 0 || in.Name == "" {
		BadRequest(c, "项目与名称不能为空")
		return
	}
	raw, prefix, err := generateToken()
	if err != nil {
		ServerError(c, err)
		return
	}
	t := &model.IngestToken{
		ProjectID: in.ProjectID, Name: in.Name, Prefix: prefix,
		TokenHash: crypto.SHA256(raw), Enabled: true,
		ExpiresAt: in.ExpiresAt, AllowCIDR: in.AllowCIDR,
	}
	if err := h.db.Create(t).Error; err != nil {
		ServerError(c, err)
		return
	}
	// 明文只在创建时返回一次
	OK(c, gin.H{"token": t, "plain_token": raw})
}

func (h *Handlers) UpdateToken(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var t model.IngestToken
	if err := h.db.First(&t, id).Error; err != nil {
		NotFound(c, "令牌不存在")
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, err)
		return
	}
	delete(body, "id")
	delete(body, "token_hash")
	if err := h.db.Model(&t).Updates(body).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, t)
}

func (h *Handlers) DeleteToken(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if err := h.db.Delete(&model.IngestToken{}, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

// ---------- 规则 ----------

func (h *Handlers) ListRules(c *gin.Context) {
	var list []model.Rule
	q := h.db.Model(&model.Rule{}).Preload("Project")
	if pid := c.Query("project_id"); pid != "" {
		q = q.Where("project_id = ?", pid)
	}
	var total int64
	q.Count(&total)
	page, size := QueryPage(c)
	if err := q.Order("priority DESC, id ASC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	OKPage(c, list, Page{Page: page, PageSize: size, Total: total})
}

func (h *Handlers) CreateRule(c *gin.Context) {
	var r model.Rule
	if err := c.ShouldBindJSON(&r); err != nil {
		BadRequest(c, err)
		return
	}
	if r.ProjectID == 0 || r.Name == "" {
		BadRequest(c, "项目与规则名称不能为空")
		return
	}
	if r.Action == "" {
		r.Action = "fix"
	}
	if r.Action != "fix" && r.Action != "ignore" {
		BadRequest(c, "action 只能是 fix 或 ignore")
		return
	}
	if err := h.db.Create(&r).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, r)
}

func (h *Handlers) UpdateRule(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var r model.Rule
	if err := h.db.First(&r, id).Error; err != nil {
		NotFound(c, "规则不存在")
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, err)
		return
	}
	delete(body, "id")
	if err := h.db.Model(&r).Updates(body).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, r)
}

func (h *Handlers) DeleteRule(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if err := h.db.Delete(&model.Rule{}, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

func maskKey(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 10 {
		return strings.Repeat("*", len(s))
	}
	return s[:6] + strings.Repeat("*", 6) + s[len(s)-4:]
}

var _ = errors.New
