<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useVaultStore } from '../stores/vault'

const store = useVaultStore()
const router = useRouter()
const searchText = ref('')
const creating = ref(false)

onMounted(() => {
  store.fetchStats()
  store.fetchVaults({ limit: 20, reset: true })
})

function formatDate(d: string) {
  if (!d || d === '0001-01-01T00:00:00Z') return '—'
  return new Date(d).toLocaleDateString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric',
  })
}

function timeAgo(d: string) {
  if (!d || d === '0001-01-01T00:00:00Z') return 'Never'
  const diff = Date.now() - new Date(d).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'Just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

async function approveVault(id: string) {
  await store.approveVault(id)
  await store.fetchStats()
  await store.fetchVaults({ limit: 20, reset: true })
}

async function createPendingVault() {
  if (!searchText.value.trim()) return
  creating.value = true
  try {
    await store.createPendingVault(searchText.value.trim())
    searchText.value = ''
    await store.fetchStats()
    await store.fetchVaults({ limit: 20, reset: true })
  } finally {
    creating.value = false
  }
}

function searchVault() {
  const q = searchText.value.trim()
  if (!q) return
  router.push(`/vaults/${q}`)
}

function loadMore() {
  store.fetchVaults({ limit: 20, cursor: store.cursor })
}

function statusBadge(vault: any) {
  if (vault.released) return { class: 'badge-warning', text: 'Released' }
  if (vault.status === 'pending') return { class: 'badge-dim', text: 'Pending' }
  if (vault.status === 'active') return { class: 'badge-success', text: 'Active' }
  return { class: 'badge-danger', text: 'Inactive' }
}
</script>

<template>
  <div>
    <div class="page-header">
      <h1>Vaults</h1>
      <p>Manage registered dead man's switch vaults</p>
    </div>

    <!-- Toolbar: Search / Create -->
    <div class="vaults-toolbar">
      <div class="toolbar-input-group">
        <svg class="toolbar-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg>
        <input
          v-model="searchText"
          type="text"
          placeholder="Enter vault ID to search or create…"
          class="form-input toolbar-input"
          @keyup.enter="searchVault"
        />
      </div>
      <div class="toolbar-actions">
        <button class="btn btn-ghost" @click="searchVault" :disabled="!searchText.trim()">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg>
          Search
        </button>
        <button class="btn btn-primary" @click="createPendingVault" :disabled="creating || !searchText.trim()">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14M5 12h14"/></svg>
          {{ creating ? 'Creating…' : 'Create Vault' }}
        </button>
      </div>
    </div>

    <!-- Vault count -->
    <div class="vault-count" v-if="store.stats.total > 0">
      <span class="text-dim">{{ store.stats.total }} vault{{ store.stats.total === 1 ? '' : 's' }} total</span>
    </div>

    <div class="card">
      <div class="card-body" style="padding: 0;">
        <div v-if="store.loading && store.vaults.length === 0" class="loading-center">
          <div class="spinner"></div>
        </div>
        <div v-else-if="store.vaults.length === 0" class="empty-state">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0110 0v4"/></svg>
          <h3>No vaults registered</h3>
          <p>Create a pending vault above, then register from the desktop app.</p>
        </div>
        <div v-else class="table-wrapper">
          <table>
            <thead>
              <tr>
                <th>Vault ID</th>
                <th>Status</th>
                <th>Keep-Alive</th>
                <th>Credit Expiry</th>
                <th>Last Check-In</th>
                <th>Created</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="vault in store.vaults" :key="vault.id"
                  class="clickable-row" @click="router.push(`/vaults/${vault.id}`)">
                <td class="mono">{{ vault.id.substring(0, 8) }}…</td>
                <td>
                  <span :class="['badge', statusBadge(vault).class]">{{ statusBadge(vault).text }}</span>
                </td>
                <td>
                  <span v-if="vault.enableKeepAlive" class="badge badge-info">{{ vault.keepAliveDays }}d</span>
                  <span v-else class="text-muted">Off</span>
                </td>
                <td>
                  <span v-if="vault.releaseOnExpiry" class="badge badge-info">Enabled</span>
                  <span v-else class="text-muted">Off</span>
                </td>
                <td>{{ timeAgo(vault.lastCheckIn) }}</td>
                <td class="text-dim">{{ formatDate(vault.createdAt) }}</td>
                <td @click.stop>
                  <button v-if="vault.status === 'pending'" class="btn btn-success btn-sm" @click="approveVault(vault.id)">
                    Approve
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- View More -->
        <div v-if="store.hasMore" class="view-more-container">
          <button class="btn btn-ghost view-more-btn" @click="loadMore" :disabled="store.loading">
            <div v-if="store.loading" class="spinner" style="width: 14px; height: 14px;"></div>
            <span v-else>View More</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.vaults-toolbar {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 1rem;
  padding: 0.75rem 1rem;
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.toolbar-input-group {
  flex: 1;
  position: relative;
  display: flex;
  align-items: center;
}

.toolbar-icon {
  position: absolute;
  left: 0.75rem;
  width: 16px;
  height: 16px;
  color: var(--text-muted);
  pointer-events: none;
}

.toolbar-input {
  width: 100%;
  padding-left: 2.25rem !important;
  background: var(--bg-app) !important;
}

.toolbar-actions {
  display: flex;
  gap: 0.5rem;
  flex-shrink: 0;
}

.vault-count {
  margin-bottom: 0.75rem;
  font-size: 0.8rem;
}

.view-more-container {
  display: flex;
  justify-content: center;
  padding: 1rem;
  border-top: 1px solid var(--border);
}

.view-more-btn {
  min-width: 140px;
  justify-content: center;
}

.badge-dim {
  background: rgba(139, 143, 163, 0.15);
  color: var(--text-dim);
}
</style>
