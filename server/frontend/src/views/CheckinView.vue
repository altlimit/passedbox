<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()

const vaultId = ref('')
const token = ref('')

const loading = ref(false)
const status = ref<'idle' | 'success' | 'error'>('idle')
const statusTitle = ref('PassedBox Keep-Alive')
const statusSubtitle = ref('Confirm you are still active to keep your vault safe.')
const statusMsg = ref('')
const btnText = ref('Check In Now')
const showCheckmark = ref(false)

// Push notification state
const showPush = ref(false)
const pushLoading = ref(false)
const pushSubscribed = ref(false)
const pushTitle = ref('Enable Push Notifications')
const pushDesc = ref("Get reminded when it's time to check in — no calendar needed.")
const pushStatus = ref('')
const pushStatusType = ref<'success' | 'error' | ''>('')

function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4)
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/')
  const rawData = window.atob(base64)
  const outputArray = new Uint8Array(rawData.length) as Uint8Array<ArrayBuffer>
  for (let i = 0; i < rawData.length; ++i) {
    outputArray[i] = rawData.charCodeAt(i)
  }
  return outputArray
}

async function doCheckIn() {
  if (!vaultId.value || !token.value) {
    statusMsg.value = 'Missing vault ID or token. Please use the link from your calendar or notification.'
    status.value = 'error'
    return
  }

  loading.value = true
  statusMsg.value = ''

  try {
    const url = `/api/v1/vaults/${encodeURIComponent(vaultId.value)}/checkin?token=${encodeURIComponent(token.value)}&method=web`
    const resp = await fetch(url, { method: 'POST' })
    const data = await resp.json()

    if (resp.ok && data.ok) {
      status.value = 'success'
      showCheckmark.value = true
      statusTitle.value = 'Check-In Successful'
      statusSubtitle.value = 'Your vault is safe. See you next time!'
      btnText.value = '✓ Checked In'

      if (data.lastCheckIn) {
        const d = new Date(data.lastCheckIn)
        statusMsg.value = 'Last check-in recorded: ' + d.toLocaleString()
      }

      showPushSection()
    } else {
      const errMsg = data?.error || 'Check-in failed. Please try again.'
      status.value = 'error'
      statusTitle.value = 'Check-In Failed'
      statusSubtitle.value = 'Something went wrong.'
      statusMsg.value = errMsg
      btnText.value = 'Try Again'
    }
  } catch (err: any) {
    status.value = 'error'
    statusTitle.value = 'Connection Error'
    statusSubtitle.value = 'Could not reach the server.'
    statusMsg.value = 'Network error: ' + err.message
    btnText.value = 'Try Again'
  } finally {
    loading.value = false
  }
}

async function showPushSection() {
  if (!('serviceWorker' in navigator) || !('PushManager' in window)) return

  try {
    const reg = await navigator.serviceWorker.getRegistration('/sw.js')
    if (reg) {
      const sub = await reg.pushManager.getSubscription()
      if (sub) {
        pushTitle.value = 'Notifications Active'
        pushDesc.value = 'You are already subscribed to push notifications for this vault.'
        pushSubscribed.value = true
      }
    }
  } catch {
    // Ignore
  }

  showPush.value = true
}

