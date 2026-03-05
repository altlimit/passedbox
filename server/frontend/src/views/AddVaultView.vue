<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useVaultStore } from '../stores/vault'

const store = useVaultStore()
const router = useRouter()

const vaultId = ref('')
const submitting = ref(false)
const errorMsg = ref('')

async function handleSubmit() {
  if (!vaultId.value.trim()) {
    errorMsg.value = 'Vault ID is required'
    return
  }

  submitting.value = true
  errorMsg.value = ''
  try {
    // Register with empty share3Enc for now (desktop will send it)
    await store.addVault(vaultId.value.trim(), '')
    router.push('/vaults')
  } catch (e: any) {
    errorMsg.value = e.message || 'Failed to add vault'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div>
    <div class="page-header">
      <router-link to="/vaults" class="btn btn-ghost btn-sm mb-2" style="display: inline-flex;">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="15 18 9 12 15 6"/></svg>
        Back to Vaults
      </router-link>
      <h1>Add Vault</h1>
      <p>Register a vault ID to manage its dead man's switch</p>
    </div>

    <div class="card" style="max-width: 500px;">
      <div class="card-body">
        <form @submit.prevent="handleSubmit" style="display: flex; flex-direction: column; gap: 1.25rem;">
          <div class="form-group">
            <label class="form-label">Vault ID</label>
            <input
              v-model="vaultId"
              class="form-input"
              type="text"
              placeholder="Enter the vault UUID (from desktop app)"
              autofocus
            />
            <p class="text-dim" style="font-size: 0.75rem; margin-top: 0.25rem;">
              You can find this in your desktop app under Vault Settings → Info → Vault ID
            </p>
          </div>

          <p v-if="errorMsg" class="text-danger" style="font-size: 0.85rem;">{{ errorMsg }}</p>

          <button type="submit" class="btn btn-primary" :disabled="submitting">
            {{ submitting ? 'Adding…' : 'Add Vault' }}
          </button>
        </form>
      </div>
    </div>
  </div>
</template>
