<script setup lang="ts">
import { Bell, Calendar, Check, Copy, Info, Key, Loader2, Save, Shield, X } from 'lucide-vue-next';
import QRCode from 'qrcode';
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import {
  BuyCredits,
  ChangeVaultPassword,
  DisableDeadManSwitch,
  EnableDeadManSwitch,
  GetDeadManSwitchStatus,
  GetDevicePepperInfo,
  GetVaultInfo,
  IsRecoveredVault,
  ResetDeadManSwitch,
  SetNewPassword,
  UpdateDeadManSwitchSettings
} from '../../bindings/passedbox/vaultmanager';
import { useToast } from '../composables/useToast';
import { copyToClipboard, formatError } from '../utils';

const props = defineProps<{
  name: string
}>()

const router = useRouter()
const { addToast } = useToast()

const activeTab = ref<'info' | 'password' | 'dms'>('info')
const isLoading = ref(true)
const info = ref<{vaultId: string, totalFiles: number, totalFolders: number, totalSize: number, dmsEnabled: boolean, dmsServerUrl: string} | null>(null)

// Password form
const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const isChangingPassword = ref(false)
const isCopied = ref(false)
const isRecovered = ref(false)

const useDevicePepper = ref(false)
const devicePepperInfo = ref<{available: boolean, serialId: string, isRemovable: boolean} | null>(null)

// DMS state
const dmsLoading = ref(false)
const dmsStatus = ref<any>(null)
const dmsServerURL = ref('https://dms.passedbox.com')
const dmsPassword = ref('')
const dmsEnabling = ref(false)
const dmsSaving = ref(false)
const dmsDisabling = ref(false)
const buyingCredits = ref(false)
const dmsResetting = ref(false)
const showResetConfirm = ref(false)
const resetVaultID = ref(false)
const showConflictReset = ref(false)
const dmsAuthError = ref(false)

// DMS settings (editable)
const releaseOnExpiry = ref(false)
const enableKeepAlive = ref(false)
const keepAliveDays = ref(30)

// QR / Calendar / Keep-alive modal
const calendarCopied = ref(false)
const showKeepAliveModal = ref(false)
const keepAliveMethod = ref<'calendar' | 'push'>('calendar')
const calendarQrDataUrl = ref('')
const pushQrDataUrl = ref('')
const pushUrlCopied = ref(false)

onMounted(async () => {
  await fetchInfo()
})

const fetchInfo = async () => {
  try {
    isLoading.value = true
    const result = await GetVaultInfo(props.name)
    info.value = result
    useDevicePepper.value = result.usePepper
  } catch (err) {
    addToast(formatError(err), 'error')
  } finally {
    isLoading.value = false
  }
  
  try {
    devicePepperInfo.value = await GetDevicePepperInfo()
  } catch (e) {
    console.error("failed to get device pepper info", e)
  }

  try {
    isRecovered.value = await IsRecoveredVault(props.name)
  } catch (e) {
    console.error("failed to check recovered state", e)
  }
}

const fetchDMSStatus = async () => {
  dmsLoading.value = true
  dmsAuthError.value = false
  try {
    const status = await GetDeadManSwitchStatus(props.name)
    dmsStatus.value = status
    if (status.enabled) {
      releaseOnExpiry.value = status.releaseOnExpiry
      enableKeepAlive.value = status.enableKeepAlive
      keepAliveDays.value = status.keepAliveDays || 30
    }
  } catch (err) {
    const errStr = formatError(err)
    if (errStr.includes('UNAUTHORIZED')) {
      dmsAuthError.value = true
    } else {
      addToast(errStr, 'error')
    }
  } finally {
    dmsLoading.value = false
  }
}

const openAndWatchPopup = (url: string) => {
  const popup = window.open(url, '_blank')
  if (!popup) return
  const timer = setInterval(() => {
    if (popup.closed) {
      clearInterval(timer)
      fetchDMSStatus()
    }
  }, 1000)
}

const handleBuyCredits = async () => {
  buyingCredits.value = true
  try {
    const checkoutUrl = await BuyCredits(props.name, 1)
    if (checkoutUrl) {
      openAndWatchPopup(checkoutUrl)
      addToast('Stripe checkout opened in your browser', 'success')
    }
  } catch (err) {
    addToast(formatError(err), 'error')
  } finally {
    buyingCredits.value = false
  }
}

