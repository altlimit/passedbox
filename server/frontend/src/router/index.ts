import { createRouter, createWebHistory } from 'vue-router'

const APP_NAME = 'PassedBox'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue'),
      meta: { public: true, title: 'Login' },
    },
    {
      path: '/checkin',
      name: 'checkin',
      component: () => import('../views/CheckinView.vue'),
      meta: { public: true, title: 'Keep-Alive Check-In' },
    },
    {
      path: '/payment',
      name: 'payment',
      component: () => import('../views/PaymentView.vue'),
      meta: { public: true, title: 'Payment Confirmation' },
    },
    {
      path: '/',
      component: () => import('../layouts/AppLayout.vue'),
      children: [
        {
          path: '',
          name: 'dashboard',
          component: () => import('../views/DashboardView.vue'),
          meta: { title: 'Dashboard' },
        },
        {
          path: 'vaults',
          name: 'vaults',
          component: () => import('../views/VaultsView.vue'),
          meta: { title: 'Vaults' },
        },
        {
          path: 'vaults/:id',
          name: 'vault-detail',
          component: () => import('../views/VaultDetailView.vue'),
          meta: { title: 'Vault Details' },
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('../views/SettingsView.vue'),
          meta: { title: 'Settings' },
        },
      ],
    },
  ],
})

// Auth guard — check session on protected routes
let authChecked = false
let isAuthenticated = false

router.beforeEach(async (to) => {
  if (to.meta.public) return true

  if (!authChecked) {
    try {
      const res = await fetch('/api/v1/vaults', { method: 'GET' })
      isAuthenticated = res.ok
    } catch {
      isAuthenticated = false
    }
    authChecked = true
  }

  if (!isAuthenticated) {
    return { name: 'login' }
  }
  return true
})

// Set document title after each navigation
router.afterEach((to) => {
  const title = to.meta.title as string | undefined
  document.title = title ? `${title} — ${APP_NAME}` : APP_NAME
})

// Reset auth state on logout
export function resetAuth() {
  authChecked = false
  isAuthenticated = false
}

export default router
