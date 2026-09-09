package api

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/automedic/automedic/internal/crypto"
	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ---------- 项目 ----------

func (h *Handlers) ListProjects(c *gin.Context) {
	var list []model.Project
	q := h.tdb(c).Model(&model.Project{})
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("name LIKE ? OR key LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}
	var total int64
	q.Count(&total)
	page, size := QueryPage(c)
	var repos []struct {
		ProjectID uint `gorm:"column:project_id"`
		Cnt       int64
	}
	h.tdb(c).Model(&model.Repository{}).Select("project_id, count(*) as cnt").Group("project_id").Scan(&repos)
	var rules []struct {
		ProjectID uint `gorm:"column:project_id"`
		Cnt       int64
	}
	h.tdb(c).Model(&model.Rule{}).Select("project_id, count(*) as cnt").Group("project_id").Scan(&rules)
	var tokens []struct {
		ProjectID uint `gorm:"column:project_id"`
		Cnt       int64
	}
	h.tdb(c).Model(&model.IngestToken{}).Select("project_id, count(*) as cnt").Group("project_id").Scan(&tokens)
	if err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	repoCnt := make(map[uint]int64, len(repos))
	for _, r := range repos {
		repoCnt[r.ProjectID] = r.Cnt
	}
	ruleCnt := make(map[uint]int64, len(rules))
	for _, r := range rules {
		ruleCnt[r.ProjectID] = r.Cnt
	}
	tokenCnt := make(map[uint]int64, len(tokens))
	for _, r := range tokens {
		tokenCnt[r.ProjectID] = r.Cnt
	}
	for i := range list {
		list[i].RepoCount = repoCnt[list[i].ID]
		list[i].RuleCount = ruleCnt[list[i].ID]
		list[i].TokenCount = tokenCnt[list[i].ID]
	}
	OKPage(c, list, Page{Page: page, PageSize: size, Total: total})
}

func (h *Handlers) CreateProject(c *gin.Context) {
	var p model.Project
	if err := c.ShouldBindJSON(&p); err != nil {
		BadRequest(c, err)
		return
	}
	if p.Name == "" || p.Key == "" {
		BadRequest(c, "名称与标识不能为空")
		return
	}
	if p.FixMode == "" {
		p.FixMode = model.FixModeSemi
	}
	p.TenantID = h.tenant(c)
	if err := h.tdb(c).Create(&p).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, p)
}

func (h *Handlers) GetProject(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var p model.Project
	if err := h.tdb(c).First(&p, id).Error; err != nil {
		NotFound(c, "项目不存在")
		return
	}
	var repos []model.Repository
	h.tdb(c).Preload("Credential").Preload("Model").Preload("ReviewModel").Where("project_id = ?", id).Find(&repos)
	var rules []model.Rule
	h.tdb(c).Where("project_id = ?", id).Order("priority DESC, id ASC").Find(&rules)
	var tokens []model.IngestToken
	h.tdb(c).Where("project_id = ?", id).Order("id DESC").Find(&tokens)
	for i := range tokens {
		tokens[i].TokenHash = ""
	}
	OK(c, gin.H{"project": p, "repos": repos, "rules": rules, "tokens": tokens})
}

func (h *Handlers) UpdateProject(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var p model.Project
	if err := h.tdb(c).First(&p, id).Error; err != nil {
		NotFound(c, "项目不存在")
		return
	}
	if !h.saveUpdates(c, h.tdb(c), &p,
		"name", "description", "fix_mode", "default_model_id", "default_review_model_id",
		"release_hook", "enabled", "context") {
		return
	}
	OK(c, p)
}

