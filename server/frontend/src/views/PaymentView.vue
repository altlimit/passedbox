<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()

const status = ref<'confirming' | 'success' | 'error' | 'cancelled'>('confirming')
const message = ref('')
const credits = ref(0)
const releaseDate = ref('')

function formatDate(d: string) {
  if (!d || d === '0001-01-01T00:00:00Z') return '—'
  return new Date(d).toLocaleDateString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric',
  })
}

onMounted(async () => {
  const queryStatus = route.query.status as string
  const vaultId = route.query.id as string
  const sessionId = route.query.session_id as string

  if (queryStatus === 'cancelled') {
    status.value = 'cancelled'
    message.value = 'Payment was cancelled.'
    return
  }

  if (!sessionId || !vaultId) {
    status.value = 'error'
    message.value = 'Missing payment information.'
    return
  }

  try {
    const res = await fetch(`/api/v1/vaults/${vaultId}/confirm-payment`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ sessionId }),
    })
    if (!res.ok) throw new Error(await res.text())
    const data = await res.json()

    if (data.status === 'paid') {
      status.value = 'success'
      credits.value = data.credits
      releaseDate.value = data.releaseDate
      message.value = 'Payment confirmed! Credits have been added to your vault.'
    } else {
      status.value = 'error'
      message.value = 'Payment has not been completed yet. Please try again later.'
    }
  } catch (e: any) {
    status.value = 'error'
    message.value = e.message || 'Failed to confirm payment.'
  }
})
</script>

<template>
  <div class="payment-page">
    <div class="payment-card">
      <!-- Confirming -->
      <template v-if="status === 'confirming'">
        <div class="payment-icon spin">⏳</div>
        <h2>Confirming Payment…</h2>
        <p class="text-dim">Please wait while we verify your payment with Stripe.</p>
      </template>

      <!-- Success -->
      <template v-else-if="status === 'success'">
        <div class="payment-icon success">✓</div>
        <h2>Payment Confirmed!</h2>
        <p>{{ message }}</p>
        <div class="payment-details" v-if="credits > 0">
          <div class="detail-row">
            <span class="detail-label">Total Credits</span>
            <span class="detail-value">{{ credits }} year(s)</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">Release Date</span>
            <span class="detail-value">{{ formatDate(releaseDate) }}</span>
          </div>
        </div>
      </template>

      <!-- Cancelled -->
      <template v-else-if="status === 'cancelled'">
        <div class="payment-icon cancelled">✕</div>
        <h2>Payment Cancelled</h2>
        <p>{{ message }}</p>
      </template>

      <!-- Error -->
      <template v-else>
        <div class="payment-icon error">!</div>
        <h2>Payment Error</h2>
        <p>{{ message }}</p>
      </template>
    </div>
  </div>
</template>

<style scoped>
.payment-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 80vh;
}

.payment-card {
  text-align: center;
  max-width: 420px;
  padding: 2.5rem;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg, 1rem);
}

.payment-icon {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 1.75rem;
  margin-bottom: 1rem;
}

.payment-icon.success {
  background: rgba(16, 185, 129, 0.15);
  color: var(--success, #10b981);
}

.payment-icon.cancelled {
  background: rgba(245, 158, 11, 0.15);
  color: var(--warning, #f59e0b);
}

.payment-icon.error {
  background: rgba(239, 68, 68, 0.15);
  color: var(--danger, #ef4444);
}

.payment-icon.spin {
  background: rgba(99, 102, 241, 0.15);
  color: var(--accent, #6366f1);
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.payment-card h2 {
  margin-bottom: 0.5rem;
}

.payment-card p {
  color: var(--text-dim);
  margin-bottom: 1.5rem;
  font-size: 0.9rem;
}

.payment-details {
  text-align: left;
  background: var(--bg);
  border-radius: var(--radius-sm, 0.5rem);
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.detail-row {
  display: flex;
  justify-content: space-between;
}

.detail-label {
  color: var(--text-dim);
  font-size: 0.85rem;
}

.detail-value {
  font-weight: 500;
  font-size: 0.85rem;
}
</style>