const handlePasswordChange = async () => {
  if (newPassword.value !== confirmPassword.value) {
    addToast('New passwords do not match', 'error')
    return
  }

  if (newPassword.value.length < 1) {
    addToast('Password cannot be empty', 'error')
    return
  }

  try {
    isChangingPassword.value = true
    if (isRecovered.value) {
      await SetNewPassword(props.name, newPassword.value, useDevicePepper.value)
      isRecovered.value = false
    } else {
      await ChangeVaultPassword(props.name, oldPassword.value, newPassword.value, useDevicePepper.value)
    }
    addToast('Password changed successfully!', 'success')
    oldPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
  } catch (err) {
    addToast(formatError(err), 'error')
  } finally {
    isChangingPassword.value = false
  }
}

const handleEnableDMS = async () => {
  if (!dmsServerURL.value.trim()) {
    addToast('Server URL is required', 'error')
    return
  }
  if (!dmsPassword.value) {
    addToast('Vault password is required to enable Dead Man\'s Switch', 'error')
    return
  }
  dmsEnabling.value = true
  try {
    const result = await EnableDeadManSwitch(props.name, dmsServerURL.value.trim(), dmsPassword.value)
    dmsStatus.value = result
    dmsPassword.value = ''
    if (info.value) {
      info.value.dmsEnabled = true
      info.value.dmsServerUrl = dmsServerURL.value.trim()
    }
    // If server returned a checkout URL, open it in the browser
    if (result.checkoutUrl) {
      if (!dmsStatus.value) {
        dmsStatus.value = {}
      }
      dmsStatus.value.paymentEnabled = true
      addToast('Redirecting to payment...', 'info')
      openAndWatchPopup(result.checkoutUrl)
    } else if (result.status === 'pending') {
      addToast('Vault registered — waiting for admin approval', 'info')
    } else {
      addToast('Dead Man\'s Switch activated!', 'success')
    }
    await fetchDMSStatus()
  } catch (err) {
    const errStr = formatError(err)
    if (errStr.includes('CONFLICT')) {
      showConflictReset.value = true
      addToast('Vault ID conflict — reset required', 'error')
    } else {
      addToast(errStr, 'error')
    }
  } finally {
    dmsEnabling.value = false
  }
}

const handleSaveDMSSettings = async () => {
  dmsSaving.value = true
  try {
    await UpdateDeadManSwitchSettings(props.name, {
      releaseOnExpiry: releaseOnExpiry.value,
      enableKeepAlive: enableKeepAlive.value,
      keepAliveDays: keepAliveDays.value,
    })
    addToast('Settings saved', 'success')
  } catch (err) {
    addToast(formatError(err), 'error')
  } finally {
    dmsSaving.value = false
  }
}

const handleDisableDMS = async () => {
  dmsDisabling.value = true
  try {
    await DisableDeadManSwitch(props.name)
    dmsStatus.value = null
    if (info.value) {
      info.value.dmsEnabled = false
      info.value.dmsServerUrl = ''
    }
    addToast('Dead Man\'s Switch disabled', 'success')
  } catch (err) {
    addToast(formatError(err), 'error')
  } finally {
    dmsDisabling.value = false
  }
}

const handleResetDMS = async () => {
  dmsResetting.value = true
  try {
    await ResetDeadManSwitch(props.name, resetVaultID.value)
    dmsStatus.value = null
    showResetConfirm.value = false
    resetVaultID.value = false
    if (info.value) {
      info.value.dmsEnabled = false
      info.value.dmsServerUrl = ''
    }
    await fetchInfo()
    addToast('DMS state reset successfully', 'success')
  } catch (err) {
    addToast(formatError(err), 'error')
  } finally {
    dmsResetting.value = false
  }
}

const handleCopyCalendar = async () => {
  if (!dmsStatus.value?.calendarUrl) return
  const success = await copyToClipboard(dmsStatus.value.calendarUrl)
  if (success) {
    calendarCopied.value = true
    setTimeout(() => { calendarCopied.value = false }, 2000)
  }
}

const getCheckinUrl = () => {
  if (!dmsStatus.value) return ''
  const serverUrl = dmsStatus.value.serverUrl || info.value?.dmsServerUrl || ''
  const token = dmsStatus.value.token || ''
  const vaultId = info.value?.vaultId || ''
  return `${serverUrl}/checkin?id=${vaultId}&token=${token}`
}

const handleCopyPushUrl = async () => {
  const url = getCheckinUrl()
  if (!url) return
  const success = await copyToClipboard(url)
  if (success) {
    pushUrlCopied.value = true
    setTimeout(() => { pushUrlCopied.value = false }, 2000)
  }
}

const openKeepAliveModal = async () => {
  showKeepAliveModal.value = true
  await generateQrCodes()
}

