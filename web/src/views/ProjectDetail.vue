<template>
  <div class="am-page" v-loading="loading">
    <div class="am-card">
      <div class="am-toolbar am-toolbar--tight">
        <el-button link type="primary" @click="$router.push('/projects')">← 返回</el-button>
        <div class="am-projhead">
          <h3 class="am-projhead__name">{{ detail.project?.name }}</h3>
          <div class="am-chips">
            <el-tag size="small">{{ detail.project?.key }}</el-tag>
            <el-tag size="small" :type="detail.project?.fix_mode === 'auto' ? 'danger' : 'warning'" effect="plain">
              {{ detail.project?.fix_mode === 'auto' ? '全自动' : '半自动' }}
            </el-tag>
            <el-tag size="small" effect="plain" :type="readyState.type">{{ readyState.text }}</el-tag>
          </div>
        </div>
        <div class="am-flex-1" />
        <el-button link :icon="'QuestionFilled'" @click="ingestGuide = true">接入说明</el-button>
        <el-button :icon="'Refresh'" aria-label="刷新" @click="load" />
      </div>

      <!-- 接入进度：点任意一步直接跳到对应配置区 -->
      <div class="am-stepbar">
        <button v-for="(s, i) in steps" :key="s.key" type="button"
          class="am-stepbar__item" :class="{ 'is-done': s.done, 'is-current': tab === s.key }" @click="goStep(s)">
          <span class="am-stepbar__no">
            <el-icon v-if="s.done" :size="12"><Check /></el-icon>
            <template v-else>{{ i + 1 }}</template>
          </span>
          <span class="am-stepbar__copy">
            <span class="am-stepbar__title">{{ s.title }}</span>
            <span class="am-stepbar__desc">{{ s.desc }}</span>
          </span>
        </button>
      </div>
    </div>

    <div v-if="nextHint" class="am-card am-next">
      <div>
        <div class="am-next__kicker">下一步</div>
        <div class="am-next__title">{{ nextHint.title }}</div>
        <div class="am-next__desc">{{ nextHint.desc }}</div>
      </div>
      <el-button type="primary" @click="nextHint.run">{{ nextHint.cta }}</el-button>
    </div>

    <el-tabs v-model="tab">
      <el-tab-pane label="仓库" name="repos">
        <div class="am-card">
          <div class="am-toolbar">
            <el-button type="primary" size="small" :icon="'Plus'" @click="openCreateRepo">关联仓库</el-button>
            <div class="am-flex-1" />
            <el-button size="small" :icon="'Key'" @click="$router.push('/credentials')">管理 git 凭证</el-button>
          </div>
          <div class="am-field-help" style="margin:-4px 0 12px">
            填地址和分支。私有仓先配 git 凭证，不确定时点「测试连通」。
          </div>
          <el-table :data="detail.repos || []" size="small">
            <template #empty>
              <div class="am-empty">
                <el-icon :size="30"><Coin /></el-icon>
                <div class="am-empty__title">还没有关联仓库</div>
                <div class="am-empty__desc">
                  私有仓请先到「凭证中心」添加 git 凭证，否则拉不到代码。
                </div>
                <div class="am-empty__actions">
                  <el-button type="primary" size="small" :icon="'Plus'" @click="openCreateRepo">关联仓库</el-button>
                  <el-button size="small" @click="$router.push('/credentials')">去凭证中心</el-button>
                </div>
              </div>
            </template>
            <el-table-column label="仓库" min-width="190">
              <template #default="{ row }">
                <div style="display:flex; flex-direction:column; gap:2px; align-items:flex-start">
                  <span>{{ row.name }}</span>
                  <span class="am-mono am-text-dim">{{ row.branch }}<template v-if="row.language"> · {{ row.language }}</template></span>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="url" label="地址" min-width="220" show-overflow-tooltip>
              <template #default="{ row }"><span class="am-mono">{{ row.url }}</span></template>
            </el-table-column>
            <el-table-column label="凭证" width="140">
              <template #default="{ row }">
                <el-tag size="small" effect="plain" :type="row.credential?.name ? 'info' : 'warning'">
                  {{ row.credential?.name || '未配置' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="模型" width="150">
              <template #default="{ row }">
                <div style="font-size:12px">
                  <div>修复：{{ row.model?.name || '继承项目' }}</div>
                  <div class="am-text-dim">审查：{{ row.review_model?.name || '同修复' }}</div>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="自动推送" width="90">
              <template #default="{ row }">{{ row.auto_push ? '是' : '否' }}</template>
            </el-table-column>
            <el-table-column label="操作" width="200" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="testConn(row)">测试连通</el-button>
                <el-button link type="primary" @click="openReview(row)">审查</el-button>
                <el-dropdown trigger="click" @command="cmd => repoCommand(cmd, row)">
                  <el-button link type="primary">更多<el-icon :size="12" style="margin-left:2px"><ArrowDown /></el-icon></el-button>
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item command="tree">目录树</el-dropdown-item>
                      <el-dropdown-item command="remove" divided>移除仓库</el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>

      <el-tab-pane label="修复规则" name="rules">
        <div class="am-card">
          <el-alert type="info" show-icon :closable="false" style="margin-bottom:12px"
            title="未命中规则的告警会被忽略，一条规则都没有就不会自动修复。" />
          <div class="am-toolbar">
            <el-button type="primary" size="small" :icon="'Plus'" @click="openCreateRule">新增规则</el-button>
            <el-dropdown trigger="click" @command="applyTemplate">
              <el-button size="small" :icon="'MagicStick'">从模板创建<el-icon :size="12" style="margin-left:2px"><ArrowDown /></el-icon></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item v-for="t in RULE_TEMPLATES" :key="t.name" :command="t.name">
                    {{ t.name }}<span class="am-text-dim" style="margin-left:6px">— {{ t.hint }}</span>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
          <el-table :data="detail.rules || []" size="small">
            <template #empty>
              <div class="am-empty">
                <el-icon :size="30"><Filter /></el-icon>
                <div class="am-empty__title">还没有修复规则</div>
                <div class="am-empty__desc">
                  没有规则就不会修。可先用模板建一条，跑通后再收紧关键字。
                </div>
                <div class="am-empty__actions">
                  <el-button type="primary" size="small" :icon="'MagicStick'" @click="applyTemplate(RULE_TEMPLATES[0].name)">用模板建一条</el-button>
                  <el-button size="small" :icon="'Plus'" @click="openCreateRule">从空白新建</el-button>
                </div>
              </div>
            </template>
            <el-table-column prop="name" label="规则名称" width="180" />
            <el-table-column prop="priority" label="优先级" width="80" />
            <el-table-column label="级别" width="120">
              <template #default="{ row }">{{ row.levels || '不限' }}</template>
            </el-table-column>
            <el-table-column label="命中关键字" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">{{ row.keywords || '不限' }}</template>
            </el-table-column>
            <el-table-column label="排除关键字" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">{{ row.exclude_keywords || '-' }}</template>
            </el-table-column>
            <el-table-column label="动作" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="row.action === 'fix' ? 'success' : 'info'">
                  {{ row.action === 'fix' ? '修复' : '忽略' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="80">
              <template #default="{ row }">{{ row.enabled ? '是' : '否' }}</template>
            </el-table-column>
            <el-table-column label="操作" width="140">
              <template #default="{ row }">
                <el-button link type="primary" @click="editRule(row)">编辑</el-button>
                <el-button link type="danger" @click="removeRule(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>

      <el-tab-pane label="投递令牌" name="tokens">
        <div class="am-card">
          <div class="am-toolbar">
            <el-button type="primary" size="small" :icon="'Plus'" @click="createProjectToken">新建令牌</el-button>
            <div class="am-flex-1" />
            <el-button size="small" :icon="'QuestionFilled'" @click="ingestGuide = true">怎么把告警送进来</el-button>
          </div>
          <div class="am-field-help" style="margin:-4px 0 12px">
            请求头 <span class="am-mono">X-AM-Token</span>。明文只显示一次。
          </div>
          <el-table :data="detail.tokens || []" size="small">
            <template #empty>
              <div class="am-empty">
                <el-icon :size="30"><Ticket /></el-icon>
                <div class="am-empty__title">还没有投递令牌</div>
                <div class="am-empty__desc">
                  告警源带着令牌 POST 进来，平台才知道事件属于本项目。
                </div>
                <div class="am-empty__actions">
                  <el-button type="primary" size="small" :icon="'Plus'" @click="createProjectToken">新建令牌</el-button>
                  <el-button size="small" @click="ingestGuide = true">查看接入示例</el-button>
                </div>
              </div>
            </template>
            <el-table-column prop="name" label="名称" width="180" />
            <el-table-column prop="prefix" label="前缀" width="140" />
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '禁用' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="最近使用" width="170">
              <template #default="{ row }">
                <span :class="{ 'am-text-dim': !row.last_used_at }">{{ row.last_used_at ? formatTime(row.last_used_at) : '从未使用' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="180">
              <template #default="{ row }">
                <el-button link type="primary" @click="toggleToken(row)">{{ row.enabled ? '禁用' : '启用' }}</el-button>
                <el-button link type="danger" @click="removeToken(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>

      <el-tab-pane label="项目设置" name="settings">
        <div class="am-card">
          <el-form :model="form" label-width="120px">
            <div class="am-section">
              <div class="am-section__title">基础信息</div>
              <div class="am-section__desc">名称用于展示。标识决定工作区目录，创建后不要改。</div>
              <el-form-item label="项目名称"><el-input v-model="form.name" /></el-form-item>
              <el-form-item label="修复模式">
                <el-radio-group v-model="form.fix_mode" class="am-seg">
                  <el-radio-button value="semi">半自动</el-radio-button>
                  <el-radio-button value="auto">全自动</el-radio-button>
                </el-radio-group>
                <div class="am-field-help">半自动需在「修复任务」确认后再推送；全自动直接提交。</div>
              </el-form-item>
              <el-form-item label="启用">
                <el-switch v-model="form.enabled" />
                <div class="am-field-help">关闭后不再自动修复该项目的告警。</div>
              </el-form-item>
            </div>

            <div class="am-section">
              <div class="am-section__title">模型</div>
              <div class="am-section__desc">留空则跟随全局默认。优先级：仓库 &gt; 项目 &gt; 全局。</div>
              <el-form-item label="默认修复模型">
                <el-select v-model="form.default_model_id" clearable style="width:320px" placeholder="留空用全局默认">
                  <el-option v-for="m in models" :key="m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
                </el-select>
              </el-form-item>
              <el-form-item label="默认审查模型">
                <el-select v-model="form.default_review_model_id" clearable style="width:320px" placeholder="留空则与修复模型同一套">
                  <el-option v-for="m in models" :key="'r'+m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
                </el-select>
              </el-form-item>
            </div>

            <div class="am-section">
              <div class="am-section__title">业务上下文</div>
              <div class="am-section__desc">写入工作区指令，帮模型定位代码。越具体越好。</div>
              <el-form-item label="业务上下文">
                <el-input v-model="form.context" type="textarea" :rows="6"
                  placeholder="例：订单系统，Go + MySQL；下单核心链路 internal/order/service.go；金额统一用 int64 分；不允许修改 internal/sdk 目录" />
              </el-form-item>
            </div>

            <div class="am-section">
              <div class="am-section__title">高级</div>
              <div class="am-section__desc">推送成功后在仓库目录执行的命令，覆盖系统设置 git.release_hook。</div>
              <el-form-item label="发布钩子">
                <el-input v-model="form.release_hook" placeholder="留空则用系统设置" />
              </el-form-item>
              <el-form-item label="描述"><el-input v-model="form.description" /></el-form-item>
            </div>

            <el-form-item>
              <el-button type="primary" @click="saveProject">保存</el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 仓库对话框 -->
    <el-dialog v-model="repoDialog" title="关联仓库" class="am-dialog-form" width="620px">
      <el-form :model="repoForm" label-width="110px">
        <el-form-item label="仓库名称"><el-input v-model="repoForm.name" placeholder="与代码仓库同名的展示名" /></el-form-item>
        <el-form-item label="仓库地址">
          <el-input v-model="repoForm.url" placeholder="git@github.com:org/repo.git 或 https://..." />
          <div class="am-field-help">平台会克隆这个地址到隔离工作区；私有仓库必须在下面选一个 git 凭证，否则拉不下来。</div>
        </el-form-item>
        <el-form-item label="分支">
          <el-select v-model="repoForm.branch" filterable allow-create default-first-option style="width:100%" placeholder="main">
            <el-option v-for="b in dict.values('git_branch')" :key="b" :label="b" :value="b" />
          </el-select>
          <div class="am-field-help">修复基于这个分支进行，推送也回到这个分支。</div>
        </el-form-item>
        <el-form-item label="主要语言">
          <el-select v-model="repoForm.language" filterable allow-create default-first-option clearable style="width:100%">
            <el-option v-for="l in dict.values('language')" :key="l" :label="l" :value="l" />
          </el-select>
        </el-form-item>
        <el-form-item label="关注路径">
          <el-select v-model="repoForm.code_paths" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="勾选目录前缀，可输入后回车">
            <el-option v-for="p in dict.values('code_path')" :key="p" :label="p" :value="p" />
          </el-select>
          <div class="am-field-help">只在这些目录里找代码，能显著缩短定位时间；留空则扫描整个仓库。</div>
        </el-form-item>
        <el-form-item label="git 凭证">
          <el-select v-model="repoForm.credential_id" clearable style="width:100%">
            <el-option v-for="c in credentials" :key="c.id" :label="`${c.name}（${c.type}）`" :value="c.id" />
          </el-select>
          <div class="am-field-help">没有可选项？先到「凭证中心」添加。</div>
        </el-form-item>
        <el-form-item label="修复模型">
          <el-select v-model="repoForm.model_id" clearable style="width:100%" placeholder="留空继承项目配置">
            <el-option v-for="m in models" :key="m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="审查模型">
          <el-select v-model="repoForm.review_model_id" clearable style="width:100%" placeholder="留空则与修复模型同一套">
            <el-option v-for="m in models" :key="'r'+m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="自动推送">
          <el-switch v-model="repoForm.auto_push" />
          <div class="am-field-help">关闭则只在本工作区提交，不推到远端；想先本地看效果就关掉。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="repoDialog = false">取消</el-button>
        <el-button type="primary" @click="saveRepo">保存</el-button>
      </template>
    </el-dialog>

    <!-- 规则对话框 -->
    <el-dialog v-model="ruleDialog" :title="ruleForm.id ? '编辑规则' : '新增规则'" class="am-dialog-form" width="720px">
      <el-form :model="ruleForm" label-width="120px">
        <el-form-item label="规则名称"><el-input v-model="ruleForm.name" placeholder="例如：Java 异常兜底" /></el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="ruleForm.priority" :min="0" :max="999" />
          <div class="am-field-help">数字越大越先匹配；命中第一条适用规则后不再继续。</div>
        </el-form-item>
        <el-form-item label="日志级别">
          <el-select v-model="ruleForm.levels" multiple clearable collapse-tags style="width:100%" placeholder="留空不限">
            <el-option v-for="o in dict.options('log_level')" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="命中关键字">
          <el-select v-model="ruleForm.keywords" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="勾选或输入后回车">
            <el-option v-for="k in dict.values('hit_keyword')" :key="k" :label="k" :value="k" />
          </el-select>
          <div class="am-field-help">告警的 title / message / stack 命中任意一个即算命中；留空表示不限关键字。</div>
        </el-form-item>
        <el-form-item label="排除关键字">
          <el-select v-model="ruleForm.exclude_keywords" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%">
            <el-option v-for="k in dict.values('exclude_keyword')" :key="k" :label="k" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="来源白名单">
          <el-select v-model="ruleForm.sources" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="留空不限">
            <el-option v-for="s in dict.values('event_source')" :key="s" :label="s" :value="s" />
          </el-select>
        </el-form-item>
        <el-form-item label="排除来源">
          <el-select v-model="ruleForm.exclude_sources" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%">
            <el-option v-for="s in dict.values('event_source')" :key="'ex-'+s" :label="s" :value="s" />
          </el-select>
        </el-form-item>
        <el-form-item label="频次阈值">
          <el-input-number v-model="ruleForm.min_count" :min="1" /> 次 /
          <el-select v-model="ruleForm.window_sec" style="width:140px">
            <el-option v-for="o in dict.numberOptions('window_sec')" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
          <div class="am-field-help">时间窗内累计出现够次数才触发，避免偶发单条告警就开工。</div>
        </el-form-item>
        <el-form-item label="冷却时间">
          <el-select v-model="ruleForm.cooldown_sec" style="width:160px">
            <el-option v-for="o in dict.numberOptions('cooldown_sec')" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
          <div class="am-field-help">触发后这段时间内不再为同类告警开新任务，防止刷任务。</div>
        </el-form-item>
        <el-form-item label="动作">
          <el-radio-group v-model="ruleForm.action">
            <el-radio value="fix">触发修复</el-radio>
            <el-radio value="ignore">忽略</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="修复模式">
          <el-radio-group v-model="ruleForm.fix_mode">
            <el-radio value="">继承项目</el-radio>
            <el-radio value="auto">全自动</el-radio>
            <el-radio value="semi">半自动</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="附加要求">
          <el-input v-model="ruleForm.prompt_template" type="textarea" :rows="3"
            placeholder="追加到 dsh 任务的额外约束，如：必须补充单元测试" />
        </el-form-item>
        <el-form-item label="最大重试"><el-input-number v-model="ruleForm.max_retries" :min="0" :max="10" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="ruleForm.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialog = false">取消</el-button>
        <el-button type="primary" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>

    <!-- 令牌明文：只出现一次，给复制按钮而不是纯文本弹窗 -->
    <el-dialog v-model="tokenDialog" title="令牌已创建" width="640">
      <el-alert type="warning" show-icon :closable="false" title="明文只显示这一次，请立即复制保存；关闭后只能重新创建" />
      <div style="margin-top:12px">
        <div class="am-code__head">
          <span>投递令牌</span>
          <el-button link type="primary" :icon="'DocumentCopy'" @click="copy(tokenPlain, '令牌')">复制</el-button>
        </div>
        <pre class="am-code">{{ tokenPlain }}</pre>
      </div>
      <template #footer>
        <el-button @click="tokenDialog = false">我已保存</el-button>
        <el-button type="primary" @click="ingestGuide = true">查看接入示例</el-button>
      </template>
    </el-dialog>

    <!-- 接入告警源：新用户最容易卡住的一步 -->
    <el-dialog v-model="ingestGuide" title="如何把告警送进这个项目" width="720">
      <ol class="am-guidelist">
        <li>在「投递令牌」建一个令牌，请求头用 <span class="am-mono">X-AM-Token</span> 带上它。</li>
        <li>让告警源（Sentry Webhook / Grafana Alerting / 自研脚本）POST 到下面的投递接口。</li>
        <li>到「事件中心」确认收到；命中规则的事件会自动进入「修复任务」。</li>
      </ol>
      <div style="margin-top:14px">
        <div class="am-code__head">
          <span>投递接口（单条；批量把路径换成 /events/batch，一次最多 100 条）</span>
          <el-button link type="primary" :icon="'DocumentCopy'" @click="copy(ingestCurl, '示例')">复制</el-button>
        </div>
        <pre class="am-code">{{ ingestCurl }}</pre>
      </div>
      <div class="am-field-help">
        <span class="am-mono">title</span> 与 <span class="am-mono">message</span> 至少填一个；<span class="am-mono">level</span> 默认 error（fatal / error / warn / info）；
        <span class="am-mono">fingerprint</span> 相同的告警会合并为一条事件，处理中或已修复的不会重复开工；
        <span class="am-mono">repo_hint</span> 填仓库名可缩小定位范围。
      </div>
      <template #footer>
        <el-button @click="ingestGuide = false">关闭</el-button>
        <el-button type="primary" @click="$router.push('/events')">去事件中心验证</el-button>
      </template>
    </el-dialog>

    <RepoTreeDrawer v-model="treeDrawer" :repo="treeRepo" />
    <RepoReviewDrawer v-model="reviewDrawer" :repo="reviewRepo" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  getProject, updateProject, createRepo, deleteRepo, testRepo,
  createRule, updateRule, deleteRule, createToken, updateToken, deleteToken,
  listModels, listCredentials
} from '@/api'
import { formatTime } from '@/utils/format'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useDicts, splitCSV, joinCSV } from '@/composables/useDicts'
import RepoTreeDrawer from './RepoTreeDrawer.vue'
import RepoReviewDrawer from './RepoReviewDrawer.vue'

const dict = useDicts()
const route = useRoute()
const id = ref(route.params.id)
const detail = ref({})
const loading = ref(false)
const tab = ref('repos')
const models = ref([])
const credentials = ref([])

const repoDialog = ref(false)
const treeDrawer = ref(false)
const treeRepo = ref(null)
const reviewDrawer = ref(false)
const reviewRepo = ref(null)
const ruleDialog = ref(false)
const tokenDialog = ref(false)
const tokenPlain = ref('')
const ingestGuide = ref(false)
const repoForm = ref({ auto_push: true, branch: 'main', code_paths: [] })
const ruleForm = ref({
  enabled: true, action: 'fix', priority: 0, min_count: 1, window_sec: 300, cooldown_sec: 600,
  max_retries: 2, fix_mode: '', levels: ['fatal', 'error'], keywords: [], exclude_keywords: [],
  sources: [], exclude_sources: []
})
const form = ref({})

const repos = computed(() => detail.value.repos || [])
const rules = computed(() => detail.value.rules || [])
const tokens = computed(() => detail.value.tokens || [])

// 接入进度：前三步对应 tab，最后一步是外部动作（打开接入指引）
const steps = computed(() => [
  {
    key: 'repos', title: '关联仓库', done: repos.value.length > 0,
    desc: repos.value.length ? `已关联 ${repos.value.length} 个` : '挂代码仓库'
  },
  {
    key: 'rules', title: '配置规则', done: rules.value.length > 0,
    desc: rules.value.length ? `${rules.value.length} 条规则` : '决定哪些告警会修'
  },
  {
    key: 'tokens', title: '生成令牌', done: tokens.value.length > 0,
    desc: tokens.value.length ? `${tokens.value.length} 个可用` : '告警源鉴权'
  },
  {
    key: 'ingest', title: '接入告警源', done: tokens.value.length > 0,
    desc: tokens.value.length ? '用令牌 POST 进来' : 'Sentry / Loki 投递'
  }
])

const readyState = computed(() => {
  if (!repos.value.length) return { type: 'warning', text: '待关联仓库' }
  if (!rules.value.length) return { type: 'warning', text: '待配置规则' }
  if (!tokens.value.length) return { type: 'warning', text: '待生成令牌' }
  return { type: 'success', text: '已就绪' }
})

function goStep(s) {
  if (s.key === 'ingest') {
    ingestGuide.value = true
    return
  }
  tab.value = s.key
}

const ingestCurl = computed(() => {
  const token = tokenPlain.value || 'am_xxxxxxxx（在「投递令牌」页创建）'
  const hint = detail.value.project?.key || 'shop-api'
  return `curl -X POST ${location.origin}/api/v1/ingest/events \\
  -H "Content-Type: application/json" \\
  -H "X-AM-Token: ${token}" \\
  -d '{
    "source": "sentry",
    "level": "fatal",
    "title": "TypeError: Cannot read properties of undefined (reading id)",
    "message": "order service checkout",
    "stack": "TypeError: ...\\n    at Service.checkout (internal/order/service.go:128)",
    "repo_hint": "${hint}"
  }'`
})

async function copy(text, label) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(label + '已复制')
  } catch {
    ElMessage.warning('浏览器不允许写入剪贴板，请手动选中复制')
  }
}

function repoCommand(cmd, row) {
  if (cmd === 'tree') openTree(row)
  else if (cmd === 'remove') removeRepo(row)
}

function defaultRule() {
  return {
    enabled: true, action: 'fix', priority: 0, min_count: 1, window_sec: 300, cooldown_sec: 600,
    max_retries: 2, fix_mode: '', levels: ['fatal', 'error'], keywords: [], exclude_keywords: [],
    sources: [], exclude_sources: []
  }
}

// 首次配置规则最容易卡在「关键字填什么」，给几条可直接改的预设
const RULE_TEMPLATES = [
  {
    name: 'Java 异常兜底', hint: 'NPE / 越界等常见异常',
    rule: { keywords: ['NullPointerException', 'IndexOutOfBoundsException', 'IllegalStateException'], levels: ['fatal', 'error'] }
  },
  {
    name: 'Go panic 兜底', hint: 'panic / nil pointer',
    rule: { keywords: ['panic:', 'runtime error', 'nil pointer dereference'], levels: ['fatal', 'error'] }
  },
  {
    name: '前端 JS 错误', hint: 'TypeError / Uncaught',
    rule: { keywords: ['TypeError', 'ReferenceError', 'Uncaught'], levels: ['fatal', 'error'] }
  },
  {
    name: '全量 error 告警', hint: '不限关键字，先看效果',
    rule: { keywords: [], levels: ['fatal', 'error'] }
  },
  {
    name: '噪音只记录不修复', hint: '超时 / 连接类直接忽略',
    rule: { keywords: ['Timeout', 'Connection refused', 'context deadline exceeded'], levels: ['error', 'warn'], action: 'ignore' }
  }
]

function applyTemplate(name) {
  const t = RULE_TEMPLATES.find(x => x.name === name)
  if (!t) return
  // 只预填、不直接保存，用户确认后再落库
  ruleForm.value = { ...defaultRule(), name: t.name, ...t.rule }
  ruleDialog.value = true
}

const nextHint = computed(() => {
  if (!repos.value.length) {
    return {
      title: '先关联代码仓库',
      desc: '没有仓库就无法改代码。私有仓先去凭证中心加 git 凭证。',
      cta: '关联仓库',
      run: () => { tab.value = 'repos'; openCreateRepo() }
    }
  }
  if (!rules.value.length) {
    return {
      title: '再配一条修复规则',
      desc: '未命中规则的事件会被忽略。可先用模板建一条。',
      cta: '用模板建规则',
      run: () => { tab.value = 'rules'; applyTemplate(RULE_TEMPLATES[0].name) }
    }
  }
  if (!tokens.value.length) {
    return {
      title: '生成投递令牌',
      desc: '告警源带令牌 POST 进来。明文只显示一次。',
      cta: '新建令牌',
      run: () => { tab.value = 'tokens'; createProjectToken() }
    }
  }
  if (!tokens.value.some(t => t.last_used_at)) {
    return {
      title: '把告警送进这个项目',
      desc: 'POST 到 /api/v1/ingest/events。送到后在「事件中心」确认。',
      cta: '查看接入示例',
      run: () => { ingestGuide.value = true }
    }
  }
  return null
})

async function load() {
  loading.value = true
  try {
    const r = await getProject(id.value)
    detail.value = r.data || {}
    form.value = { ...(detail.value.project || {}) }
  } finally {
    loading.value = false
  }
}

async function saveRepo() {
  const f = repoForm.value
  await createRepo({
    project_id: Number(id.value), name: f.name, url: f.url, branch: f.branch || 'main',
    language: f.language || '', code_paths: Array.isArray(f.code_paths) ? joinCSV(f.code_paths) : (f.code_paths || ''),
    credential_id: f.credential_id || null, model_id: f.model_id || null,
    review_model_id: f.review_model_id || null, auto_push: f.auto_push !== false
  })
  ElMessage.success('已关联')
  repoDialog.value = false
  load()
}

function openTree(row) {
  treeRepo.value = row
  treeDrawer.value = true
}
function openReview(row) {
  reviewRepo.value = row
  reviewDrawer.value = true
}

async function testConn(row) {
  try {
    await testRepo(row.id)
    ElMessage.success('连通性正常')
  } catch { /* 错误已在拦截器提示 */ }
}

async function removeRepo(row) {
  const ok = await ElMessageBox.confirm(`确认移除仓库「${row.name}」？`, '警告', { type: 'warning' }).catch(() => false)
  if (!ok) return
  await deleteRepo(row.id)
  load()
}

function openCreateRepo() {
  repoForm.value = { auto_push: true, branch: 'main', code_paths: [], language: '' }
  repoDialog.value = true
}

function openCreateRule() {
  ruleForm.value = defaultRule()
  ruleDialog.value = true
}

function editRule(row) {
  ruleForm.value = {
    id: row.id, name: row.name, enabled: row.enabled, priority: row.priority,
    levels: splitCSV(row.levels), sources: splitCSV(row.sources), keywords: splitCSV(row.keywords),
    exclude_keywords: splitCSV(row.exclude_keywords), exclude_sources: splitCSV(row.exclude_sources),
    min_count: row.min_count, window_sec: row.window_sec, cooldown_sec: row.cooldown_sec,
    action: row.action, repo_ids: row.repo_ids, model_id: row.model_id, fix_mode: row.fix_mode || '',
    max_retries: row.max_retries, prompt_template: row.prompt_template, description: row.description
  }
  ruleDialog.value = true
}

async function saveRule() {
  const f = ruleForm.value
  const payload = {
    project_id: Number(id.value), name: f.name, enabled: f.enabled !== false, priority: f.priority || 0,
    levels: joinCSV(f.levels), sources: joinCSV(f.sources), keywords: joinCSV(f.keywords),
    all_keywords: '', exclude_keywords: joinCSV(f.exclude_keywords),
    exclude_sources: joinCSV(f.exclude_sources), pattern: '',
    min_count: f.min_count, window_sec: f.window_sec, cooldown_sec: f.cooldown_sec,
    action: f.action || 'fix', repo_ids: f.repo_ids || '', model_id: f.model_id || null,
    fix_mode: f.fix_mode || '', max_retries: f.max_retries,
    prompt_template: f.prompt_template || '', description: f.description || ''
  }
  if (f.id) await updateRule(f.id, payload)
  else await createRule(payload)
  ElMessage.success('已保存')
  ruleDialog.value = false
  load()
}

async function removeRule(row) {
  const ok = await ElMessageBox.confirm(`确认删除规则「${row.name}」？`, '警告', { type: 'warning' }).catch(() => false)
  if (!ok) return
  await deleteRule(row.id)
  load()
}

async function createProjectToken() {
  const r = await createToken({ project_id: Number(id.value), name: 'collector-' + Date.now().toString().slice(-6) })
  // 明文只出现一次：用可复制的面板代替纯文本弹窗，顺带给出接入示例入口
  tokenPlain.value = r.data.plain_token
  tokenDialog.value = true
  load()
}

async function toggleToken(row) {
  await updateToken(row.id, { enabled: !row.enabled })
  load()
}

async function removeToken(row) {
  const ok = await ElMessageBox.confirm('确认删除该令牌？', '警告', { type: 'warning' }).catch(() => false)
  if (!ok) return
  await deleteToken(row.id)
  load()
}

async function saveProject() {
  const f = form.value
  await updateProject(id.value, {
    name: f.name, description: f.description || '', fix_mode: f.fix_mode,
    default_model_id: f.default_model_id || null,
    default_review_model_id: f.default_review_model_id || null,
    release_hook: f.release_hook || '', enabled: f.enabled !== false, context: f.context || ''
  })
  ElMessage.success('已保存')
  load()
}

watch(() => route.params.id, (v) => { id.value = v; load() })

onMounted(async () => {
  const [m, c] = await Promise.all([listModels(), listCredentials(), dict.load()])
  models.value = m.data || []
  credentials.value = c.data || []
  load()
})
</script>
