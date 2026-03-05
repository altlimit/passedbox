<script setup lang="ts">
import { ArrowRight, Folder, FolderOpen, Loader2, Lock, Plus, Shield, Unlock } from 'lucide-vue-next'
import { nextTick, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { CheckDMSRelease, CreateVault, GetDevicePepperInfo, ListVaults, UnlockVault, UnlockVaultWithShare3 } from '../../bindings/passedbox/vaultmanager.js'

const router = useRouter()
const vaults = ref<{name: string, path: string}[]>([])
const showCreateModal = ref(false)
const showUnlockModal = ref(false)
const newVaultName = ref('')
const newVaultPassword = ref('')
const newVaultUseDevicePepper = ref(false)
const devicePepperInfo = ref<{available: boolean, serialId: string, isRemovable: boolean} | null>(null)
const unlockPassword = ref('')
const selectedVault = ref<{name: string, path: string} | null>(null)
const error = ref('')
const isLoading = ref(false)

// DMS recovery state
const dmsStatus = ref<{enabled: boolean, released: boolean, releasedAt: string} | null>(null)
const dmsChecking = ref(false)
const dmsRecovering = ref(false)

// Refs for inputs to auto-focus
const vaultNameInput = ref<HTMLInputElement | null>(null)
const unlockPasswordInput = ref<HTMLInputElement | null>(null)
const passwordInput = ref<HTMLInputElement | null>(null)

const fetchVaults = async () => {
  isLoading.value = true
  try {
    const result = await ListVaults()
    vaults.value = result || []
  } catch (e) {
    console.error(e)
    error.value = "Failed to load vaults"
  }
  isLoading.value = false
}

const openCreateModal = async () => {
  showCreateModal.value = true
  newVaultName.value = ''
  newVaultPassword.value = ''
  newVaultUseDevicePepper.value = false
  error.value = ''
  
  try {
    devicePepperInfo.value = await GetDevicePepperInfo()
  } catch (e) {
    console.error("failed to get device info", e)
  }
  
  // Auto-focus vault name input
  nextTick(() => {
    vaultNameInput.value?.focus()
  })
}

const handleCreate = async () => {
  error.value = ''
  if (!newVaultName.value || !newVaultPassword.value) {
    error.value = "Name and password are required"
    return
  }
  
  isLoading.value = true
  try {
    await CreateVault(newVaultName.value, newVaultPassword.value, newVaultUseDevicePepper.value)
    showCreateModal.value = false
    await fetchVaults()
  } catch (e) {
    error.value = (e as Error).message || "Failed to create vault"
    nextTick(() => {
      passwordInput.value?.focus()
      passwordInput.value?.select()
    })
  }
  isLoading.value = false
}

const openUnlockModal = async (vault: {name: string, path: string}) => {
  selectedVault.value = vault
  showUnlockModal.value = true
  unlockPassword.value = ''
  error.value = ''
  dmsStatus.value = null
  dmsRecovering.value = false
  
  // Auto-focus password input
  nextTick(() => {
    unlockPasswordInput.value?.focus()
  })

  // Check DMS release status in the background
  dmsChecking.value = true
  try {
    const status = await CheckDMSRelease(vault.name)
    dmsStatus.value = status
  } catch (e) {
    // Silently ignore — DMS might not be configured
    console.error('DMS check failed:', e)
  }
  dmsChecking.value = false
}

const handleRecoverVault = async () => {
  if (!selectedVault.value) return
  dmsRecovering.value = true
  error.value = ''
  try {
    await UnlockVaultWithShare3(selectedVault.value.name)
    router.push({ name: 'VaultExplorer', params: { path: '' } })
  } catch (e) {
    error.value = (e as Error).message || 'Failed to recover vault'
  }
  dmsRecovering.value = false
}

const handleUnlock = async () => {
  if (!unlockPassword.value || !selectedVault.value) return
  
  isLoading.value = true
  try {
    await UnlockVault(selectedVault.value.name, unlockPassword.value)
    router.push({ name: 'VaultExplorer', params: { path: '' } })
  } catch (e) {
    error.value = (e as Error).message || "Failed to unlock vault"
    nextTick(() => {
      unlockPasswordInput.value?.focus()
      unlockPasswordInput.value?.select()
    })
  }
  isLoading.value = false
}

onMounted(() => {
  fetchVaults()
})
</script>

<template>
  <div class="container">
    <header class="flex items-center justify-between mb-8">
      <div class="logo">
        <h1 class="flex items-center gap-2">
          <Lock class="text-primary" /> PassedBox
        </h1>
        <p>Secure Vault Storage</p>
      </div>
      <button @click="openCreateModal" class="btn-primary">
        <Plus :size="18" /> New Vault
      </button>
    </header>

    <div v-if="isLoading && vaults.length === 0" class="loading-state">
      <Loader2 class="animate-spin" :size="32" />
      <p>Loading vaults...</p>
    </div>

    <div v-else-if="vaults.length === 0" class="empty-state">
        <div class="empty-icon">
          <FolderOpen :size="64" />
        </div>
        <h3>No vaults found</h3>
        <p>Create your first encrypted vault to get started.</p>
        <button @click="openCreateModal" class="btn-primary">Create Vault</button>
    </div>

    <div v-else class="vault-grid">
      <div 
        v-for="vault in vaults" 
        :key="vault.path" 
        class="vault-card"
        @click="openUnlockModal(vault)"
      >
        <div class="icon-wrapper">
          <Folder class="text-primary" />
        </div>
        <div class="vault-info">
          <h3>{{ vault.name }}</h3>
        </div>
        <ArrowRight class="arrow" :size="18" />
      </div>
    </div>

    <!-- Create Vault Modal -->
    <div v-if="showCreateModal" class="modal-overlay" @click.self="showCreateModal = false">
      <div class="modal">
        <div class="modal-header">
          <div class="modal-icon"><Plus /></div>
          <div>
            <h2>Create New Vault</h2>
            <p>Securely encrypt your files.</p>
          </div>
        </div>
        
        <div class="form-group">
          <label>Vault Name</label>
          <input 
            ref="vaultNameInput"
            v-model="newVaultName" 
            placeholder="e.g. Finance" 
            @keyup.enter="passwordInput?.focus()"
          />
        </div>
        
        <div class="form-group">
          <label>Master Password</label>
          <input 
            ref="passwordInput"
            v-model="newVaultPassword" 
            type="password" 
            placeholder="••••••••••••" 
            @keyup.enter="handleCreate"
          />
        </div>
        
        <div class="form-group checkbox-group" v-if="devicePepperInfo?.available">
          <label class="checkbox-label" :title="devicePepperInfo.isRemovable ? 'Ties this vault password to this removable drive' : 'Ties this vault password to this drive'">
            <input type="checkbox" v-model="newVaultUseDevicePepper" />
            Hardware-lock to SN: {{ devicePepperInfo.serialId }}
          </label>
        </div>

        <p v-if="error" class="error-text">{{ error }}</p>

        <div class="actions">
          <button @click="showCreateModal = false" :disabled="isLoading" class="btn-ghost">Cancel</button>
          <button @click="handleCreate" class="btn-primary" :disabled="isLoading">
            <span v-if="isLoading" class="flex items-center gap-2"><Loader2 class="animate-spin" :size="16"/> Creating...</span>
            <span v-else>Create Vault</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Unlock Vault Modal -->
    <div v-if="showUnlockModal" class="modal-overlay" @click.self="showUnlockModal = false">
      <div class="modal">
        <div class="modal-header">
          <div class="modal-icon"><Unlock /></div>
          <div>
            <h2>Unlock {{ selectedVault?.name }}</h2>
            <p>Enter password to decrypt.</p>
          </div>
        </div>

        <!-- DMS Released Notice -->
        <div v-if="dmsStatus?.released" class="dms-release-notice">
          <div class="dms-release-header">
            <Shield :size="20" />
            <span>Dead Man's Switch Triggered</span>
          </div>
          <p>This vault's Dead Man's Switch has been released. You can recover it without a password.</p>
          <button @click="handleRecoverVault" class="btn-primary recover-btn" :disabled="dmsRecovering">
            <span v-if="dmsRecovering" class="flex items-center gap-2"><Loader2 class="animate-spin" :size="16"/> Recovering...</span>
            <span v-else class="flex items-center gap-2"><Shield :size="16" /> Recover Vault</span>
          </button>
          <div class="dms-divider">
            <span>or unlock with password</span>
          </div>
        </div>

        <!-- DMS Checking indicator -->
        <div v-else-if="dmsChecking" class="dms-checking">
          <Loader2 class="animate-spin" :size="14" />
          <span>Checking Dead Man's Switch status...</span>
        </div>
        
        <div class="form-group">
          <input 
            ref="unlockPasswordInput"
            v-model="unlockPassword" 
            type="password" 
            placeholder="Master Password" 
            @keyup.enter="handleUnlock"
          />
        </div>

        <p v-if="error" class="error-text">{{ error }}</p>

        <div class="actions">
          <button @click="showUnlockModal = false" :disabled="isLoading" class="btn-ghost">Cancel</button>
          <button @click="handleUnlock" class="btn-primary" :disabled="isLoading">
             <span v-if="isLoading" class="flex items-center gap-2"><Loader2 class="animate-spin" :size="16"/> Unlocking...</span>
             <span v-else>Unlock</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.text-primary { color: var(--primary); }
.animate-spin { animation: spin 1s linear infinite; }

@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }

