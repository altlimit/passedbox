import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { resetAuth } from '../router'

const API_BASE = '/api/v1'

// Wrapper that redirects to login on 401
async function apiFetch(url: string, init?: RequestInit): Promise<Response> {
    const res = await fetch(url, init)
    if (res.status === 401) {
        resetAuth()
        window.location.href = '/login'
        throw new Error('Session expired')
    }
    return res
}

export interface Vault {
    id: string
    status: string
    releaseOnExpiry: boolean
    enableKeepAlive: boolean
    keepAliveDays: number
    lastCheckIn: string
    released: boolean
    releasedAt: string
    createdAt: string
    updatedAt: string
    // Detail-only fields
    credits?: number
    releaseDate?: string
    creditsActive?: boolean
}

export interface Stats {
    total: number
    active: number
    released: number
    pending: number
    keepAlive: number
}

export const useVaultStore = defineStore('vault', () => {
    const vaults = ref<Vault[]>([])
    const currentVault = ref<Vault | null>(null)
    const loading = ref(false)
    const error = ref<string | null>(null)
    const stats = ref<Stats>({ total: 0, active: 0, released: 0, pending: 0, keepAlive: 0 })
    const cursor = ref<string>('')
    const hasMore = ref(false)

    const activeVaults = computed(() => vaults.value.filter(v => !v.released))
    const releasedVaults = computed(() => vaults.value.filter(v => v.released))

    function clearError() {
        error.value = null
    }

    async function fetchStats() {
        try {
            const res = await apiFetch(`${API_BASE}/stats`)
            if (!res.ok) throw await res.json()
            stats.value = await res.json()
        } catch (e: any) {
            error.value = e.error || e.message
        }
    }

    async function fetchVaults(opts?: { cursor?: string; limit?: number; reset?: boolean }) {
        const limit = opts?.limit ?? 20
        const cursorVal = opts?.cursor ?? ''
        const reset = opts?.reset ?? !cursorVal

        if (reset) {
            loading.value = true
        }
        error.value = null
        try {
            const params = new URLSearchParams({ limit: String(limit) })
            if (cursorVal) params.set('cursor', cursorVal)

            const res = await apiFetch(`${API_BASE}/vaults?${params}`)
            if (!res.ok) throw await res.json()
            const data = await res.json()

            if (reset) {
                vaults.value = data.vaults || []
            } else {
                vaults.value = [...vaults.value, ...(data.vaults || [])]
            }
            cursor.value = data.cursor || ''
            hasMore.value = !!(data.cursor && (data.vaults || []).length >= limit)
        } catch (e: any) {
            error.value = e.error || e.message
        } finally {
            loading.value = false
        }
    }

    async function fetchVault(id: string) {
        loading.value = true
        error.value = null
        try {
            const res = await apiFetch(`${API_BASE}/vaults/${id}`)
            if (!res.ok) throw await res.json()
            currentVault.value = await res.json()
        } catch (e: any) {
            error.value = e.error || e.message
            currentVault.value = null
        } finally {
            loading.value = false
        }
    }

    async function addVault(id: string, share3Enc: string) {
        error.value = null
        try {
            const res = await apiFetch(`${API_BASE}/vaults`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    id,
                    share3Enc: share3Enc ? Array.from(atob(share3Enc), c => c.charCodeAt(0)) : [],
                    releaseOnExpiry: false,
                    enableKeepAlive: false,
                    keepAliveDays: 30,
                }),
            })
            if (!res.ok) throw await res.json()
            const result = await res.json()
            await fetchVaults()
            return result
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    async function updateVault(id: string, settings: Partial<Vault>) {
        error.value = null
        try {
            const res = await apiFetch(`${API_BASE}/vaults/${id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(settings),
            })
            if (!res.ok) throw await res.json()
            await fetchVault(id)
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    async function deleteVault(id: string) {
        error.value = null
        try {
            const res = await apiFetch(`${API_BASE}/vaults/${id}`, { method: 'DELETE' })
            if (!res.ok) throw await res.json()
            await fetchVaults()
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    async function releaseVault(id: string) {
        error.value = null
        try {
            const res = await apiFetch(`${API_BASE}/vaults/${id}/release`, { method: 'POST' })
            if (!res.ok) throw await res.json()
            await fetchVault(id)
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    async function addCredits(vaultId: string, years: number) {
        error.value = null
        try {
            const res = await apiFetch(`${API_BASE}/vaults/${vaultId}/credits`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ years }),
            })
            if (!res.ok) throw await res.json()
            await fetchVault(vaultId)
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    async function buyCredits(vaultId: string, years: number) {
        error.value = null
        try {
            const res = await apiFetch(`${API_BASE}/vaults/${vaultId}/buy`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ years }),
            })
            if (!res.ok) throw await res.json()
            const result = await res.json()
            // Redirect to Stripe checkout
            if (result.checkoutUrl) {
                window.location.href = result.checkoutUrl
            }
            return result
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    async function confirmPayment(vaultId: string, sessionId: string) {
        error.value = null
        try {
            const res = await fetch(`${API_BASE}/vaults/${vaultId}/confirm-payment`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ sessionId }),
            })
            if (!res.ok) throw await res.json()
            const result = await res.json()
            await fetchVault(vaultId)
            return result
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    async function manualCheckIn(vaultId: string) {
        error.value = null
        try {
            const res = await fetch(`${API_BASE}/vaults/${vaultId}/checkin`, { method: 'POST' })
            if (!res.ok) throw await res.json()
            await fetchVault(vaultId)
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    async function sendTestPush(vaultId: string) {
        error.value = null
        try {
            const res = await apiFetch(`${API_BASE}/vaults/${vaultId}/push/test`, { method: 'POST' })
            if (!res.ok) throw await res.json()
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    async function approveVault(vaultId: string) {
        error.value = null
        try {
            const res = await apiFetch(`${API_BASE}/vaults/${vaultId}/approve`, { method: 'POST' })
            if (!res.ok) throw await res.json()
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    async function createPendingVault(vaultId: string) {
        error.value = null
        try {
            const res = await apiFetch(`${API_BASE}/vaults/pending`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id: vaultId }),
            })
            if (!res.ok) throw await res.json()
        } catch (e: any) {
            error.value = e.error || e.message
            throw e
        }
    }

    return {
        vaults, currentVault, loading, error, stats, cursor, hasMore,
        activeVaults, releasedVaults,
        fetchStats, fetchVaults, fetchVault, addVault, updateVault, deleteVault, releaseVault,
        addCredits, buyCredits, confirmPayment, manualCheckIn, sendTestPush,
        approveVault, createPendingVault, clearError,
    }
})