const generateQrCodes = async () => {
  try {
    if (dmsStatus.value?.calendarUrl) {
      calendarQrDataUrl.value = await QRCode.toDataURL(dmsStatus.value.calendarUrl, {
        width: 220,
        margin: 2,
        color: { dark: '#000000', light: '#ffffff' },
      })
    }
    const checkinUrl = getCheckinUrl()
    if (checkinUrl) {
      pushQrDataUrl.value = await QRCode.toDataURL(checkinUrl, {
        width: 220,
        margin: 2,
        color: { dark: '#000000', light: '#ffffff' },
      })
    }
  } catch (err) {
    console.error('Failed to generate QR codes:', err)
  }
}

const formatSize = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const goBack = () => {
  router.push(`/vault/${props.name}`)
}

const handleCopy = async () => {
  if (!info.value?.vaultId) return
  const success = await copyToClipboard(info.value.vaultId)
  if (success) {
    isCopied.value = true
    setTimeout(() => {
      isCopied.value = false
    }, 2000)
  }
}

const formatDate = (d: string) => {
  if (!d || d === '0001-01-01T00:00:00Z') return '—'
  return new Date(d).toLocaleDateString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}
</script>

<template>
  <div class="settings-view">
    <div class="settings-container">
      <div class="settings-tabs">
        <button 
          :class="['tab-btn', { active: activeTab === 'info' }]" 
          @click="activeTab = 'info'"
        >
          <Info :size="18" />
          <span>Info</span>
        </button>
        <button 
          :class="['tab-btn', { active: activeTab === 'password' }]" 
          @click="activeTab = 'password'"
        >
          <Key :size="18" />
          <span>Security</span>
        </button>
        <button 
          :class="['tab-btn', { active: activeTab === 'dms' }]" 
          @click="activeTab = 'dms'; fetchDMSStatus()"
        >
          <Shield :size="18" />
          <span>Dead Man's Switch</span>
        </button>
      </div>

      <div class="settings-content">
        <!-- INFO TAB -->
        <div v-if="activeTab === 'info'" class="tab-pane">
          <div v-if="isLoading" class="loading-state">
            <Loader2 class="animate-spin" :size="32" />
            <p>Gathering vault information...</p>
          </div>
          <div v-else-if="info" class="info-grid">
            <div class="info-item">
              <label>Vault ID</label>
              <div class="id-container">
                <span class="value path">{{ info.vaultId }}</span>
                <button class="copy-btn" @click="handleCopy" :title="isCopied ? 'Copied!' : 'Copy Vault ID'">
                  <Check v-if="isCopied" :size="16" class="success-icon" />
                  <Copy v-else :size="16" />
                </button>
              </div>
            </div>
            <div class="info-item">
              <label>Total Files</label>
              <span class="value">{{ info.totalFiles }}</span>
            </div>
            <div class="info-item">
              <label>Total Folders</label>
              <span class="value">{{ info.totalFolders }}</span>
            </div>
            <div class="info-item">
              <label>Total Vault Size</label>
              <span class="value">{{ formatSize(info.totalSize) }}</span>
            </div>
            <div class="info-item">
              <label>Vault Path</label>
              <span class="value">{{ name }}.pbx</span>
            </div>
          </div>
        </div>

        <!-- PASSWORD TAB -->
        <div v-if="activeTab === 'password'" class="tab-pane">
          <form @submit.prevent="handlePasswordChange" class="password-form">
            <div v-if="isRecovered" class="dms-released-notice" style="margin-bottom: 1rem;">
              <p style="font-size: 0.85rem; color: var(--text-muted); line-height: 1.5;">
                This vault was recovered via Dead Man's Switch. You must set a new master password to secure it.
              </p>
            </div>
            <template v-if="!isRecovered">
              <div class="input-group">
                <label>Current Password</label>
                <input 
                  v-model="oldPassword" 
                  type="password" 
                  required 
                  placeholder="Enter current master password"
                />
              </div>
              <div class="divider"></div>
            </template>
            <div class="input-group">
              <label>New Password</label>
              <input 
                v-model="newPassword" 
                type="password" 
                required 
                placeholder="Enter new master password"
              />
            </div>
            <div class="input-group">
              <label>Confirm New Password</label>
              <input 
                v-model="confirmPassword" 
                type="password" 
                required 
                placeholder="Re-type new master password"
              />
            </div>
            
            <div class="input-group checkbox-group" v-if="devicePepperInfo?.available">
              <label class="checkbox-label" :title="devicePepperInfo.isRemovable ? 'Ties this vault password to this removable drive' : 'Ties this vault password to this drive'">
                <input type="checkbox" v-model="useDevicePepper" />
                Hardware-lock to SN: {{ devicePepperInfo.serialId }}
              </label>
            </div>
            
            <button type="submit" class="btn-primary" :disabled="isChangingPassword">
              <Loader2 v-if="isChangingPassword" class="animate-spin" :size="18" />
              <Save v-else :size="18" />
              <span>{{ isRecovered ? 'Set Password' : 'Change Password' }}</span>
            </button>
          </form>
        </div>

        <!-- DEAD MAN'S SWITCH TAB -->
        <div v-if="activeTab === 'dms'" class="tab-pane tab-pane-scrollable">
          <div v-if="dmsLoading" class="loading-state">
            <Loader2 class="animate-spin" :size="32" />
            <p>Checking dead man's switch status...</p>
          </div>

          <!-- NOT ENABLED — Setup Flow -->
          <div v-else-if="!info?.dmsEnabled" class="dms-setup">
            <div class="dms-hero">
              <Shield :size="48" class="dms-icon" />
              <h3>Dead Man's Switch</h3>
              <p class="dms-desc">
                Ensure your vault is accessible to your trusted contacts if you become unavailable.
                A third Shamir key share will be encrypted and stored on a server — released only when
                your credits expire or you stop checking in.
              </p>
            </div>

            <div class="dms-setup-form">
              <div class="input-group">
                <label>Server URL</label>
                <input 
                  v-model="dmsServerURL" 
                  type="url" 
                  placeholder="https://dms.passedbox.com"
                />
                <span class="input-hint">
                  Use dms.passedbox.com (paid) or self-host for free
                </span>
              </div>

              <div class="input-group">
                <label>Vault Password</label>
                <input 
                  v-model="dmsPassword" 
                  type="password" 
                  placeholder="Enter your vault password"
                />
                <span class="input-hint">
                  Required to re-encrypt key shares for the Dead Man's Switch
                </span>
              </div>

              <template v-if="dmsStatus?.paymentEnabled">
                <div class="dms-pricing">
                  <span class="price-tag">$15 / year per vault</span>
                  <span class="price-note">Not a subscription — buy credits upfront</span>
                </div>
              </template>

              <button 
                class="btn-primary btn-activate" 
                @click="handleEnableDMS" 
                :disabled="dmsEnabling"
              >
                <Loader2 v-if="dmsEnabling" class="animate-spin" :size="18" />
                <Shield v-else :size="18" />
                <span>{{ dmsEnabling ? 'Activating...' : 'Activate Dead Man\'s Switch' }}</span>
              </button>

              <!-- Conflict reset prompt -->
              <div v-if="showConflictReset" style="margin-top: 1rem; background: rgba(239,68,68,0.08); border: 1px solid rgba(239,68,68,0.25); border-radius: 8px; padding: 0.75rem;">
                <p style="font-size: 0.85rem; color: var(--danger, #ef4444); margin-bottom: 0.5rem; font-weight: 600;">Vault ID already exists on the server</p>
                <p style="font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.75rem;">The current vault ID is already registered. Reset with a new vault ID to continue.</p>
                <button class="btn-danger-outline" @click="async () => { await ResetDeadManSwitch(props.name, true); showConflictReset = false; await fetchInfo(); addToast('Vault ID reset — try activating again', 'success'); }" :disabled="dmsResetting">
                  Reset Vault ID &amp; Retry
                </button>
              </div>
            </div>
          </div>

          <!-- ENABLED — Status & Settings -->
          <div v-else-if="dmsStatus?.enabled" class="dms-active">
            <div class="dms-status-header">
              <div :class="['status-badge', dmsStatus.released ? 'warning' : 'active']">
                <Shield :size="14" />
                {{ dmsStatus.released ? 'Released' : 'Active' }}
              </div>
              <span class="server-url">{{ dmsStatus.serverUrl }}</span>
            </div>

            <!-- RELEASED STATE -->
            <template v-if="dmsStatus.released">
              <div class="dms-released-notice">
                <p>This vault's Dead Man's Switch has been triggered. The encrypted share is now available for recovery.</p>
                <p v-if="dmsStatus.releasedAt" style="margin-top: 0.5rem; font-weight: 600; color: #f59e0b;">Released: {{ formatDate(dmsStatus.releasedAt) }}</p>
              </div>
              <p class="dms-desc">To re-enable the Dead Man's Switch, disable it first, then set it up again with a new key split.</p>
              <div class="dms-danger">
                <button class="btn-danger-outline" @click="handleDisableDMS" :disabled="dmsDisabling">
                  {{ dmsDisabling ? 'Disabling...' : 'Disable Dead Man\'s Switch' }}
                </button>
              </div>
            </template>

            <!-- ACTIVE / PENDING / INACTIVE STATE -->
            <template v-else>
              <!-- Status Banner -->
              <div v-if="dmsStatus.status === 'pending'" class="dms-status-banner" style="background: rgba(251,191,36,0.1); border: 1px solid rgba(251,191,36,0.3); border-radius: 8px; padding: 0.75rem 1rem; margin-bottom: 0.75rem; font-size: 0.85rem; color: #f59e0b;">
                <template v-if="dmsStatus?.paymentEnabled">
                  ⚠ No active credits. Purchase credits to enable.
                </template>
                <template v-else>
                  ⏳ Waiting for admin approval. Settings will be available once approved.
                </template>
              </div>
              <div v-else-if="dmsStatus.status === 'inactive'" class="dms-status-banner" style="background: rgba(148,163,184,0.1); border: 1px solid rgba(148,163,184,0.3); border-radius: 8px; padding: 0.75rem 1rem; margin-bottom: 0.75rem; font-size: 0.85rem; color: #94a3b8;">
                ⏸ Vault is inactive. Re-enable Dead Man's Switch to reactivate.
              </div>

              <!-- Status Grid -->
              <div class="dms-stats">
                <div class="dms-stat">
                  <label>Status</label>
                  <span :class="{
                    'text-success': dmsStatus.status === 'active',
                    'text-warning': dmsStatus.status === 'pending',
                    'text-muted': dmsStatus.status === 'inactive'
                  }">{{ dmsStatus.status || 'unknown' }}</span>
                </div>
                <div class="dms-stat">
                  <label>Last Check-In</label>
                  <span>{{ formatDate(dmsStatus.lastCheckIn) }}</span>
                </div>
                <div class="dms-stat">
                  <label>Credits</label>
                  <span :class="dmsStatus.creditsActive ? 'text-success' : 'text-danger'">
                    {{ dmsStatus.credits || 0 }} year(s)
                    {{ dmsStatus.creditsActive ? '(active)' : '(expired)' }}
                  </span>
                </div>
                <div class="dms-stat" v-if="dmsStatus.releaseDate">
                  <label>Release Date</label>
                  <span>{{ formatDate(dmsStatus.releaseDate) }}</span>
                </div>
              </div>

              <template v-if="dmsStatus?.paymentEnabled">
                <button
                  class="btn btn-accent btn-sm"
                  @click="handleBuyCredits"
                  :disabled="buyingCredits"
                  style="align-self: flex-start; margin-top: 0.25rem;"
                >
                  {{ buyingCredits ? 'Opening...' : 'Buy More Credits' }}
                </button>
              </template>

              <!-- Settings (only when active) -->
              <template v-if="dmsStatus.status === 'active'">
                <h4 class="section-title">Trigger Settings</h4>

                <div class="dms-settings">
                <label class="checkbox-label" :class="{ 'disabled-label': dmsStatus?.paymentEnabled && !dmsStatus.credits}">
                    <input type="checkbox" v-model="releaseOnExpiry" :disabled="dmsStatus?.paymentEnabled && !dmsStatus.credits" />
                    Release vault when credits expire
                  </label>
                  <label class="checkbox-label">
                    <input type="checkbox" v-model="enableKeepAlive" />
                    Enable keep-alive check-in
                  </label>
                  <div v-if="enableKeepAlive" class="input-group" style="max-width: 200px; margin-left: 1.5rem;">
                    <label>Check-in interval (days)</label>
                    <input 
                      type="number" 
                      v-model.number="keepAliveDays" 
                      min="1" 
                      max="365" 
                    />
                  </div>
                </div>

                <button class="btn-primary" @click="handleSaveDMSSettings" :disabled="dmsSaving" style="margin-top: 1rem;">
                  <Loader2 v-if="dmsSaving" class="animate-spin" :size="18" />
                  <Save v-else :size="18" />
                  <span>{{ dmsSaving ? 'Saving...' : 'Save Settings' }}</span>
                </button>

                <!-- Keep-Alive Setup Link -->
                <div v-if="enableKeepAlive" class="dms-keepalive-link">
                  <div class="divider"></div>
                  <h4 class="section-title">Keep-Alive Reminders</h4>
                  <p class="dms-desc">Set up automatic reminders so you never miss a check-in.</p>
                  <button class="btn-secondary" @click="openKeepAliveModal">
                    <Bell :size="16" />
                    <span>Setup Keep-Alive Reminders</span>
                  </button>
                </div>
              </template>

              <!-- Danger Zone -->
              <div class="dms-danger">
                <div class="divider"></div>
                <h4 class="section-title text-danger">Danger Zone</h4>
                <button class="btn-danger-outline" @click="handleDisableDMS" :disabled="dmsDisabling">
                  {{ dmsDisabling ? 'Disabling...' : 'Disable Dead Man\'s Switch' }}
                </button>
              </div>
            </template>
          </div>

          <!-- DMS enabled locally but auth failed (bad token, deleted vault, etc) -->
          <div v-else-if="info?.dmsEnabled && dmsAuthError" class="dms-active">
            <div class="dms-status-header">
              <div class="status-badge warning">
                <Shield :size="14" />
                Authentication Failed
              </div>
              <span class="server-url">{{ info.dmsServerUrl }}</span>
            </div>
            <p class="dms-desc" style="margin-top: 1rem;">
              Could not authenticate with the server. The vault may have been deleted, or the token is no longer valid.
            </p>
            <div style="margin-top: 1rem; background: rgba(239,68,68,0.08); border: 1px solid rgba(239,68,68,0.25); border-radius: 8px; padding: 0.75rem;">
              <p style="font-size: 0.85rem; color: var(--text-muted); margin-bottom: 0.5rem;">Reset local DMS state to re-register with a new vault ID.</p>
              <label class="checkbox-label" style="margin-bottom: 0.5rem;">
                <input type="checkbox" v-model="resetVaultID" />
                Also reset Vault ID (generates new ID)
              </label>
              <div style="display: flex; gap: 0.5rem;">
                <button class="btn-danger-outline" @click="handleResetDMS" :disabled="dmsResetting">
                  {{ dmsResetting ? 'Resetting...' : 'Reset DMS State' }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Keep-Alive Modal -->
    <Teleport to="body">
      <div v-if="showKeepAliveModal" class="modal-overlay" @click.self="showKeepAliveModal = false">
        <div class="modal-container">
          <div class="modal-header">
            <h3>Keep-Alive Setup</h3>
            <button class="modal-close" @click="showKeepAliveModal = false">
              <X :size="20" />
            </button>
          </div>

          <div class="modal-body">
            <!-- Method Toggle -->
            <div class="method-toggle">
              <button 
                :class="['toggle-btn', { active: keepAliveMethod === 'calendar' }]"
                @click="keepAliveMethod = 'calendar'"
              >
                <Calendar :size="16" />
                <span>Calendar</span>
              </button>
              <button 
                :class="['toggle-btn', { active: keepAliveMethod === 'push' }]"
                @click="keepAliveMethod = 'push'"
              >
                <Bell :size="16" />
                <span>Web Push</span>
              </button>
            </div>

            <!-- Calendar Method -->
            <div v-if="keepAliveMethod === 'calendar'" class="method-content">
              <p class="method-desc">
                Subscribe to a calendar feed that creates recurring reminders. Click the reminder link to check in.
              </p>
              <div class="qr-section">
                <div class="qr-code" v-if="calendarQrDataUrl">
                  <img :src="calendarQrDataUrl" alt="Calendar feed QR code" />
                </div>
                <p class="qr-label">Scan to add calendar feed</p>
              </div>
              <div class="url-section">
                <label>Calendar URL</label>
                <div class="id-container">
                  <span class="value path" style="font-size: 0.75rem;">{{ dmsStatus?.calendarUrl }}</span>
                  <button class="copy-btn" @click="handleCopyCalendar" :title="calendarCopied ? 'Copied!' : 'Copy Calendar URL'">
                    <Check v-if="calendarCopied" :size="16" class="success-icon" />
                    <Copy v-else :size="16" />
                  </button>
                </div>
              </div>
              <div class="method-instructions">
                <h5>How to set up</h5>
                <ol>
                  <li>Copy the Calendar URL or scan the QR code</li>
                  <li>In Google Calendar: Other Calendars → From URL → Paste</li>
                  <li>In Apple Calendar: File → New Subscription → Paste</li>
                  <li>You'll receive recurring events reminding you to check in</li>
                  <li>Click the link in the event to confirm your check-in</li>
                </ol>
              </div>
            </div>

            <!-- Web Push Method -->
            <div v-if="keepAliveMethod === 'push'" class="method-content">
              <p class="method-desc">
                Get browser push notifications when it's time to check in. Open the check-in page on any device to subscribe.
              </p>
              <div class="qr-section">
                <div class="qr-code" v-if="pushQrDataUrl">
                  <img :src="pushQrDataUrl" alt="Check-in URL QR code" />
                </div>
                <p class="qr-label">Scan to open check-in page</p>
              </div>
              <div class="url-section">
                <label>Check-In URL</label>
                <div class="id-container">
                  <span class="value path" style="font-size: 0.75rem;">{{ getCheckinUrl() }}</span>
                  <button class="copy-btn" @click="handleCopyPushUrl" :title="pushUrlCopied ? 'Copied!' : 'Copy Check-In URL'">
                    <Check v-if="pushUrlCopied" :size="16" class="success-icon" />
                    <Copy v-else :size="16" />
                  </button>
                </div>
              </div>
              <div class="method-instructions">
                <h5>How to set up</h5>
                <ol>
                  <li>Open the check-in URL in your browser (or scan the QR code)</li>
                  <li>Allow push notifications when prompted</li>
                  <li>You'll receive push notifications when it's time to check in</li>
                  <li>Click the notification to confirm your check-in</li>
                </ol>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.settings-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  padding: 1.5rem;
  background-color: var(--bg-app);
  color: var(--text-main);
  overflow-y: auto;
}