async function subscribePush() {
  pushLoading.value = true
  pushStatus.value = ''
  pushStatusType.value = ''

  try {
    // 1. Get VAPID public key
    const keyResp = await fetch('/api/v1/push/vapid-key')
    const keyData = await keyResp.json()
    if (!keyData.publicKey) {
      throw new Error('Push notifications are not configured on this server.')
    }

    // 2. Register service worker
    await navigator.serviceWorker.register('/sw.js', { scope: '/' })
    const reg = await navigator.serviceWorker.ready

    // 3. Subscribe to push
    const vapidKey = urlBase64ToUint8Array(keyData.publicKey)
    const subscription = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: vapidKey,
    })

    // 4. Send subscription to server
    const subJson = subscription.toJSON()
    await fetch(`/api/v1/vaults/${encodeURIComponent(vaultId.value)}/push/subscribe`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        endpoint: subJson.endpoint,
        p256dh: subJson.keys?.p256dh,
        auth: subJson.keys?.auth,
      }),
    })

    pushSubscribed.value = true
    pushTitle.value = 'Notifications Active'
    pushDesc.value = "You'll receive push notifications when it's time to check in."
    pushStatus.value = 'Push notifications enabled successfully!'
    pushStatusType.value = 'success'
  } catch (err: any) {
    let errMsg = err.message || 'Failed to enable push notifications.'
    if (err.name === 'NotAllowedError') {
      errMsg = 'Notification permission was denied. Please allow notifications in your browser settings.'
    }
    pushStatus.value = errMsg
    pushStatusType.value = 'error'
  } finally {
    pushLoading.value = false
  }
}

onMounted(() => {
  vaultId.value = (route.query.id as string) || ''
  token.value = (route.query.token as string) || ''

  if (vaultId.value && token.value) {
    doCheckIn()
  }
})
</script>

<template>
  <div class="checkin-page">
    <div class="checkin-card">
      <!-- Shield Icon -->
      <div class="checkin-icon" :class="status">
        <svg xmlns="http://www.w3.org/2000/svg" width="56" height="56" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <path
            d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" />
          <path d="m9 12 2 2 4-4" :style="{ opacity: showCheckmark ? 1 : 0, transition: 'opacity 0.4s ease' }" />
        </svg>
      </div>

      <h1 class="checkin-title">{{ statusTitle }}</h1>
      <p class="checkin-subtitle">{{ statusSubtitle }}</p>

      <!-- Check-In Button -->
      <button
        class="checkin-btn"
        :class="{ success: status === 'success' }"
        :disabled="loading || status === 'success'"
        @click="doCheckIn"
      >
        <span v-if="!loading">{{ btnText }}</span>
        <span v-else>Checking in…</span>
        <div v-if="loading" class="spinner" />
      </button>

      <!-- Push Notification Section -->
      <div v-if="showPush" class="push-section">
        <div class="push-divider" />
        <div class="push-card">
          <div class="push-icon-wrap">
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none"
              stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
              <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
            </svg>
          </div>
          <div class="push-text">
            <p class="push-title-text">{{ pushTitle }}</p>
            <p class="push-desc-text">{{ pushDesc }}</p>
          </div>
        </div>
        <button
          class="push-btn"
          :class="{ subscribed: pushSubscribed }"
          :disabled="pushLoading || pushSubscribed"
          @click="subscribePush"
        >
          <span v-if="!pushLoading && !pushSubscribed">Enable Notifications</span>
          <span v-else-if="pushSubscribed">✓ Subscribed</span>
          <span v-else>Setting up…</span>
          <div v-if="pushLoading" class="spinner" />
        </button>
        <div v-if="pushStatus" class="push-status" :class="pushStatusType">{{ pushStatus }}</div>
      </div>

      <!-- Status Message -->
      <div v-if="statusMsg" class="status-msg" :class="status">{{ statusMsg }}</div>

      <!-- Footer -->
      <div class="checkin-footer">
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
          <path d="M7 11V7a5 5 0 0 1 10 0v4" />
        </svg>
        <span>PassedBox Dead Man's Switch</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.checkin-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #0f0f23 0%, #1a1a3e 50%, #0f0f23 100%);
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Inter', sans-serif;
  color: #e2e8f0;
  padding: 1rem;
}

.checkin-card {
  width: 100%;
  max-width: 420px;
  background: rgba(30, 31, 60, 0.8);
  border: 1px solid rgba(99, 102, 241, 0.2);
  border-radius: 20px;
  padding: 3rem 2.5rem;
  text-align: center;
  backdrop-filter: blur(20px);
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.4), 0 0 40px rgba(99, 102, 241, 0.05);
}

