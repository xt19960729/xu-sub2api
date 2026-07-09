<template>
  <AppLayout>
    <div class="space-y-6 p-4 sm:p-6">
      <section class="space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">每日消费返现规则</h2>
          </div>
          <div class="flex items-center gap-2">
            <button class="btn btn-secondary" :disabled="rulesLoading" :title="t('common.refresh')" @click="loadRules">
              <Icon name="refresh" size="md" :class="rulesLoading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="openCreateRule">
              <Icon name="plus" size="md" class="mr-1" />
              新增规则
            </button>
          </div>
        </div>

        <DataTable :columns="ruleColumns" :data="rules" :loading="rulesLoading">
          <template #cell-enabled="{ value }">
            <span :class="['badge', value ? 'badge-success' : 'badge-gray']">
              {{ value ? '启用' : '停用' }}
            </span>
          </template>
          <template #cell-range="{ row }">
            <span class="font-mono text-sm text-gray-700 dark:text-gray-200">
              {{ formatRange(row) }}
            </span>
          </template>
          <template #cell-rate_percent="{ value }">
            <span class="font-medium text-gray-900 dark:text-white">{{ formatPercent(value) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button class="rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-300" :title="t('common.edit')" @click="openEditRule(row)">
                <Icon name="edit" size="sm" />
              </button>
              <button class="rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400" :title="t('common.delete')" @click="handleDeleteRule(row)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>
        </DataTable>
      </section>

      <section class="space-y-3">
        <div class="flex flex-wrap items-end gap-3">
          <div class="min-w-56 flex-1">
            <label class="input-label">搜索用户</label>
            <input v-model="recordSearch" class="input" placeholder="邮箱 / 用户名 / 用户 ID" @input="handleRecordSearch" />
          </div>
          <div class="w-44">
            <label class="input-label">记录日期</label>
            <input v-model="recordDate" type="date" class="input" @change="reloadRecords" />
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button class="btn btn-secondary" :disabled="recordsLoading" :title="t('common.refresh')" @click="reloadRecords">
              <Icon name="refresh" size="md" :class="recordsLoading ? 'animate-spin' : ''" />
            </button>
            <input v-model="runDate" type="date" class="input w-44" />
            <button class="btn btn-primary" :disabled="running || !runDate" @click="handleRun">
              <Icon name="play" size="md" class="mr-1" />
              {{ running ? '结算中' : '手动结算' }}
            </button>
          </div>
        </div>

        <DataTable :columns="recordColumns" :data="records" :loading="recordsLoading">
          <template #cell-user="{ row }">
            <div class="min-w-0">
              <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ row.user_email || row.username || `#${row.user_id}` }}</div>
              <div class="text-xs text-gray-500">ID {{ row.user_id }}</div>
            </div>
          </template>
          <template #cell-spend_amount="{ value }">
            <span class="font-mono">${{ formatMoney(value) }}</span>
          </template>
          <template #cell-rate_percent="{ value }">
            {{ formatPercent(value) }}
          </template>
          <template #cell-cashback_amount="{ value }">
            <span class="font-mono font-semibold text-emerald-600 dark:text-emerald-400">${{ formatMoney(value) }}</span>
          </template>
          <template #cell-balance_after="{ value }">
            <span class="font-mono">{{ value == null ? '-' : `$${formatMoney(value)}` }}</span>
          </template>
          <template #cell-applied_at="{ value }">
            <span class="text-sm text-gray-500">{{ formatDateTime(value) }}</span>
          </template>
        </DataTable>

        <Pagination
          v-if="recordPagination.total > 0"
          :page="recordPagination.page"
          :total="recordPagination.total"
          :page-size="recordPagination.page_size"
          @update:page="handleRecordPageChange"
          @update:pageSize="handleRecordPageSizeChange"
        />
      </section>
    </div>

    <BaseDialog :show="ruleDialogOpen" :title="editingRule ? '编辑返现规则' : '新增返现规则'" width="normal" @close="ruleDialogOpen = false">
      <form id="daily-cashback-rule-form" class="space-y-4" @submit.prevent="submitRule">
        <div>
          <label class="input-label">规则名称</label>
          <input v-model="ruleForm.name" class="input" placeholder="例如：高消费返现" />
        </div>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">消费下限</label>
            <input v-model.number="ruleForm.min_amount" type="number" min="0" step="0.00000001" class="input" required />
          </div>
          <div>
            <label class="input-label">消费上限</label>
            <input v-model="ruleForm.max_amount_text" type="number" min="0" step="0.00000001" class="input" placeholder="不填表示无上限" />
          </div>
        </div>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">返现比例 (%)</label>
            <input v-model.number="ruleForm.rate_percent" type="number" min="0.0001" max="100" step="0.0001" class="input" required />
          </div>
          <div>
            <label class="input-label">排序</label>
            <input v-model.number="ruleForm.sort_order" type="number" step="1" class="input" />
          </div>
        </div>
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input v-model="ruleForm.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          启用规则
        </label>
      </form>
      <template #footer>
        <button class="btn btn-secondary" type="button" @click="ruleDialogOpen = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" type="submit" form="daily-cashback-rule-form" :disabled="savingRule">
          {{ savingRule ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { DailyCashbackRecord, DailyCashbackRule } from '@/api/admin/dailyCashback'
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatDateTime } from '@/utils/format'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const rules = ref<DailyCashbackRule[]>([])
const records = ref<DailyCashbackRecord[]>([])
const rulesLoading = ref(false)
const recordsLoading = ref(false)
const savingRule = ref(false)
const running = ref(false)
const ruleDialogOpen = ref(false)
const editingRule = ref<DailyCashbackRule | null>(null)
const recordSearch = ref('')
const recordDate = ref('')
const runDate = ref(yesterdayString())
let searchTimer: ReturnType<typeof setTimeout> | null = null

const recordPagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
})

