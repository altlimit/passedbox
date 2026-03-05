<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useVaultStore } from '../stores/vault'

const store = useVaultStore()
const route = useRoute()
const router = useRouter()

const vaultId = route.params.id as string
const showAddCredits = ref(false)
const creditYears = ref(1)
const showDeleteConfirm = ref(false)
const showReleaseConfirm = ref(false)
const saving = ref(false)
const sendingPush = ref(false)

// Editable settings
const releaseOnExpiry = ref(false)
const enableKeepAlive = ref(false)
const keepAliveDays = ref(30)

onMounted(async () => {
  await store.fetchVault(vaultId)
  if (store.currentVault) {
    releaseOnExpiry.value = store.currentVault.releaseOnExpiry
    enableKeepAlive.value = store.currentVault.enableKeepAlive
    keepAliveDays.value = store.currentVault.keepAliveDays || 30
  }
})

function formatDate(d: string) {
  if (!d || d === '0001-01-01T00:00:00Z') return '—'
  return new Date(d).toLocaleDateString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}



async function saveSettings() {
  saving.value = true
  try {
    await store.updateVault(vaultId, {
      releaseOnExpiry: releaseOnExpiry.value,
      enableKeepAlive: enableKeepAlive.value,
      keepAliveDays: keepAliveDays.value,
    })
  } finally {
    saving.value = false
  }
}

async function handleAddCredits() {
  try {
    await store.addCredits(vaultId, creditYears.value)
    showAddCredits.value = false
    creditYears.value = 1
  } catch { /* handled by store */ }
}

async function handleCheckIn() {
  await store.manualCheckIn(vaultId)
}

async function handleSendKeepAlive() {
  sendingPush.value = true
  try {
    await store.sendTestPush(vaultId)
  } catch { /* handled by store */ }
  finally {
    sendingPush.value = false
  }
}

async function handleDelete() {
  try {
    await store.deleteVault(vaultId)
    router.push('/vaults')
  } catch { /* handled by store */ }
}

async function handleRelease() {
  try {
    await store.releaseVault(vaultId)
    showReleaseConfirm.value = false
  } catch { /* handled by store */ }
}
</script>

