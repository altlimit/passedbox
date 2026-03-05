<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { resetAuth } from '../router'

const router = useRouter()
const password = ref('')
const loading = ref(false)
const error = ref('')

async function handleLogin() {
  if (!password.value) return
  loading.value = true
  error.value = ''
  try {
    const res = await fetch('/api/v1/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: password.value }),
    })
    const data = await res.json()
    if (res.ok && data.ok) {
      resetAuth()
      router.push('/')
    } else {
      error.value = data.error || 'Login failed'
      password.value = ''
    }
  } catch (e: any) {
    error.value = 'Network error: ' + e.message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-icon">
        <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
          <path d="M7 11V7a5 5 0 0 1 10 0v4" />
        </svg>
      </div>
      <h1 class="login-title">PassedBox</h1>
      <p class="login-subtitle">Sign in to manage your vaults</p>

      <form @submit.prevent="handleLogin">
        <div class="form-group">
          <input
            v-model="password"
            type="password"
            placeholder="Admin Password"
            autocomplete="current-password"
            autofocus
            required
          />
        </div>
        <button type="submit" class="login-btn" :disabled="loading || !password">
          <span v-if="!loading">Sign In</span>
          <span v-else>Signing in…</span>
        </button>
      </form>

      <div v-if="error" class="login-error">{{ error }}</div>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #0f0f23 0%, #1a1a3e 50%, #0f0f23 100%);
  padding: 1rem;
}

.login-card {
  width: 100%;
  max-width: 380px;
  background: rgba(30, 31, 60, 0.8);
  border: 1px solid rgba(99, 102, 241, 0.2);
  border-radius: 20px;
  padding: 3rem 2.5rem;
  text-align: center;
  backdrop-filter: blur(20px);
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.4), 0 0 40px rgba(99, 102, 241, 0.05);
}

.login-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 88px;
  height: 88px;
  border-radius: 50%;
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.15), rgba(139, 92, 246, 0.15));
  border: 1px solid rgba(99, 102, 241, 0.25);
  color: #818cf8;
  margin-bottom: 1.5rem;
}

.login-title {
  font-size: 1.5rem;
  font-weight: 700;
  margin-bottom: 0.4rem;
  background: linear-gradient(135deg, #e2e8f0, #a5b4fc);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.login-subtitle {
  font-size: 0.9rem;
  color: #94a3b8;
  margin-bottom: 2rem;
}

.form-group {
  margin-bottom: 1rem;
}

.form-group input {
  width: 100%;
  padding: 0.85rem 1rem;
  background: rgba(15, 15, 35, 0.6);
  border: 1px solid rgba(99, 102, 241, 0.2);
  border-radius: 10px;
  color: #e2e8f0;
  font-size: 0.95rem;
  outline: none;
  transition: all 0.3s ease;
}

.form-group input:focus {
  border-color: rgba(99, 102, 241, 0.5);
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.form-group input::placeholder {
  color: #64748b;
}

.login-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  width: 100%;
  padding: 0.85rem;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  border: none;
  border-radius: 10px;
  color: #fff;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s ease;
  box-shadow: 0 4px 16px rgba(99, 102, 241, 0.3);
}

.login-btn:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 6px 24px rgba(99, 102, 241, 0.4);
}

.login-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.login-error {
  margin-top: 1rem;
  padding: 0.7rem 1rem;
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.2);
  border-radius: 8px;
  color: #f87171;
  font-size: 0.85rem;
}
</style>