func (h *Handlers) DeleteProject(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if err := h.tdb(c).Transaction(func(tx *gorm.DB) error {
		tx.Where("project_id = ?", id).Delete(&model.Repository{})
		tx.Where("project_id = ?", id).Delete(&model.Rule{})
		tx.Where("project_id = ?", id).Delete(&model.IngestToken{})
		return tx.Delete(&model.Project{}, id).Error
	}); err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

// ---------- 仓库 ----------

func (h *Handlers) ListRepos(c *gin.Context) {
	var list []model.Repository
	q := h.tdb(c).Model(&model.Repository{}).Preload("Credential").Preload("Model").Preload("ReviewModel")
	if pid := c.Query("project_id"); pid != "" {
		q = q.Where("project_id = ?", pid)
	}
	var total int64
	q.Count(&total)
	page, size := QueryPage(c)
	if err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	OKPage(c, list, Page{Page: page, PageSize: size, Total: total})
}

func (h *Handlers) CreateRepo(c *gin.Context) {
	var r model.Repository
	if err := c.ShouldBindJSON(&r); err != nil {
		BadRequest(c, err)
		return
	}
	if r.URL == "" || r.Name == "" || r.ProjectID == 0 {
		BadRequest(c, "项目、名称与仓库地址不能为空")
		return
	}
	if r.Branch == "" {
		r.Branch = "main"
	}
	r.Project, r.Credential, r.Model, r.ReviewModel = nil, nil, nil, nil
	tid, ok := h.requireTenantOfProject(c, r.ProjectID)
	if !ok {
		return
	}
	r.TenantID = tid
	if err := h.tdb(c).Create(&r).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, r)
}

func (h *Handlers) GetRepo(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var r model.Repository
	if err := h.tdb(c).Preload("Credential").Preload("Project").Preload("Model").Preload("ReviewModel").First(&r, id).Error; err != nil {
		NotFound(c, "仓库不存在")
		return
	}
	OK(c, r)
}

func (h *Handlers) UpdateRepo(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var r model.Repository
	if err := h.tdb(c).First(&r, id).Error; err != nil {
		NotFound(c, "仓库不存在")
		return
	}
	if !h.saveUpdates(c, h.tdb(c), &r,
		"name", "url", "branch", "code_paths", "language", "credential_id",
		"model_id", "review_model_id", "auto_push", "enabled") {
		return
	}
	OK(c, r)
}

func (h *Handlers) DeleteRepo(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if err := h.tdb(c).Delete(&model.Repository{}, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

// TestRepo 连通性测试（ls-remote）
func (h *Handlers) TestRepo(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var r model.Repository
	if err := h.tdb(c).Preload("Credential").Preload("Project").First(&r, id).Error; err != nil {
		NotFound(c, "仓库不存在")
		return
	}
	start := time.Now()
	err := h.exec.TestRepo(c.Request.Context(), &r)
	usage := model.CredentialUsage{
		TenantID: r.TenantID, RefType: "repo", RefID: r.ID, Action: "test", CreatedAt: time.Now(),
	}
	if r.CredentialID != nil {
		usage.CredentialID = *r.CredentialID
	}
	if err != nil {
		usage.Result = "fail"
		usage.Message = err.Error()
		h.tdb(c).Create(&usage)
		Fail(c, 500, "连通性测试失败："+err.Error())
		return
	}
	usage.Result = "ok"
	usage.Message = fmt.Sprintf("耗时 %dms", time.Since(start).Milliseconds())
	h.tdb(c).Create(&usage)
	OK(c, gin.H{"ok": true, "cost_ms": time.Since(start).Milliseconds()})
}

func (h *Handlers) loadRepo(c *gin.Context) (*model.Repository, bool) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return nil, false
	}
	var r model.Repository
	if err := h.tdb(c).Preload("Credential").Preload("Project").First(&r, id).Error; err != nil {
		NotFound(c, "仓库不存在")
		return nil, false
	}
	return &r, true
}

func (h *Handlers) recordRepoUsage(r *model.Repository, action, result, message string) {
	usage := model.CredentialUsage{TenantID: r.TenantID, RefType: "repo", RefID: r.ID, Action: action, Result: result, Message: message, CreatedAt: time.Now()}
	if r.CredentialID != nil {
		usage.CredentialID = *r.CredentialID
	}
	h.db.Create(&usage)
}

// GetRepoTree 浏览仓库目录树（浅取远端 tip）
func (h *Handlers) GetRepoTree(c *gin.Context) {
	r, ok := h.loadRepo(c)
	if !ok {
		return
	}
	if h.exec == nil {
		Fail(c, 500, "执行器未就绪")
		return
	}
	start := time.Now()
	tree, err := h.exec.ListRepoTree(c.Request.Context(), r)
	if err != nil {
		h.recordRepoUsage(r, "tree", "fail", err.Error())
		Fail(c, 500, "读取目录树失败："+err.Error())
		return
	}
	h.recordRepoUsage(r, "tree", "ok", fmt.Sprintf("耗时 %dms", time.Since(start).Milliseconds()))
	OK(c, tree)
}

