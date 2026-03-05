<script setup lang="ts">
import { onMounted } from 'vue'
import { useVaultStore } from '../stores/vault'

const store = useVaultStore()

onMounted(() => {
  store.fetchStats()
  store.fetchVaults({ limit: 20, reset: true })
})

function formatDate(d: string) {
  if (!d || d === '0001-01-01T00:00:00Z') return '—'
  return new Date(d).toLocaleDateString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
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
</script>

<template>
  <div>
    <div class="page-header">
      <h1>Dashboard</h1>
      <p>Dead Man's Switch server overview</p>
    </div>

    <div class="stats-grid">
      <div class="stat-card">
        <span class="stat-label">Total Vaults</span>
        <span class="stat-value">{{ store.stats.total }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">Active</span>
        <span class="stat-value text-success">{{ store.stats.active }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">Released</span>
        <span class="stat-value text-warning">{{ store.stats.released }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">Keep-Alive Enabled</span>
        <span class="stat-value">{{ store.stats.keepAlive }}</span>
      </div>
    </div>

    <div class="card">
      <div class="card-header">
        <h2>Recent Activity</h2>
        <router-link to="/vaults" class="btn btn-ghost btn-sm">View All</router-link>
      </div>
      <div class="card-body" style="padding: 0;">
        <div v-if="store.loading" class="loading-center">
          <div class="spinner"></div>
        </div>
        <div v-else-if="store.vaults.length === 0" class="empty-state">
          <h3>No vaults registered</h3>
          <p>Add a vault to start managing dead man's switches.</p>
          <router-link to="/vaults" class="btn btn-primary mt-2" style="display: inline-flex;">Add Vault</router-link>
        </div>
        <div v-else class="table-wrapper">
          <table>
            <thead>
              <tr>
                <th>Vault ID</th>
                <th>Status</th>
                <th>Last Check-In</th>
                <th>Updated</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="vault in store.vaults" :key="vault.id"
                  class="clickable-row" @click="$router.push(`/vaults/${vault.id}`)">
                <td class="mono">{{ vault.id.substring(0, 8) }}…</td>
                <td>
                  <span v-if="vault.released" class="badge badge-warning">Released</span>
                  <span v-else-if="vault.status === 'pending'" class="badge badge-dim">Pending</span>
                  <span v-else class="badge badge-success">Active</span>
                </td>
                <td>{{ timeAgo(vault.lastCheckIn) }}</td>
                <td class="text-dim">{{ formatDate(vault.updatedAt) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>