.logo h1 {
  font-size: 1.5rem;
  color: var(--text-main);
  margin-bottom: 0.25rem;
}

.empty-state {
  text-align: center;
  padding: 4rem 2rem;
  background: var(--bg-surface);
  border: 1px dashed var(--border);
  border-radius: var(--radius);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
}

.empty-icon {
  color: var(--text-faint);
  margin-bottom: 0.5rem;
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 4rem;
  color: var(--text-muted);
  gap: 1rem;
}

.vault-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 1rem;
}

.vault-card {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  background: var(--bg-app);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  cursor: pointer;
  transition: all 0.2s;
}

.vault-card:hover {
  background: var(--bg-surface);
  border-color: var(--primary);
  transform: translateY(-2px);
  box-shadow: var(--shadow);
}

.icon-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  background: var(--bg-surface-hover);
  border-radius: 0.5rem;
}

.vault-info { flex: 1; }
.vault-info h3 { font-size: 0.95rem; color: var(--text-main); font-weight: 500; }

.arrow {
  color: var(--text-muted);
  opacity: 0;
  transform: translateX(-5px);
  transition: all 0.2s;
}

.vault-card:hover .arrow { opacity: 1; transform: translateX(0); }

.modal-header {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.modal-icon {
  padding: 0.75rem;
  background: var(--bg-surface-hover);
  border-radius: 0.75rem;
  color: var(--primary);
  display: flex;
}

.form-group { margin-bottom: 1.25rem; }
.form-group label {
  display: block;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-muted);
  margin-bottom: 0.5rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  margin-top: 2rem;
}

.actions button { flex: 1; }

.dms-release-notice {
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.25);
  border-radius: 0.75rem;
  padding: 1rem;
  margin-bottom: 1.25rem;
}

.dms-release-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: #f59e0b;
  font-weight: 600;
  font-size: 0.9rem;
  margin-bottom: 0.5rem;
}

.dms-release-notice p {
  font-size: 0.82rem;
  color: var(--text-muted);
  margin-bottom: 0.75rem;
  line-height: 1.4;
}

.recover-btn {
  width: 100%;
  justify-content: center;
}

.dms-divider {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-top: 1rem;
  color: var(--text-muted);
  font-size: 0.75rem;
}

.dms-divider::before,
.dms-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--border);
}

.dms-checking {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.8rem;
  color: var(--text-muted);
  margin-bottom: 1rem;
}
</style>