.settings-header {
  margin-bottom: 2rem;
}

.back-btn {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  background: none;
  border: none;
  color: var(--text-dim);
  cursor: pointer;
  padding: 0.5rem 0;
  font-size: 0.9rem;
  transition: color 0.2s;
}

.back-btn:hover {
  color: var(--text-main);
}

.settings-header h1 {
  font-size: 1.5rem;
  font-weight: 600;
  margin-top: 0.5rem;
}

.settings-container {
  max-width: 600px;
  width: 100%;
  margin: 0 auto;
  background-color: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 4px 12px rgba(0,0,0,0.1);
  display: flex;
  flex-direction: column;
  max-height: 100%;
}

.settings-tabs {
  display: flex;
  background-color: var(--bg-surface-off);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

.tab-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 1rem;
  border: none;
  background: none;
  color: var(--text-dim);
  cursor: pointer;
  font-weight: 500;
  transition: all 0.2s;
  border-bottom: 2px solid transparent;
}

.tab-btn:hover {
  background-color: rgba(255,255,255,0.05);
  color: var(--text-main);
}

.tab-btn.active {
  color: var(--accent);
  border-bottom-color: var(--accent);
  background-color: var(--bg-surface);
}

.settings-content {
  padding: 2rem;
  min-height: 300px;
  overflow-y: auto;
  flex: 1;
}