.checkin-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 96px;
  height: 96px;
  border-radius: 50%;
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.15), rgba(139, 92, 246, 0.15));
  border: 1px solid rgba(99, 102, 241, 0.25);
  color: #818cf8;
  margin-bottom: 1.5rem;
  transition: all 0.5s ease;
}

.checkin-icon.success {
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.15), rgba(52, 211, 153, 0.15));
  border-color: rgba(16, 185, 129, 0.3);
  color: #34d399;
}

.checkin-icon.error {
  background: linear-gradient(135deg, rgba(239, 68, 68, 0.15), rgba(248, 113, 113, 0.15));
  border-color: rgba(239, 68, 68, 0.3);
  color: #f87171;
}

.checkin-title {
  font-size: 1.5rem;
  font-weight: 700;
  margin-bottom: 0.5rem;
  background: linear-gradient(135deg, #e2e8f0, #a5b4fc);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.checkin-subtitle {
  font-size: 0.9rem;
  color: #94a3b8;
  line-height: 1.5;
  margin-bottom: 2rem;
}

.checkin-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  width: 100%;
  padding: 0.9rem 1.5rem;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  border: none;
  border-radius: 12px;
  color: #fff;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s ease;
  box-shadow: 0 4px 16px rgba(99, 102, 241, 0.3);
  min-height: 50px;
}

.checkin-btn:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 6px 24px rgba(99, 102, 241, 0.4);
}

.checkin-btn:active:not(:disabled) {
  transform: translateY(0);
}

.checkin-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.checkin-btn.success {
  background: linear-gradient(135deg, #10b981, #34d399);
  box-shadow: 0 4px 16px rgba(16, 185, 129, 0.3);
}

.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.status-msg {
  margin-top: 1.25rem;
  padding: 0.75rem 1rem;
  border-radius: 10px;
  font-size: 0.85rem;
  line-height: 1.4;
}

.status-msg.success {
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
  color: #34d399;
}

.status-msg.error {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.2);
  color: #f87171;
}

/* Push Notification Section */
.push-section {
  margin-top: 1.5rem;
}

.push-divider {
  height: 1px;
  background: rgba(99, 102, 241, 0.15);
  margin-bottom: 1.25rem;
}

.push-card {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  padding: 1rem;
  background: rgba(99, 102, 241, 0.06);
  border: 1px solid rgba(99, 102, 241, 0.15);
  border-radius: 12px;
  text-align: left;
  margin-bottom: 0.75rem;
}

.push-icon-wrap {
  flex-shrink: 0;
  color: #818cf8;
  margin-top: 2px;
}

.push-text {
  flex: 1;
}

.push-title-text {
  font-size: 0.9rem;
  font-weight: 600;
  color: #e2e8f0;
  margin-bottom: 0.25rem;
}

.push-desc-text {
  font-size: 0.8rem;
  color: #94a3b8;
  line-height: 1.4;
}

.push-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  width: 100%;
  padding: 0.7rem 1rem;
  background: rgba(99, 102, 241, 0.15);
  border: 1px solid rgba(99, 102, 241, 0.3);
  border-radius: 10px;
  color: #a5b4fc;
  font-size: 0.9rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.3s ease;
  min-height: 44px;
}

.push-btn:hover:not(:disabled) {
  background: rgba(99, 102, 241, 0.25);
  border-color: rgba(99, 102, 241, 0.5);
}

.push-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.push-btn.subscribed {
  background: rgba(16, 185, 129, 0.1);
  border-color: rgba(16, 185, 129, 0.3);
  color: #34d399;
}

.push-status {
  margin-top: 0.75rem;
  padding: 0.6rem 0.8rem;
  border-radius: 8px;
  font-size: 0.8rem;
  line-height: 1.4;
}

.push-status.success {
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
  color: #34d399;
}

.push-status.error {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.2);
  color: #f87171;
}

.checkin-footer {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  margin-top: 2rem;
  font-size: 0.75rem;
  color: #475569;
}

@media (max-width: 480px) {
  .checkin-card {
    padding: 2rem 1.5rem;
  }

  .checkin-title {
    font-size: 1.25rem;
  }
}
</style>