// GetRepoFile 预览仓库文件
func (h *Handlers) GetRepoFile(c *gin.Context) {
	r, ok := h.loadRepo(c)
	if !ok {
		return
	}
	path := c.Query("path")
	if strings.TrimSpace(path) == "" {
		BadRequest(c, "缺少 path")
		return
	}
	if h.exec == nil {
		Fail(c, 500, "执行器未就绪")
		return
	}
	file, err := h.exec.ShowRepoFile(c.Request.Context(), r, path)
	if err != nil {
		h.recordRepoUsage(r, "file", "fail", err.Error())
		Fail(c, 500, "读取文件失败："+err.Error())
		return
	}
	h.recordRepoUsage(r, "file", "ok", path)
	OK(c, file)
}

type reviewStartIn struct {
	Mode    string `json:"mode"`
	From    string `json:"from"`
	To      string `json:"to"`
	Path    string `json:"path"`
	ScanAll bool   `json:"scan_all"`
}

// StartRepoReview 手动触发 OCR 审查（异步）
func (h *Handlers) StartRepoReview(c *gin.Context) {
	r, ok := h.loadRepo(c)
	if !ok {
		return
	}
	if h.exec == nil {
		Fail(c, 500, "执行器未就绪")
		return
	}
	var in reviewStartIn
	_ = c.ShouldBindJSON(&in)
	job, err := h.exec.StartRepoReview(c.Request.Context(), r, service.ReviewStartInput{
		Mode: in.Mode, From: in.From, To: in.To, Path: in.Path, ScanAll: in.ScanAll,
	})
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	OK(c, service.ReviewJobAPI(job))
}

func (h *Handlers) ListRepoReviews(c *gin.Context) {
	r, ok := h.loadRepo(c)
	if !ok {
		return
	}
	if h.exec == nil {
		Fail(c, 500, "执行器未就绪")
		return
	}
	list, err := h.exec.ListRepoReviews(r.ID, h.tenant(c), 20)
	if err != nil {
		ServerError(c, err)
		return
	}
	out := make([]service.ReviewJobView, 0, len(list))
	for i := range list {
		out = append(out, service.ReviewJobAPI(&list[i]))
	}
	OK(c, out)
}

func (h *Handlers) GetReviewJob(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if h.exec == nil {
		Fail(c, 500, "执行器未就绪")
		return
	}
	// 归属校验：service 层按主键查询，必须先在 api 层确认审查任务属于当前租户
	var owned model.ReviewJob
	if err := h.tdb(c).Select("id").First(&owned, id).Error; err != nil {
		NotFound(c, "审查任务不存在")
		return
	}
	job, err := h.exec.GetReviewJob(id)
	if err != nil {
		NotFound(c, "审查任务不存在")
		return
	}
	OK(c, service.ReviewJobAPI(job))
}

type reviewFixIn struct {
	Keys []string `json:"keys"`
}

func (h *Handlers) FixReviewJob(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if h.exec == nil {
		Fail(c, 500, "执行器未就绪")
		return
	}
	var in reviewFixIn
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	// 归属校验：避免对其它租户的审查任务触发修复
	var owned model.ReviewJob
	if err := h.tdb(c).Select("id").First(&owned, id).Error; err != nil {
		NotFound(c, "审查任务不存在")
		return
	}
	ids, err := h.exec.FixReviewFindings(id, in.Keys)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	OK(c, gin.H{"task_ids": ids, "mode": "semi"})
}

// ---------- 凭证 ----------