.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 200px;
  gap: 1rem;
  color: var(--text-dim);
}

.info-grid {
  display: grid;
  gap: 1.5rem;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.info-item label {
  font-size: 0.8rem;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.info-item .value {
  font-size: 1.1rem;
  font-weight: 500;
}

.info-item .value.path {
  font-family: monospace;
  background-color: var(--bg-app);
  padding: 0.25rem 0.6rem;
  border-radius: 6px;
  border: 1px solid var(--border);
  font-size: 0.95rem;
  width: fit-content;
  max-width: 100%;
  overflow-x: auto;
}

.id-container {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
}

.copy-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 0.4rem;
  color: var(--text-dim);
  cursor: pointer;
  transition: all 0.2s;
  flex-shrink: 0;
}

.copy-btn:hover {
  border-color: var(--accent);
  color: var(--accent);
  background-color: rgba(var(--accent-rgb), 0.1);
}

.success-icon {
  color: #10b981;
}

.password-form {
  display: grid;
  gap: 1.25rem;
}

.input-group {
  display: grid;
  gap: 0.5rem;
}

.input-group label {
  font-size: 0.9rem;
  font-weight: 500;
}

.input-group input[type="text"],
.input-group input[type="password"],
.input-group input[type="url"],
.input-group input[type="number"] {
  padding: 0.75rem;
  background-color: var(--bg-app);
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text-main);
  outline: none;
  font-size: 0.9rem;
}