const ruleForm = reactive({
  name: '',
  enabled: true,
  min_amount: 0,
  max_amount_text: '',
  rate_percent: 10,
  sort_order: 0,
})

const ruleColumns = computed<Column[]>(() => [
  { key: 'enabled', label: '状态' },
  { key: 'name', label: '规则名称' },
  { key: 'range', label: '消费区间' },
  { key: 'rate_percent', label: '返现比例' },
  { key: 'sort_order', label: '排序' },
  { key: 'actions', label: t('common.actions') },
])

const recordColumns = computed<Column[]>(() => [
  { key: 'business_date', label: '日期' },
  { key: 'user', label: '用户' },
  { key: 'rule_name', label: '规则' },
  { key: 'spend_amount', label: '消费金额' },
  { key: 'rate_percent', label: '比例' },
  { key: 'cashback_amount', label: '返现金额' },
  { key: 'balance_after', label: '返现后余额' },
  { key: 'applied_at', label: '结算时间' },
])

function yesterdayString(): string {
  const d = new Date()
  d.setDate(d.getDate() - 1)
  return d.toISOString().slice(0, 10)
}

function formatMoney(value: number): string {
  return Number(value || 0).toFixed(4)
}

function formatPercent(value: number): string {
  return `${Number(value || 0).toFixed(2)}%`
}

function formatRange(rule: DailyCashbackRule): string {
  const min = `$${formatMoney(rule.min_amount)}`
  const max = rule.max_amount == null ? '∞' : `$${formatMoney(rule.max_amount)}`
  return `${min} <= x < ${max}`
}

function resetRuleForm(rule?: DailyCashbackRule) {
  editingRule.value = rule ?? null
  ruleForm.name = rule?.name ?? ''
  ruleForm.enabled = rule?.enabled ?? true
  ruleForm.min_amount = rule?.min_amount ?? 0
  ruleForm.max_amount_text = rule?.max_amount == null ? '' : String(rule.max_amount)
  ruleForm.rate_percent = rule?.rate_percent ?? 10
  ruleForm.sort_order = rule?.sort_order ?? rules.value.length
}

function openCreateRule() {
  resetRuleForm()
  ruleDialogOpen.value = true
}

function openEditRule(rule: DailyCashbackRule) {
  resetRuleForm(rule)
  ruleDialogOpen.value = true
}

async function loadRules() {
  rulesLoading.value = true
  try {
    rules.value = await adminAPI.dailyCashback.listRules()
  } catch (error) {
    console.error('Failed to load daily cashback rules', error)
    appStore.showError('加载返现规则失败')
  } finally {
    rulesLoading.value = false
  }
}

async function submitRule() {
  const maxText = String(ruleForm.max_amount_text || '').trim()
  const maxAmount = maxText === '' ? null : Number(maxText)
  const payload = {
    name: ruleForm.name.trim(),
    enabled: ruleForm.enabled,
    min_amount: Number(ruleForm.min_amount),
    max_amount: maxAmount,
    rate_percent: Number(ruleForm.rate_percent),
    sort_order: Number(ruleForm.sort_order || 0),
  }
  savingRule.value = true
  try {
    if (editingRule.value) {
      await adminAPI.dailyCashback.updateRule(editingRule.value.id, payload)
    } else {
      await adminAPI.dailyCashback.createRule(payload)
    }
    ruleDialogOpen.value = false
    appStore.showSuccess(t('common.saved'))
    await loadRules()
  } catch (error) {
    console.error('Failed to save daily cashback rule', error)
    appStore.showError('保存返现规则失败')
  } finally {
    savingRule.value = false
  }
}

async function handleDeleteRule(rule: DailyCashbackRule) {
  if (!window.confirm(`确认删除规则「${rule.name || rule.id}」？`)) return
  try {
    await adminAPI.dailyCashback.deleteRule(rule.id)
    appStore.showSuccess(t('common.deleted'))
    await loadRules()
  } catch (error) {
    console.error('Failed to delete daily cashback rule', error)
    appStore.showError('删除返现规则失败')
  }
}

async function loadRecords() {
  recordsLoading.value = true
  try {
    const resp = await adminAPI.dailyCashback.listRecords({
      page: recordPagination.page,
      page_size: recordPagination.page_size,
      search: recordSearch.value.trim(),
      business_date: recordDate.value,
    })
    records.value = resp.items
    recordPagination.total = resp.total
  } catch (error) {
    console.error('Failed to load daily cashback records', error)
    appStore.showError('加载返现记录失败')
  } finally {
    recordsLoading.value = false
  }
}

function reloadRecords() {
  recordPagination.page = 1
  void loadRecords()
}

function handleRecordSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(reloadRecords, 300)
}

function handleRecordPageChange(page: number) {
  recordPagination.page = page
  void loadRecords()
}

function handleRecordPageSizeChange(pageSize: number) {
  recordPagination.page_size = pageSize
  recordPagination.page = 1
  void loadRecords()
}

async function handleRun() {
  if (!runDate.value) return
  running.value = true
  try {
    const result = await adminAPI.dailyCashback.runForDate(runDate.value)
    appStore.showSuccess(`结算完成：新增 ${result.applied_users} 人，返现 $${formatMoney(result.total_cashback)}`)
    recordDate.value = result.business_date
    await Promise.all([loadRules(), loadRecords()])
  } catch (error) {
    console.error('Failed to run daily cashback', error)
    appStore.showError('手动结算失败')
  } finally {
    running.value = false
  }
}

onMounted(() => {
  void Promise.all([loadRules(), loadRecords()])
})
</script>