type credIn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Username    string `json:"username"`
	Secret      string `json:"secret"`
	Passphrase  string `json:"passphrase"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}

func (h *Handlers) ListCredentials(c *gin.Context) {
	var list []model.Credential
	if err := h.tdb(c).Order("id DESC").Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	type item struct {
		model.Credential
		SecretMasked string     `json:"secret_masked"`
		UsedBy       []string   `json:"used_by"`
		UseCount     int64      `json:"use_count"`
		LastUsedAt   *time.Time `json:"last_used_at"`
	}
	out := make([]item, 0, len(list))
	for _, cd := range list {
		secret, _ := h.crypt.Decrypt(cd.SecretEnc)
		it := item{Credential: cd, SecretMasked: mask(secret)}
		var repos []model.Repository
		h.tdb(c).Where("credential_id = ?", cd.ID).Find(&repos)
		for _, r := range repos {
			it.UsedBy = append(it.UsedBy, "仓库:"+r.Name)
		}
		h.tdb(c).Model(&model.CredentialUsage{}).Where("credential_id = ?", cd.ID).Count(&it.UseCount)
		var last model.CredentialUsage
		if err := h.tdb(c).Where("credential_id = ?", cd.ID).Order("id DESC").First(&last).Error; err == nil {
			it.LastUsedAt = &last.CreatedAt
		}
		out = append(out, it)
	}
	OK(c, out)
}

func (h *Handlers) CreateCredential(c *gin.Context) {
	var in credIn
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	cd, err := h.buildCredential(&in)
	if err != nil {
		BadRequest(c, err)
		return
	}
	cd.TenantID = h.tenant(c)
	if err := h.tdb(c).Create(cd).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, cd)
}

func (h *Handlers) GetCredential(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var cd model.Credential
	if err := h.tdb(c).First(&cd, id).Error; err != nil {
		NotFound(c, "凭证不存在")
		return
	}
	secret, _ := h.crypt.Decrypt(cd.SecretEnc)
	OK(c, gin.H{"credential": cd, "secret_masked": mask(secret)})
}

func (h *Handlers) UpdateCredential(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var cd model.Credential
	if err := h.tdb(c).First(&cd, id).Error; err != nil {
		NotFound(c, "凭证不存在")
		return
	}
	var in credIn
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if in.Name != "" {
		cd.Name = in.Name
	}
	if in.Type != "" {
		cd.Type = model.CredType(in.Type)
	}
	if in.Username != "" {
		cd.Username = in.Username
	}
	if in.Description != "" {
		cd.Description = in.Description
	}
	if in.Enabled != nil {
		cd.Enabled = *in.Enabled
	}
	if in.Secret != "" {
		enc, err := h.crypt.Encrypt(in.Secret)
		if err != nil {
			BadRequest(c, err)
			return
		}
		cd.SecretEnc = enc
	}
	if in.Passphrase != "" {
		enc, err := h.crypt.Encrypt(in.Passphrase)
		if err != nil {
			BadRequest(c, err)
			return
		}
		cd.PassphraseEnc = enc
	}
	if err := h.tdb(c).Save(&cd).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, cd)
}

func (h *Handlers) DeleteCredential(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var n int64
	h.tdb(c).Model(&model.Repository{}).Where("credential_id = ?", id).Count(&n)
	if n > 0 {
		BadRequest(c, fmt.Sprintf("仍有 %d 个仓库引用该凭证，请先解除引用", n))
		return
	}
	if err := h.tdb(c).Delete(&model.Credential{}, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

func (h *Handlers) ListCredentialUsages(c *gin.Context) {
	var list []model.CredentialUsage
	q := h.tdb(c).Model(&model.CredentialUsage{}).Preload("Credential").Order("id DESC")
	if cid := c.Query("credential_id"); cid != "" {
		q = q.Where("credential_id = ?", cid)
	}
	var total int64
	q.Count(&total)
	page, size := QueryPage(c)
	if err := q.Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	OKPage(c, list, Page{Page: page, PageSize: size, Total: total})
}

func (h *Handlers) buildCredential(in *credIn) (*model.Credential, error) {
	if in.Name == "" || in.Type == "" {
		return nil, errors.New("名称与类型不能为空")
	}
	if in.Secret == "" {
		return nil, errors.New("密钥内容不能为空")
	}
	enc, err := h.crypt.Encrypt(in.Secret)
	if err != nil {
		return nil, err
	}
	cd := &model.Credential{
		Name: in.Name, Type: model.CredType(in.Type), Username: in.Username,
		SecretEnc: enc, Description: in.Description, Enabled: true,
	}
	if in.Passphrase != "" {
		p, err := h.crypt.Encrypt(in.Passphrase)
		if err != nil {
			return nil, err
		}
		cd.PassphraseEnc = p
	}
	if in.Enabled != nil {
		cd.Enabled = *in.Enabled
	}
	if in.Type == string(model.CredTypeSSHKey) && !strings.Contains(in.Secret, "PRIVATE KEY") {
		return nil, errors.New("SSH 私钥内容需包含 PRIVATE KEY 标记")
	}
	return cd, nil
}

func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", 8) + s[len(s)-4:]
}

var _ = crypto.SHA256