.input-group input:focus {
  border-color: var(--accent);
}

.input-hint {
  font-size: 0.75rem;
  color: var(--text-dim);
}

.divider {
  height: 1px;
  background-color: var(--border);
  margin: 0.5rem 0;
}

.btn-primary {
  width: 100%;
  margin-top: 1.5rem;
}

.btn-secondary {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.6rem 1.2rem;
  background: rgba(99, 102, 241, 0.1);
  border: 1px solid rgba(99, 102, 241, 0.3);
  border-radius: 8px;
  color: var(--accent);
  font-size: 0.85rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-secondary:hover {
  background: rgba(99, 102, 241, 0.2);
  border-color: var(--accent);
}

.animate-spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* DMS specific styles */
.dms-hero {
  text-align: center;
  padding: 1rem 0 1.5rem;
}

.dms-icon {
  color: var(--accent);
  margin-bottom: 0.75rem;
}

.dms-hero h3 {
  font-size: 1.2rem;
  font-weight: 600;
  margin-bottom: 0.5rem;
}

.dms-desc {
  font-size: 0.85rem;
  color: var(--text-dim);
  line-height: 1.5;
}

.dms-setup-form {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.dms-pricing {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.75rem;
  background: rgba(99, 102, 241, 0.08);
  border: 1px solid rgba(99, 102, 241, 0.2);
  border-radius: 8px;
}

.price-tag {
  font-weight: 600;
  color: var(--accent);
  font-size: 1rem;
}

.price-note {
  font-size: 0.75rem;
  color: var(--text-dim);
}

.btn-activate {
  margin-top: 0.5rem;
}

.dms-status-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.dms-released-notice {
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.25);
  border-radius: 0.75rem;
  padding: 1rem;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.3rem 0.7rem;
  border-radius: 100px;
  font-size: 0.75rem;
  font-weight: 600;
}

.status-badge.active {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.status-badge.warning {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
}

.server-url {
  font-family: monospace;
  font-size: 0.8rem;
  color: var(--text-dim);
}

.dms-stats {
  display: grid;
  gap: 0.75rem;
  margin-top: 1rem;
}

.dms-stat {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.dms-stat label {
  font-size: 0.7rem;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.dms-stat span {
  font-size: 0.9rem;
  font-weight: 500;
}

.text-success { color: #10b981; }
.text-danger { color: #ef4444; }

.section-title {
  font-size: 0.85rem;
  font-weight: 600;
  margin: 0.75rem 0;
}

.dms-settings {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.9rem;
  cursor: pointer;
}

.dms-keepalive-link {
  margin-top: 0.5rem;
}

.dms-danger {
  margin-top: 1rem;
}

.btn-danger-outline {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.5rem 1rem;
  background: none;
  border: 1px solid #ef4444;
  border-radius: 8px;
  color: #ef4444;
  font-size: 0.85rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-danger-outline:hover {
  background: rgba(239, 68, 68, 0.1);
}

.btn-danger-outline:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Modal Styles */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
}

.modal-container {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: 16px;
  width: 90%;
  max-width: 480px;
  max-height: 85vh;
  overflow-y: auto;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid var(--border);
}

.modal-header h3 {
  font-size: 1.1rem;
  font-weight: 600;
}

.modal-close {
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  color: var(--text-dim);
  cursor: pointer;
  padding: 0.25rem;
  border-radius: 6px;
  transition: all 0.2s;
}

.modal-close:hover {
  color: var(--text-main);
  background: rgba(255, 255, 255, 0.1);
}

.modal-body {
  padding: 1.5rem;
}

/* Method Toggle */
.method-toggle {
  display: flex;
  background: var(--bg-app);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 3px;
  margin-bottom: 1.5rem;
}

.toggle-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  padding: 0.6rem;
  border: none;
  background: none;
  color: var(--text-dim);
  font-size: 0.85rem;
  font-weight: 500;
  cursor: pointer;
  border-radius: 8px;
  transition: all 0.2s;
}

.toggle-btn.active {
  background: var(--accent);
  color: #fff;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.3);
}

.toggle-btn:not(.active):hover {
  color: var(--text-main);
}

/* Method Content */
.method-content {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.method-desc {
  font-size: 0.85rem;
  color: var(--text-dim);
  line-height: 1.5;
}

.qr-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 1rem;
  background: #ffffff;
  border-radius: 12px;
  width: fit-content;
  margin: 0 auto;
}

.qr-code img {
  display: block;
  width: 220px;
  height: 220px;
  border-radius: 4px;
}

.qr-label {
  font-size: 0.75rem;
  color: #666;
  text-align: center;
}

.url-section {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.url-section label {
  font-size: 0.75rem;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.method-instructions {
  border-top: 1px solid var(--border);
  padding-top: 1rem;
}

.method-instructions h5 {
  font-size: 0.8rem;
  font-weight: 600;
  margin-bottom: 0.5rem;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.method-instructions ol {
  margin: 0;
  padding-left: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.method-instructions li {
  font-size: 0.82rem;
  color: var(--text-dim);
  line-height: 1.4;
}
</style>
