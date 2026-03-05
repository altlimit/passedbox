<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { resetAuth } from '../router'

const router = useRouter()

// Settings state
const loading = ref(true)
const saving = ref(false)
const message = ref('')
const messageType = ref<'success' | 'error'>('success')

// Payment settings
const paymentEnabled = ref(false)
const stripeSecretKey = ref('')
const stripePriceId = ref('')
const baseUrl = ref('')
const hasStripeKey = ref(false)

// Password change
const changingPassword = ref(false)
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')

function showMessage(msg: string, type: 'success' | 'error' = 'success') {
  message.value = msg
  messageType.value = type
  setTimeout(() => { message.value = '' }, 4000)
}

async function fetchSettings() {
  loading.value = true
  try {
    const res = await fetch('/api/v1/settings')
    if (!res.ok) throw new Error(await res.text())
    const data = await res.json()
    paymentEnabled.value = data.paymentEnabled
    stripeSecretKey.value = ''
    stripePriceId.value = data.stripePriceId || ''
    baseUrl.value = data.baseUrl || ''
    hasStripeKey.value = data.hasStripeKey
  } catch (e: any) {
    showMessage(e.message, 'error')
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  saving.value = true
  try {
    const body: any = {
      paymentEnabled: paymentEnabled.value,
      stripePriceId: stripePriceId.value,
      baseUrl: baseUrl.value,
    }
    if (stripeSecretKey.value) body.stripeSecretKey = stripeSecretKey.value

    const res = await fetch('/api/v1/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw new Error(await res.text())
    showMessage('Settings saved')
    await fetchSettings()
  } catch (e: any) {
    showMessage(e.message, 'error')
  } finally {
    saving.value = false
  }
}

async function changePassword() {
  if (newPassword.value !== confirmPassword.value) {
    showMessage('Passwords do not match', 'error')
    return
  }
  changingPassword.value = true
  try {
    const res = await fetch('/api/v1/settings/password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        currentPassword: currentPassword.value,
        newPassword: newPassword.value,
      }),
    })
    if (!res.ok) {
      const data = await res.json().catch(() => ({ error: 'Failed' }))
      throw new Error(data.error || 'Failed to change password')
    }
    showMessage('Password changed — you will be redirected to login')
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    // Redirect to login after password change
    setTimeout(() => {
      resetAuth()
      router.push('/login')
    }, 2000)
  } catch (e: any) {
    showMessage(e.message, 'error')
  } finally {
    changingPassword.value = false
  }
}

async function handleLogout() {
  await fetch('/api/v1/logout', { method: 'POST' })
  resetAuth()
  router.push('/login')
}

onMounted(fetchSettings)
</script>

<template>
  <div>
    <div class="page-header flex-between">
      <div>
        <h1>Settings</h1>
        <p>Server configuration and admin account</p>
      </div>
      <button class="btn btn-ghost" @click="handleLogout">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
        Logout
      </button>
    </div>

    <!-- Notification banner -->
    <div v-if="message" class="alert" :class="messageType === 'error' ? 'alert-error' : 'alert-success'">
      {{ message }}
    </div>

    <div v-if="loading" class="loading-center"><div class="spinner"></div></div>

    <template v-else>
      <!-- Payment Settings -->
      <div class="card mb-2">
        <div class="card-header">
          <h2>Payment & Stripe</h2>
        </div>
        <div class="card-body settings-form">
          <div class="form-group">
            <label class="form-label">Payment Mode</label>
            <div class="toggle-row">
              <label class="toggle">
                <input type="checkbox" v-model="paymentEnabled" />
                <span class="toggle-slider"></span>
              </label>
              <span class="toggle-label">{{ paymentEnabled ? 'Enabled — vault registration requires Stripe payment' : 'Disabled — admin manually approves vaults' }}</span>
            </div>
          </div>

          <template v-if="paymentEnabled">
            <div class="form-group">
              <label class="form-label">Stripe Secret Key</label>
              <input class="form-input" v-model="stripeSecretKey" type="password"
                :placeholder="hasStripeKey ? '••••(configured — enter new value to replace)' : 'sk_live_...'" />
            </div>

            <div class="form-group">
              <label class="form-label">Stripe Price ID</label>
              <input class="form-input" v-model="stripePriceId" placeholder="price_..." />
            </div>
          </template>
          <div class="form-group">
            <label class="form-label">Base URL</label>
            <input class="form-input" v-model="baseUrl" placeholder="https://passedbox.com" />
            <span class="form-hint">Used for Stripe checkout redirect URLs</span>
          </div>

          <div class="form-actions">
            <button class="btn btn-primary" @click="saveSettings" :disabled="saving">
              {{ saving ? 'Saving…' : 'Save Settings' }}
            </button>
          </div>
        </div>
      </div>

      <!-- Change Password -->
      <div class="card">
        <div class="card-header">
          <h2>Change Password</h2>
        </div>
        <div class="card-body settings-form">
          <div class="form-group">
            <label class="form-label">Current Password</label>
            <input class="form-input" v-model="currentPassword" type="password" autocomplete="current-password" />
          </div>
          <div class="form-group">
            <label class="form-label">New Password</label>
            <input class="form-input" v-model="newPassword" type="password" autocomplete="new-password" />
          </div>
          <div class="form-group">
            <label class="form-label">Confirm New Password</label>
            <input class="form-input" v-model="confirmPassword" type="password" autocomplete="new-password" />
          </div>
          <div class="form-actions">
            <button class="btn btn-primary" @click="changePassword"
              :disabled="changingPassword || !currentPassword || !newPassword || !confirmPassword">
              {{ changingPassword ? 'Changing…' : 'Change Password' }}
            </button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.settings-form {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
  max-width: 480px;
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.toggle {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
  flex-shrink: 0;
}

.toggle input { display: none; }

.toggle-slider {
  position: absolute;
  inset: 0;
  background: var(--border);
  border-radius: 24px;
  cursor: pointer;
  transition: all 0.2s;
}

.toggle-slider::before {
  content: '';
  position: absolute;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: white;
  top: 3px;
  left: 3px;
  transition: transform 0.2s;
}

.toggle input:checked + .toggle-slider {
  background: var(--accent);
}

.toggle input:checked + .toggle-slider::before {
  transform: translateX(20px);
}

.toggle-label {
  font-size: 0.85rem;
  color: var(--text-dim);
}

.form-hint {
  font-size: 0.75rem;
  color: var(--text-muted);
  margin-top: 0.2rem;
}

.form-actions {
  padding-top: 0.5rem;
}

.alert {
  padding: 0.75rem 1rem;
  border-radius: var(--radius-sm);
  margin-bottom: 1rem;
  font-size: 0.85rem;
  font-weight: 500;
}

.alert-success {
  background: var(--success-dim);
  color: var(--success);
  border: 1px solid rgba(16, 185, 129, 0.2);
}

.alert-error {
  background: var(--danger-dim);
  color: var(--danger);
  border: 1px solid rgba(239, 68, 68, 0.2);
}
</style>