<template>
  <div>
    <div class="page-header">
      <router-link to="/vaults" class="btn btn-ghost btn-sm mb-2" style="display: inline-flex;">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="15 18 9 12 15 6"/></svg>
        Back to Vaults
      </router-link>
      <h1>Vault Details</h1>
      <p class="mono" style="margin-top: 0.25rem;">{{ vaultId }}</p>
    </div>

    <div v-if="store.loading" class="loading-center">
      <div class="spinner"></div>
    </div>

    <template v-else-if="store.currentVault">
      <!-- Status & Info -->
      <div class="card mb-2">
        <div class="card-header">
          <h2>Status</h2>
          <span v-if="store.currentVault.released" class="badge badge-warning">Released</span>
          <span v-else class="badge badge-success">Active</span>
        </div>
        <div class="card-body">
          <div class="detail-grid">
            <div class="detail-item">
              <span class="detail-label">Last Check-In</span>
              <span class="detail-value">{{ formatDate(store.currentVault.lastCheckIn) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">Credits</span>
              <span class="detail-value">{{ store.currentVault.credits || 0 }} year(s)</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">Release Date</span>
              <span class="detail-value" :class="store.currentVault.creditsActive ? 'text-success' : 'text-danger'">
                {{ formatDate(store.currentVault.releaseDate || '') }}
              </span>
            </div>
            <div class="detail-item">
              <span class="detail-label">Created</span>
              <span class="detail-value">{{ formatDate(store.currentVault.createdAt) }}</span>
            </div>
            <div class="detail-item" v-if="store.currentVault.released">
              <span class="detail-label">Released At</span>
              <span class="detail-value text-warning">{{ formatDate(store.currentVault.releasedAt) }}</span>
            </div>
          </div>
          <div class="mt-2" v-if="!store.currentVault.released" style="display: flex; gap: 0.5rem; flex-wrap: wrap;">
            <button class="btn btn-success btn-sm" @click="handleCheckIn">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>
              Manual Check-In
            </button>
            <button class="btn btn-sm" style="border: 1px solid var(--border); color: var(--text-dim);" @click="handleSendKeepAlive" :disabled="sendingPush">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>
              {{ sendingPush ? 'Sending…' : 'Send Keep Alive' }}
            </button>
          </div>
        </div>
      </div>

      <!-- Settings -->
      <div class="card mb-2">
        <div class="card-header">
          <h2>Dead Man's Switch Settings</h2>
        </div>
        <div class="card-body" style="display: flex; flex-direction: column; gap: 1rem;">
          <label style="display: flex; align-items: center; gap: 0.6rem; cursor: pointer;">
            <input type="checkbox" v-model="releaseOnExpiry" />
            <span>Release vault when credits expire</span>
          </label>
          <label style="display: flex; align-items: center; gap: 0.6rem; cursor: pointer;">
            <input type="checkbox" v-model="enableKeepAlive" />
            <span>Enable keep-alive check-in</span>
          </label>
          <div v-if="enableKeepAlive" class="form-group" style="max-width: 200px;">
            <label class="form-label">Keep-alive interval (days)</label>
            <input type="number" v-model.number="keepAliveDays" class="form-input" min="1" max="365" />
          </div>
          <button class="btn btn-primary" style="align-self: flex-start;" @click="saveSettings" :disabled="saving">
            {{ saving ? 'Saving…' : 'Save Settings' }}
          </button>
        </div>
      </div>

      <!-- Credits -->
      <div class="card mb-2">
        <div class="card-header">
          <h2>Credits</h2>
          <button class="btn btn-primary btn-sm" @click="showAddCredits = true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
            Add Credits
          </button>
        </div>
        <div class="card-body">
          <div class="detail-grid">
            <div class="detail-item">
              <span class="detail-label">Total Credits</span>
              <span class="detail-value">{{ store.currentVault.credits || 0 }} year(s)</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">Release Date</span>
              <span class="detail-value" :class="store.currentVault.creditsActive ? 'text-success' : 'text-danger'">
                {{ formatDate(store.currentVault.releaseDate || '') }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- Danger Zone -->
      <div class="card" style="border-color: var(--danger-dim);">
        <div class="card-header">
          <h2 class="text-danger">Danger Zone</h2>
        </div>
        <div class="card-body" style="display: flex; flex-direction: column; gap: 1rem;">
          <div v-if="!store.currentVault.released" class="flex-between">
            <div>
              <p style="font-size: 0.875rem; font-weight: 500;">Release Vault</p>
              <p style="font-size: 0.8rem; color: var(--text-dim);">Make the encrypted share recoverable by the vault owner. This cannot be undone.</p>
            </div>
            <button class="btn btn-danger" @click="showReleaseConfirm = true">Release Vault</button>
          </div>
          <div class="flex-between">
            <div>
              <p style="font-size: 0.875rem; font-weight: 500;">Delete Vault</p>
              <p style="font-size: 0.8rem; color: var(--text-dim);">Remove this vault from the server. This cannot be undone.</p>
            </div>
            <button class="btn btn-danger" @click="showDeleteConfirm = true">Delete Vault</button>
          </div>
        </div>
      </div>
    </template>

    <div v-else class="empty-state">
      <h3>Vault not found</h3>
      <p>The vault ID may be incorrect or it was deleted.</p>
    </div>

    <!-- Add Credits Modal -->
    <div v-if="showAddCredits" class="modal-overlay" @click.self="showAddCredits = false">
      <div class="modal">
        <div class="modal-header">Add Credits</div>
        <div class="modal-body">
          <div class="form-group">
            <label class="form-label">Years (1–30)</label>
            <input type="number" v-model.number="creditYears" class="form-input" min="1" max="30" />
          </div>
          <p class="text-dim" style="font-size: 0.85rem;">
            Cost: <strong>${{ (creditYears * 15).toFixed(2) }}</strong> ({{ creditYears }} × $15/year)
          </p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-ghost" @click="showAddCredits = false">Cancel</button>
          <button class="btn btn-primary" @click="handleAddCredits">Add Credits</button>
        </div>
      </div>
    </div>

    <!-- Release Confirm Modal -->
    <div v-if="showReleaseConfirm" class="modal-overlay" @click.self="showReleaseConfirm = false">
      <div class="modal">
        <div class="modal-header">Release Vault</div>
        <div class="modal-body">
          <p>Are you sure you want to release this vault? This will make the encrypted share available for recovery by the vault owner.</p>
          <p class="text-warning" style="font-size: 0.85rem; margin-top: 0.5rem;">⚠ This action cannot be undone.</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-ghost" @click="showReleaseConfirm = false">Cancel</button>
          <button class="btn btn-danger" @click="handleRelease">Release</button>
        </div>
      </div>
    </div>

    <!-- Delete Confirm Modal -->
    <div v-if="showDeleteConfirm" class="modal-overlay" @click.self="showDeleteConfirm = false">
      <div class="modal">
        <div class="modal-header">Delete Vault</div>
        <div class="modal-body">
          <p>Are you sure you want to delete this vault? This will permanently remove the encrypted share and all associated data.</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-ghost" @click="showDeleteConfirm = false">Cancel</button>
          <button class="btn btn-danger" @click="handleDelete">Delete</button>
        </div>
      </div>
    </div>
  </div>
</template>
